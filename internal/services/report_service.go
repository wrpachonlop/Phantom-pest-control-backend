package services

import (
	"context"
	"fmt"
	"time"

	"github.com/phantompestcontrol/crm/internal/dto"
	"github.com/phantompestcontrol/crm/internal/repositories"
	"go.uber.org/zap"
)

// ReportService computes dynamic reports. Reports are NEVER cached or snapshotted –
// they are always calculated fresh from the database to maintain retroactive accuracy.
//
// CORE RULE:
//
//	ALL sales count by first_contact_date.
//	If a client was first contacted last week and sold today,
//	the sale appears in last week's report.
type ReportService struct {
	reportRepo *repositories.ReportRepository
	logger     *zap.Logger
}

func NewReportService(reportRepo *repositories.ReportRepository, logger *zap.Logger) *ReportService {
	return &ReportService{reportRepo: reportRepo, logger: logger}
}

// GetPeriodReport computes a full report for a given period type.
func (s *ReportService) GetPeriodReport(
	ctx context.Context,
	req *dto.ReportRequest,
) (*dto.ReportPeriodResult, error) {
	from, to, label, err := s.resolvePeriod(req)
	if err != nil {
		return nil, err
	}
	return s.computeReport(ctx, from, to, label)
}

// GetDashboard returns all stats needed for the main dashboard.
func (s *ReportService) GetDashboard(ctx context.Context, anchor time.Time) (*dto.DashboardResponse, error) {
	loc := anchor.Location()

	// Status distribution
	statusDist, err := s.reportRepo.GetStatusDistribution(ctx)
	if err != nil {
		return nil, fmt.Errorf("status distribution: %w", err)
	}

	// Today
	todayStart := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, loc)
	todayEnd := todayStart.Add(24*time.Hour - time.Nanosecond)
	todayStats, err := s.computeReport(ctx, todayStart, todayEnd, "Today")
	if err != nil {
		return nil, err
	}

	// This week (Mon–Sun)
	weekStart := weekStartDate(anchor)
	weekEnd := weekStart.AddDate(0, 0, 6).Add(24*time.Hour - time.Nanosecond)
	thisWeekStats, err := s.computeReport(ctx, weekStart, weekEnd, "This Week")
	if err != nil {
		return nil, err
	}

	// This month
	monthStart := time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, loc)
	monthEnd := monthStart.AddDate(0, 1, 0).Add(-time.Nanosecond)
	thisMonthStats, err := s.computeReport(ctx, monthStart, monthEnd, "This Month")
	if err != nil {
		return nil, err
	}

	// Weekly trend: last 12 weeks
	weeklyTrendPeriods := make([]dto.DateRange, 12)
	for i := 11; i >= 0; i-- {
		ws := weekStart.AddDate(0, 0, -(7 * i))
		we := ws.AddDate(0, 0, 6).Add(24*time.Hour - time.Nanosecond)
		label := fmt.Sprintf("W%s", ws.Format("Jan 2"))
		weeklyTrendPeriods[11-i] = dto.DateRange{Label: label, From: ws, To: we}
	}
	weeklyTrend, err := s.reportRepo.GetMultiPeriodSummary(ctx, weeklyTrendPeriods)
	if err != nil {
		return nil, err
	}

	// Top performers (this month)
	topPerformers, err := s.reportRepo.GetTopPerformers(ctx, monthStart, monthEnd, 5)
	if err != nil {
		return nil, err
	}

	return &dto.DashboardResponse{
		StatusDistribution: statusDist,
		WeeklyTrend:        weeklyTrend,
		TopPerformers:      topPerformers,
		TodayStats:         todayStats,
		ThisWeekStats:      thisWeekStats,
		ThisMonthStats:     thisMonthStats,
	}, nil
}

// computeReport assembles a ReportPeriodResult for the given window.
func (s *ReportService) computeReport(
	ctx context.Context,
	from, to time.Time,
	label string,
) (*dto.ReportPeriodResult, error) {
	// Residential
	residentialRows, err := s.reportRepo.GetResidentialStats(ctx, from, to)
	if err != nil {
		return nil, fmt.Errorf("residential stats: %w", err)
	}

	residentialSection := &dto.ResidentialReportSection{}
	for _, row := range residentialRows {
		rate := conversionRate(row.TotalSold, row.TotalReceived)
		residentialSection.ByContactMethod = append(
			residentialSection.ByContactMethod,
			dto.ContactMethodBreakdown{
				ContactMethodID:   row.ContactMethodID,
				ContactMethodName: row.ContactMethodName,
				Received:          row.TotalReceived,
				Sold:              row.TotalSold,
				ConversionRate:    rate,
			},
		)
		residentialSection.TotalReceived += row.TotalReceived
		residentialSection.TotalSold += row.TotalSold
	}
	residentialSection.ConversionRate = conversionRate(
		residentialSection.TotalSold,
		residentialSection.TotalReceived,
	)

	// Commercial
	commercialRow, err := s.reportRepo.GetCommercialStats(ctx, from, to)
	if err != nil {
		return nil, fmt.Errorf("commercial stats: %w", err)
	}
	commercialSection := &dto.CommercialReportSection{
		TotalReceived:  commercialRow.TotalReceived,
		TotalSold:      commercialRow.TotalSold,
		ConversionRate: conversionRate(commercialRow.TotalSold, commercialRow.TotalReceived),
	}

	// After hours
	afterHoursRow, err := s.reportRepo.GetAfterHoursStats(ctx, from, to)
	if err != nil {
		return nil, fmt.Errorf("after hours stats: %w", err)
	}
	afterHoursSection := &dto.AfterHoursSection{
		TotalReceived:  afterHoursRow.TotalReceived,
		TotalSold:      afterHoursRow.TotalSold,
		ConversionRate: conversionRate(afterHoursRow.TotalSold, afterHoursRow.TotalReceived),
	}

	// Totals
	totalReceived := residentialSection.TotalReceived + commercialSection.TotalReceived
	totalSold := residentialSection.TotalSold + commercialSection.TotalSold
	totals := &dto.ReportTotals{
		TotalReceived:  totalReceived,
		TotalSold:      totalSold,
		ConversionRate: conversionRate(totalSold, totalReceived),
	}

	return &dto.ReportPeriodResult{
		PeriodLabel: label,
		DateFrom:    from,
		DateTo:      to,
		Residential: residentialSection,
		Commercial:  commercialSection,
		AfterHours:  afterHoursSection,
		Totals:      totals,
	}, nil
}

// resolvePeriod converts a ReportRequest into concrete from/to timestamps.
func (s *ReportService) resolvePeriod(req *dto.ReportRequest) (from, to time.Time, label string, err error) {
	loc, _ := time.LoadLocation("America/Vancouver")
	now := time.Now().In(loc)

	switch req.Period {
	case "daily":
		anchor := now
		if req.AnchorDate != "" {
			anchor, err = time.ParseInLocation(time.DateOnly, req.AnchorDate, loc)
			if err != nil {
				return time.Time{}, time.Time{}, "", fmt.Errorf("invalid anchor_date: %w", err)
			}
		}
		from = time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, loc)
		to = from.Add(24*time.Hour - time.Nanosecond)
		label = from.Format("Jan 2, 2006")

	case "weekly":
		anchor := now
		if req.AnchorDate != "" {
			anchor, err = time.ParseInLocation(time.DateOnly, req.AnchorDate, loc)
			if err != nil {
				return time.Time{}, time.Time{}, "", fmt.Errorf("invalid anchor_date: %w", err)
			}
		}
		from = weekStartDate(anchor).In(loc)
		to = from.AddDate(0, 0, 7).Add(-time.Nanosecond)
		label = fmt.Sprintf("Week of %s", from.Format("Jan 2, 2006"))

	case "monthly":
		anchor := now
		if req.AnchorDate != "" {
			anchor, err = time.ParseInLocation(time.DateOnly, req.AnchorDate, loc)
			if err != nil {
				return time.Time{}, time.Time{}, "", fmt.Errorf("invalid anchor_date: %w", err)
			}
		}
		from = time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, loc)
		to = from.AddDate(0, 1, 0).Add(-time.Nanosecond)
		label = from.Format("January 2006")

	case "custom":
		if req.DateFrom == "" || req.DateTo == "" {
			return time.Time{}, time.Time{}, "", fmt.Errorf("date_from and date_to are required for custom period")
		}
		from, err = time.ParseInLocation(time.DateOnly, req.DateFrom, loc)
		if err != nil {
			return time.Time{}, time.Time{}, "", fmt.Errorf("invalid date_from: %w", err)
		}
		to, err = time.ParseInLocation(time.DateOnly, req.DateTo, loc)
		if err != nil {
			return time.Time{}, time.Time{}, "", fmt.Errorf("invalid date_to: %w", err)
		}
		to = to.Add(24*time.Hour - time.Nanosecond) // inclusive end
		label = fmt.Sprintf("%s – %s", from.Format("Jan 2"), to.Format("Jan 2, 2006"))

	default:
		return time.Time{}, time.Time{}, "", fmt.Errorf("unknown period type: %q", req.Period)
	}

	return from, to, label, nil
}

// weekStartDate returns the Monday of the week containing t.
func weekStartDate(t time.Time) time.Time {
	weekday := t.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	monday := t.AddDate(0, 0, -int(weekday-time.Monday))
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, t.Location())
}

func conversionRate(sold, received int) float64 {
	if received == 0 {
		return 0
	}
	return float64(sold) / float64(received) * 100
}
