package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/phantompestcontrol/crm/internal/dto"
)

// ReportRepository handles all reporting queries.
// CRITICAL RULE: All sales are counted by first_contact_date, NOT sold_date.
// This means if a client contacted last week and sold today,
// the sale counts in LAST WEEK's report.
type ReportRepository struct {
	db *pgxpool.Pool
}

func NewReportRepository(db *pgxpool.Pool) *ReportRepository {
	return &ReportRepository{db: db}
}

// ResidentialRow is a raw row from the residential reporting query.
type ResidentialRow struct {
	ContactMethodID   uuid.UUID
	ContactMethodName string
	TotalReceived     int
	TotalSold         int
}

// CommercialRow is a raw row from the commercial reporting query.
type CommercialRow struct {
	TotalReceived int
	TotalSold     int
}

// AfterHoursRow is a raw row from the after_hours query.
type AfterHoursRow struct {
	TotalReceived int
	TotalSold     int
}

// GetResidentialStats returns residential breakdowns grouped by contact_method
// for the given date window, keyed on first_contact_date.
//
// IMPORTANT: The window is on first_contact_date, not client_contact_date or sold_date.
// This implements the retroactive sale logic: if sold today but first contacted last week,
// the sale appears in last week's report.
func (r *ReportRepository) GetResidentialStats(
	ctx context.Context,
	from, to time.Time,
) ([]ResidentialRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			cm.id                                           AS contact_method_id,
			cm.name                                         AS contact_method_name,
			COUNT(c.id)                                     AS total_received,
			COUNT(c.id) FILTER (WHERE c.status = 'green')  AS total_sold
		FROM clients c
		JOIN contact_methods cm ON cm.id = c.contact_method_id
		WHERE
			c.property_type = 'residential'
			AND c.client_contact_date >= $1
			AND c.client_contact_date <= $2
		GROUP BY cm.id, cm.name
		ORDER BY cm.name
	`, from, to)
	if err != nil {
		return nil, fmt.Errorf("residential stats query: %w", err)
	}
	defer rows.Close()

	result := []ResidentialRow{}
	for rows.Next() {
		row := ResidentialRow{}
		if err := rows.Scan(
			&row.ContactMethodID,
			&row.ContactMethodName,
			&row.TotalReceived,
			&row.TotalSold,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, nil
}

// GetCommercialStats returns commercial totals for the given window.
// Commercial does NOT break down by contact method per spec.
func (r *ReportRepository) GetCommercialStats(
	ctx context.Context,
	from, to time.Time,
) (*CommercialRow, error) {
	row := &CommercialRow{}
	err := r.db.QueryRow(ctx, `
		SELECT
			COUNT(id)                                   AS total_received,
			COUNT(id) FILTER (WHERE status = 'green')  AS total_sold
		FROM clients
		WHERE
			property_type = 'commercial'
			AND client_contact_date >= $1
			AND client_contact_date <= $2
	`, from, to).Scan(&row.TotalReceived, &row.TotalSold)
	if err != nil {
		return nil, fmt.Errorf("commercial stats query: %w", err)
	}
	return row, nil
}

// GetAfterHoursStats returns after_hours performance within the window.
func (r *ReportRepository) GetAfterHoursStats(
	ctx context.Context,
	from, to time.Time,
) (*AfterHoursRow, error) {
	row := &AfterHoursRow{}
	err := r.db.QueryRow(ctx, `
		SELECT
			COUNT(id)                                   AS total_received,
			COUNT(id) FILTER (WHERE status = 'green')  AS total_sold
		FROM clients
		WHERE
			after_hours = TRUE
			AND client_contact_date >= $1
			AND client_contact_date <= $2
	`, from, to).Scan(&row.TotalReceived, &row.TotalSold)
	if err != nil {
		return nil, fmt.Errorf("after hours stats query: %w", err)
	}
	return row, nil
}

// GetMultiPeriodSummary returns a high-level summary across multiple periods.
// Used for dashboard trend charts. Returns data points suitable for charting.
func (r *ReportRepository) GetMultiPeriodSummary(
	ctx context.Context,
	periods []dto.DateRange,
) ([]dto.PeriodSummaryPoint, error) {
	result := make([]dto.PeriodSummaryPoint, 0, len(periods))

	for _, period := range periods {
		var received, sold int
		err := r.db.QueryRow(ctx, `
			SELECT
				COUNT(id)                                   AS received,
				COUNT(id) FILTER (WHERE status = 'green')  AS sold
			FROM clients
			WHERE
				first_contact_date >= $1
				AND first_contact_date <= $2
		`, period.From, period.To).Scan(&received, &sold)
		if err != nil {
			return nil, fmt.Errorf("multi-period summary: %w", err)
		}

		convRate := 0.0
		if received > 0 {
			convRate = float64(sold) / float64(received) * 100
		}

		result = append(result, dto.PeriodSummaryPoint{
			Label:          period.Label,
			From:           period.From,
			To:             period.To,
			TotalReceived:  received,
			TotalSold:      sold,
			ConversionRate: convRate,
		})
	}

	return result, nil
}

// GetTopPerformers returns users ranked by sales within the date window.
func (r *ReportRepository) GetTopPerformers(
	ctx context.Context,
	from, to time.Time,
	limit int,
) ([]dto.PerformerRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			u.id,
			u.full_name,
			u.email,
			COUNT(c.id) AS total_sales,
			COUNT(c.id) FILTER (WHERE c.property_type = 'residential') AS residential_sales,
			COUNT(c.id) FILTER (WHERE c.property_type = 'commercial')  AS commercial_sales
		FROM clients c
		JOIN users u ON u.id = c.sold_by
		WHERE
			c.status = 'green'
			AND c.first_contact_date >= $1
			AND c.first_contact_date <= $2
			AND c.sold_by IS NOT NULL
		GROUP BY u.id, u.full_name, u.email
		ORDER BY total_sales DESC
		LIMIT $3
	`, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("top performers query: %w", err)
	}
	defer rows.Close()

	performers := []dto.PerformerRow{}
	for rows.Next() {
		p := dto.PerformerRow{}
		if err := rows.Scan(
			&p.UserID, &p.FullName, &p.Email,
			&p.TotalSales, &p.ResidentialSales, &p.CommercialSales,
		); err != nil {
			return nil, err
		}
		performers = append(performers, p)
	}
	return performers, nil
}

// GetStatusDistribution returns client count by status for the dashboard.
func (r *ReportRepository) GetStatusDistribution(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.Query(ctx, `
		SELECT status, COUNT(*) FROM clients GROUP BY status
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dist := map[string]int{}
	for rows.Next() {
		var status string
		var count int
		rows.Scan(&status, &count)
		dist[status] = count
	}
	return dist, nil
}
