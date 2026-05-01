package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/phantompestcontrol/crm/internal/models"
)

// CommercialRepository handles all DB operations for the commercial workflow.
type CommercialRepository struct {
	db *pgxpool.Pool
}

func NewCommercialRepository(db *pgxpool.Pool) *CommercialRepository {
	return &CommercialRepository{db: db}
}

// ─── Commercial Details ───────────────────────────────────────────────────────

// GetDetailsByClientID fetches the commercial details for a client,
// joined with inspector and crew member.
func (r *CommercialRepository) GetDetailsByClientID(
	ctx context.Context,
	clientID uuid.UUID,
) (*models.CommercialClientDetails, error) {
	d := &models.CommercialClientDetails{}

	err := r.db.QueryRow(ctx, `
		SELECT
			ccd.id, ccd.client_id, ccd.workflow_status, ccd.lead_source,
			ccd.crew_member_id, ccd.inspector_id,
			ccd.company_name, ccd.contact_person_name,
			ccd.service_address, ccd.billing_address, ccd.billing_same_as_service,
			ccd.billing_terms, ccd.initial_setup_cost, ccd.recurring_service_cost,
			ccd.service_frequency, ccd.frequency_interval,
			ccd.phone_number, ccd.email, ccd.notes,
			ccd.proposal_drive_link,
			ccd.approved_by_name, ccd.approved_date, ccd.approved_by_user_id,
			ccd.next_followup_date,
			ccd.installation_date, ccd.installation_notes,
			ccd.cancelled_date, ccd.cancel_reason,
			ccd.created_at, ccd.updated_at
		FROM commercial_client_details ccd
		WHERE ccd.client_id = $1
	`, clientID).Scan(
		&d.ID, &d.ClientID, &d.WorkflowStatus, &d.LeadSource,
		&d.CrewMemberID, &d.InspectorID,
		&d.CompanyName, &d.ContactPersonName,
		&d.ServiceAddress, &d.BillingAddress, &d.BillingSameAsService,
		&d.BillingTerms, &d.InitialSetupCost, &d.RecurringServiceCost,
		&d.ServiceFrequency, &d.FrequencyInterval,
		&d.PhoneNumber, &d.Email, &d.Notes,
		&d.ProposalDriveLink,
		&d.ApprovedByName, &d.ApprovedDate, &d.ApprovedByUserID,
		&d.NextFollowupDate,
		&d.InstallationDate, &d.InstallationNotes,
		&d.CancelledDate, &d.CancelReason,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get commercial details: %w", err)
	}
	return d, nil
}

// CreateDetails inserts a new commercial_client_details row.
// Called atomically with the client creation transaction.
func (r *CommercialRepository) CreateDetails(
	ctx context.Context,
	tx pgx.Tx,
	d *models.CommercialClientDetails,
) (*models.CommercialClientDetails, error) {
	err := tx.QueryRow(ctx, `
		INSERT INTO commercial_client_details (
			client_id, workflow_status, lead_source, crew_member_id, inspector_id,
			company_name, contact_person_name,
			service_address, billing_address, billing_same_as_service,
			billing_terms, phone_number, email, notes
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING id, created_at, updated_at
	`,
		d.ClientID, d.WorkflowStatus, d.LeadSource, d.CrewMemberID, d.InspectorID,
		d.CompanyName, d.ContactPersonName,
		d.ServiceAddress, d.BillingAddress, d.BillingSameAsService,
		d.BillingTerms, d.PhoneNumber, d.Email, d.Notes,
	).Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create commercial details: %w", err)
	}
	return d, nil
}

// UpdateDetails performs a targeted partial update on commercial_client_details.
// Uses explicit field map to avoid blind struct serialisation.
func (r *CommercialRepository) UpdateDetails(
	ctx context.Context,
	clientID uuid.UUID,
	updates map[string]interface{},
) error {
	if len(updates) == 0 {
		return nil
	}

	setClauses := ""
	args := []interface{}{}
	i := 1
	for col, val := range updates {
		if i > 1 {
			setClauses += ", "
		}
		setClauses += fmt.Sprintf("%s = $%d", col, i)
		args = append(args, val)
		i++
	}
	args = append(args, clientID)

	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		UPDATE commercial_client_details
		SET %s, updated_at = NOW()
		WHERE client_id = $%d
	`, setClauses, i), args...)
	if err != nil {
		return fmt.Errorf("update commercial details: %w", err)
	}
	return nil
}

// ─── Inspector Assignment ──────────────────────────────────────────────────────

// RecordInspectorAssignment closes the current active assignment and
// opens a new one. Runs in a transaction for atomicity.
func (r *CommercialRepository) RecordInspectorAssignment(
	ctx context.Context,
	clientID, inspectorID uuid.UUID,
	assignedBy *uuid.UUID,
	notes *string,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Close current active assignment
	_, err = tx.Exec(ctx, `
		UPDATE commercial_inspector_assignments
		SET unassigned_at = NOW()
		WHERE client_id = $1 AND unassigned_at IS NULL
	`, clientID)
	if err != nil {
		return fmt.Errorf("close inspector assignment: %w", err)
	}

	// Also update the details table's inspector_id
	_, err = tx.Exec(ctx, `
		UPDATE commercial_client_details
		SET inspector_id = $2, updated_at = NOW()
		WHERE client_id = $1
	`, clientID, inspectorID)
	if err != nil {
		return fmt.Errorf("update inspector on details: %w", err)
	}

	// Open new assignment
	_, err = tx.Exec(ctx, `
		INSERT INTO commercial_inspector_assignments
			(client_id, inspector_id, assigned_by, notes)
		VALUES ($1, $2, $3, $4)
	`, clientID, inspectorID, assignedBy, notes)
	if err != nil {
		return fmt.Errorf("insert inspector assignment: %w", err)
	}

	return tx.Commit(ctx)
}

// GetAssignmentHistory returns all inspector assignments for a client.
func (r *CommercialRepository) GetAssignmentHistory(
	ctx context.Context,
	clientID uuid.UUID,
) ([]models.CommercialInspectorAssignment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			cia.id, cia.client_id, cia.inspector_id,
			cia.assigned_by, cia.assigned_at, cia.unassigned_at, cia.notes,
			u.full_name, u.email
		FROM commercial_inspector_assignments cia
		JOIN users u ON u.id = cia.inspector_id
		WHERE cia.client_id = $1
		ORDER BY cia.assigned_at DESC
	`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	assignments := []models.CommercialInspectorAssignment{}
	for rows.Next() {
		a := models.CommercialInspectorAssignment{
			Inspector: &models.User{},
		}
		rows.Scan(
			&a.ID, &a.ClientID, &a.InspectorID,
			&a.AssignedBy, &a.AssignedAt, &a.UnassignedAt, &a.Notes,
			&a.Inspector.FullName, &a.Inspector.Email,
		)
		assignments = append(assignments, a)
	}
	return assignments, nil
}

// ─── Status Transitions ────────────────────────────────────────────────────────

// RecordStatusTransition appends an immutable transition record.
func (r *CommercialRepository) RecordStatusTransition(
	ctx context.Context,
	tx pgx.Tx,
	t *models.CommercialStatusTransition,
) error {
	var execFn func(ctx context.Context, sql string, args ...interface{}) (interface{}, error)

	if tx != nil {
		_, err := tx.Exec(ctx, `
			INSERT INTO commercial_status_transitions
				(client_id, from_status, to_status, changed_by, proposal_link, notes)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, t.ClientID, t.FromStatus, t.ToStatus, t.ChangedBy, t.ProposalLink, t.Notes)
		return err
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO commercial_status_transitions
			(client_id, from_status, to_status, changed_by, proposal_link, notes)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, t.ClientID, t.FromStatus, t.ToStatus, t.ChangedBy, t.ProposalLink, t.Notes)
	_ = execFn
	return err
}

// GetTransitionHistory returns all status transitions for a client.
func (r *CommercialRepository) GetTransitionHistory(
	ctx context.Context,
	clientID uuid.UUID,
) ([]models.CommercialStatusTransition, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			cst.id, cst.client_id, cst.from_status, cst.to_status,
			cst.changed_by, cst.proposal_link, cst.notes, cst.created_at,
			u.full_name, u.email
		FROM commercial_status_transitions cst
		LEFT JOIN users u ON u.id = cst.changed_by
		WHERE cst.client_id = $1
		ORDER BY cst.created_at ASC
	`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	transitions := []models.CommercialStatusTransition{}
	for rows.Next() {
		t := models.CommercialStatusTransition{}
		var userFullName, userEmail *string
		rows.Scan(
			&t.ID, &t.ClientID, &t.FromStatus, &t.ToStatus,
			&t.ChangedBy, &t.ProposalLink, &t.Notes, &t.CreatedAt,
			&userFullName, &userEmail,
		)
		if userFullName != nil || userEmail != nil {
			t.ChangedByUser = &models.User{FullName: userFullName}
			if userEmail != nil {
				t.ChangedByUser.Email = *userEmail
			}
		}
		transitions = append(transitions, t)
	}
	return transitions, nil
}

// ─── Pending Reminders (for cron job) ─────────────────────────────────────────

// GetPendingFollowupsDue returns commercial clients whose next_followup_date
// is tomorrow (or overdue). Used by the daily cron reminder job.
func (r *CommercialRepository) GetPendingFollowupsDue(
	ctx context.Context,
	targetDate time.Time,
) ([]PendingReminderRow, error) {
	// Target = tomorrow's date (remind 1 day before)
	rows, err := r.db.Query(ctx, `
		SELECT
			ccd.client_id,
			COALESCE(ccd.company_name, c.client_name, 'Unknown') AS company_name,
			ccd.next_followup_date,
			u.full_name   AS inspector_name,
			u.email       AS inspector_email
		FROM commercial_client_details ccd
		JOIN clients c        ON c.id = ccd.client_id
		JOIN users u          ON u.id = ccd.inspector_id
		WHERE
			ccd.workflow_status = 'pending'
			AND ccd.next_followup_date = $1
	`, targetDate.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("get pending followups due: %w", err)
	}
	defer rows.Close()

	result := []PendingReminderRow{}
	for rows.Next() {
		row := PendingReminderRow{}
		rows.Scan(
			&row.ClientID,
			&row.CompanyName,
			&row.NextFollowupDate,
			&row.InspectorName,
			&row.InspectorEmail,
		)
		result = append(result, row)
	}
	return result, nil
}

type PendingReminderRow struct {
	ClientID         uuid.UUID
	CompanyName      string
	NextFollowupDate time.Time
	InspectorName    *string
	InspectorEmail   string
}

// ─── Crew Members ─────────────────────────────────────────────────────────────

// ListCrewMembers returns all active crew members.
func (r *CommercialRepository) ListCrewMembers(ctx context.Context) ([]models.CrewMember, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, full_name, employee_id, is_active, created_by, created_at, updated_at
		FROM crew_members
		ORDER BY full_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := []models.CrewMember{}
	for rows.Next() {
		m := models.CrewMember{}
		rows.Scan(&m.ID, &m.FullName, &m.EmployeeID, &m.IsActive, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt)
		members = append(members, m)
	}
	return members, nil
}

// ─── Inspectors ───────────────────────────────────────────────────────────────

// ListInspectors returns users with is_inspector = true.
func (r *CommercialRepository) ListInspectors(ctx context.Context) ([]models.User, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, email, full_name, role, avatar_url, is_active, created_at, updated_at
		FROM users
		WHERE is_inspector = true AND is_active = true
		ORDER BY full_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	inspectors := []models.User{}
	for rows.Next() {
		u := models.User{}
		rows.Scan(&u.ID, &u.Email, &u.FullName, &u.Role, &u.AvatarURL, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
		inspectors = append(inspectors, u)
	}
	return inspectors, nil
}

// ─── Notification Recipients ───────────────────────────────────────────────────

// GetRecipientsByEvent returns active notification recipients for a given event.
func (r *CommercialRepository) GetRecipientsByEvent(
	ctx context.Context,
	eventType string,
) ([]models.NotificationRecipient, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, event_type, name, email, is_active, created_at
		FROM notification_recipients
		WHERE event_type = $1 AND is_active = true
	`, eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	recipients := []models.NotificationRecipient{}
	for rows.Next() {
		nr := models.NotificationRecipient{}
		rows.Scan(&nr.ID, &nr.EventType, &nr.Name, &nr.Email, &nr.IsActive, &nr.CreatedAt)
		recipients = append(recipients, nr)
	}
	return recipients, nil
}
