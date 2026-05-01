package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/phantompestcontrol/crm/internal/dto"
	"github.com/phantompestcontrol/crm/internal/middleware"
	"github.com/phantompestcontrol/crm/internal/models"
	"github.com/phantompestcontrol/crm/internal/repositories"
	"go.uber.org/zap"
)

// CommercialService orchestrates all commercial workflow business logic.
// It deliberately knows nothing about HTTP — only domain rules.
type CommercialService struct {
	db             *pgxpool.Pool
	commercialRepo *repositories.CommercialRepository
	clientRepo     *repositories.ClientRepository
	notifier       NotificationSender
	audit          *middleware.AuditService
	logger         *zap.Logger
}

// NotificationSender is an interface so the notifier can be swapped
// (SMTP, SendGrid, log-only in tests).
type NotificationSender interface {
	SendCommercialApproved(ctx context.Context, details *models.CommercialClientDetails, recipients []models.NotificationRecipient) error
	SendPendingReminder(ctx context.Context, row repositories.PendingReminderRow) error
}

func NewCommercialService(
	db *pgxpool.Pool,
	commercialRepo *repositories.CommercialRepository,
	clientRepo *repositories.ClientRepository,
	notifier NotificationSender,
	audit *middleware.AuditService,
	logger *zap.Logger,
) *CommercialService {
	return &CommercialService{
		db:             db,
		commercialRepo: commercialRepo,
		clientRepo:     clientRepo,
		notifier:       notifier,
		audit:          audit,
		logger:         logger,
	}
}

// ─── Create ────────────────────────────────────────────────────────────────────

// CreateCommercialDetails is called when a commercial client is created.
// It validates required fields and inserts the details row.
// Must be called within the same DB transaction as the client INSERT.
func (s *CommercialService) CreateCommercialDetails(
	ctx context.Context,
	clientID uuid.UUID,
	req *dto.CreateCommercialDetailsRequest,
	createdBy uuid.UUID,
) (*models.CommercialClientDetails, error) {

	// Validate crew member required when lead_source = crew_member
	if req.LeadSource == models.LeadSourceCrewMember && req.CrewMemberID == nil {
		return nil, fmt.Errorf("crew_member_id is required when lead_source is crew_member")
	}

	// Validate inspector exists and is an inspector
	if err := s.validateInspector(ctx, req.InspectorID); err != nil {
		return nil, err
	}

	details := &models.CommercialClientDetails{
		ClientID:             clientID,
		WorkflowStatus:       models.CommercialStatusAssigned, // always starts as assigned
		LeadSource:           req.LeadSource,
		CrewMemberID:         req.CrewMemberID,
		InspectorID:          req.InspectorID,
		CompanyName:          req.CompanyName,
		ContactPersonName:    req.ContactPersonName,
		ServiceAddress:       req.ServiceAddress,
		BillingAddress:       req.BillingAddress,
		BillingSameAsService: req.BillingSameAsService,
		BillingTerms:         req.BillingTerms,
		PhoneNumber:          req.PhoneNumber,
		Email:                req.Email,
		Notes:                req.Notes,
	}

	// Use a transaction (caller must provide one via the tx-aware create path)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	created, err := s.commercialRepo.CreateDetails(ctx, tx, details)
	if err != nil {
		return nil, err
	}

	// Record initial assignment in history table
	if _, err = tx.Exec(ctx, `
		INSERT INTO commercial_inspector_assignments
			(client_id, inspector_id, assigned_by)
		VALUES ($1, $2, $3)
	`, clientID, req.InspectorID, createdBy); err != nil {
		return nil, fmt.Errorf("record initial inspector assignment: %w", err)
	}

	// Record initial status transition (null -> assigned)
	if _, err = tx.Exec(ctx, `
		INSERT INTO commercial_status_transitions
			(client_id, from_status, to_status, changed_by)
		VALUES ($1, NULL, 'assigned', $2)
	`, clientID, createdBy); err != nil {
		return nil, fmt.Errorf("record initial status transition: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return created, nil
}

// ─── Status Transition ────────────────────────────────────────────────────────

// TransitionStatus validates and executes a commercial workflow status change.
// This is the most critical method in this service.
func (s *CommercialService) TransitionStatus(
	ctx context.Context,
	clientID uuid.UUID,
	req *dto.CommercialStatusTransitionRequest,
	changedBy uuid.UUID,
	ipAddress, userAgent string,
) (*models.CommercialClientDetails, error) {

	// 1. Fetch current details
	details, err := s.commercialRepo.GetDetailsByClientID(ctx, clientID)
	if err != nil {
		return nil, err
	}

	from := details.WorkflowStatus
	to := req.ToStatus

	// 2. Validate transition is allowed
	if !models.IsTransitionAllowed(from, to) {
		return nil, fmt.Errorf(
			"transition from %q to %q is not allowed",
			from, to,
		)
	}

	// 3. Validate proposal link (required for pending/approved/declined)
	if models.StatusRequiresProposalLink(to) {
		if req.ProposalDriveLink == nil || *req.ProposalDriveLink == "" {
			return nil, fmt.Errorf("proposal_drive_link is required when transitioning to %q", to)
		}
	}

	// 4. Per-status validation
	updates := map[string]interface{}{
		"workflow_status": to,
	}

	if req.ProposalDriveLink != nil {
		updates["proposal_drive_link"] = *req.ProposalDriveLink
	}

	switch to {
	case models.CommercialStatusPending:
		if err := s.validatePendingTransition(req, updates); err != nil {
			return nil, err
		}

	case models.CommercialStatusApproved:
		if err := s.validateApprovedTransition(req, details, updates); err != nil {
			return nil, err
		}

	case models.CommercialStatusInstalled:
		if err := s.validateInstalledTransition(req, updates); err != nil {
			return nil, err
		}

	case models.CommercialStatusCancelled:
		if err := s.validateCancelledTransition(req, updates); err != nil {
			return nil, err
		}
	}

	// 5. Apply all updates atomically
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Build SET clause from updates map
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

	if _, err = tx.Exec(ctx, fmt.Sprintf(`
		UPDATE commercial_client_details
		SET %s, updated_at = NOW()
		WHERE client_id = $%d
	`, setClauses, i), args...); err != nil {
		return nil, fmt.Errorf("update details on transition: %w", err)
	}

	// 6. Record transition in history
	if _, err = tx.Exec(ctx, `
		INSERT INTO commercial_status_transitions
			(client_id, from_status, to_status, changed_by, proposal_link, notes)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, clientID, from, to, changedBy, req.ProposalDriveLink, req.TransitionNotes); err != nil {
		return nil, fmt.Errorf("record status transition: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// 7. Post-transition side effects (async, non-blocking)
	updatedDetails, _ := s.commercialRepo.GetDetailsByClientID(ctx, clientID)

	go func() {
		bgCtx := context.Background()
		switch to {
		case models.CommercialStatusApproved:
			recipients, err := s.commercialRepo.GetRecipientsByEvent(bgCtx, "commercial_approved")
			if err != nil {
				s.logger.Error("failed to get approval recipients", zap.Error(err))
				return
			}
			if err := s.notifier.SendCommercialApproved(bgCtx, updatedDetails, recipients); err != nil {
				s.logger.Error("failed to send approval notification", zap.Error(err))
			}
		}
	}()

	// 8. Audit log
	ip := &ipAddress
	ua := &userAgent
	s.audit.Log(&changedBy, "update", "commercial_workflow", clientID,
		map[string]interface{}{"workflow_status": from},
		map[string]interface{}{"workflow_status": to},
		ip, ua,
	)

	return updatedDetails, nil
}

// ─── Per-status validation helpers ────────────────────────────────────────────

func (s *CommercialService) validatePendingTransition(
	req *dto.CommercialStatusTransitionRequest,
	updates map[string]interface{},
) error {
	if req.NextFollowupDate == nil || *req.NextFollowupDate == "" {
		return fmt.Errorf("next_followup_date is required when transitioning to pending")
	}
	d, err := time.Parse(time.DateOnly, *req.NextFollowupDate)
	if err != nil {
		return fmt.Errorf("invalid next_followup_date: %w", err)
	}
	if d.Before(time.Now().Truncate(24 * time.Hour)) {
		return fmt.Errorf("next_followup_date must be today or in the future")
	}
	updates["next_followup_date"] = d
	return nil
}

func (s *CommercialService) validateApprovedTransition(
	req *dto.CommercialStatusTransitionRequest,
	details *models.CommercialClientDetails,
	updates map[string]interface{},
) error {
	// Required business fields
	errs := []string{}

	if req.ApprovedByName == nil || *req.ApprovedByName == "" {
		errs = append(errs, "approved_by_name")
	}
	if req.ApprovedDate == nil || *req.ApprovedDate == "" {
		errs = append(errs, "approved_date")
	}
	if req.InitialSetupCost == nil {
		errs = append(errs, "initial_setup_cost")
	}
	if req.RecurringServiceCost == nil {
		errs = append(errs, "recurring_service_cost")
	}
	if req.ServiceFrequency == nil {
		errs = append(errs, "service_frequency")
	}
	if req.BillingTerms == nil {
		errs = append(errs, "billing_terms")
	}

	// Check required business details are populated (either previously or now)
	if details.CompanyName == nil && req.ApprovedByName == nil {
		errs = append(errs, "company_name (must be set before approval)")
	}
	if details.ServiceAddress == nil {
		errs = append(errs, "service_address (must be set before approval)")
	}

	if len(errs) > 0 {
		return fmt.Errorf("approval requires the following fields: %v", errs)
	}

	// Parse approved date
	d, err := time.Parse(time.DateOnly, *req.ApprovedDate)
	if err != nil {
		return fmt.Errorf("invalid approved_date: %w", err)
	}

	// Validate frequency_interval only for daily/weekly/monthly
	if req.FrequencyInterval != nil {
		if !models.FrequencySupportsInterval(*req.ServiceFrequency) {
			return fmt.Errorf("frequency_interval is only valid for daily, weekly, or monthly frequencies")
		}
		if *req.FrequencyInterval <= 0 {
			return fmt.Errorf("frequency_interval must be greater than 0")
		}
	}

	updates["approved_by_name"] = *req.ApprovedByName
	updates["approved_date"] = d
	updates["initial_setup_cost"] = *req.InitialSetupCost
	updates["recurring_service_cost"] = *req.RecurringServiceCost
	updates["service_frequency"] = *req.ServiceFrequency
	updates["billing_terms"] = *req.BillingTerms
	if req.FrequencyInterval != nil {
		updates["frequency_interval"] = *req.FrequencyInterval
	}
	return nil
}

func (s *CommercialService) validateInstalledTransition(
	req *dto.CommercialStatusTransitionRequest,
	updates map[string]interface{},
) error {
	if req.InstallationDate == nil || *req.InstallationDate == "" {
		return fmt.Errorf("installation_date is required when transitioning to installed")
	}
	d, err := time.Parse(time.DateOnly, *req.InstallationDate)
	if err != nil {
		return fmt.Errorf("invalid installation_date: %w", err)
	}
	updates["installation_date"] = d
	if req.InstallationNotes != nil {
		updates["installation_notes"] = *req.InstallationNotes
	}
	return nil
}

func (s *CommercialService) validateCancelledTransition(
	req *dto.CommercialStatusTransitionRequest,
	updates map[string]interface{},
) error {
	if req.CancelledDate == nil || *req.CancelledDate == "" {
		return fmt.Errorf("cancelled_date is required when transitioning to cancelled")
	}
	d, err := time.Parse(time.DateOnly, *req.CancelledDate)
	if err != nil {
		return fmt.Errorf("invalid cancelled_date: %w", err)
	}
	updates["cancelled_date"] = d
	if req.CancelReason != nil {
		updates["cancel_reason"] = *req.CancelReason
	}
	return nil
}

// ─── Inspector Reassignment ───────────────────────────────────────────────────

// ReassignInspector changes the inspector for a commercial client.
func (s *CommercialService) ReassignInspector(
	ctx context.Context,
	clientID uuid.UUID,
	req *dto.ReassignInspectorRequest,
	changedBy uuid.UUID,
	ipAddress, userAgent string,
) error {
	if err := s.validateInspector(ctx, req.InspectorID); err != nil {
		return err
	}

	if err := s.commercialRepo.RecordInspectorAssignment(
		ctx, clientID, req.InspectorID, &changedBy, req.Notes,
	); err != nil {
		return err
	}

	ip := &ipAddress
	ua := &userAgent
	s.audit.Log(&changedBy, "update", "inspector_assignment", clientID, nil,
		map[string]interface{}{"inspector_id": req.InspectorID}, ip, ua)

	return nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (s *CommercialService) validateInspector(ctx context.Context, inspectorID uuid.UUID) error {
	var isInspector bool
	err := s.db.QueryRow(ctx,
		`SELECT is_inspector FROM users WHERE id = $1 AND is_active = true`,
		inspectorID,
	).Scan(&isInspector)
	if err != nil {
		return fmt.Errorf("inspector user not found: %w", err)
	}
	if !isInspector {
		return fmt.Errorf("user %s is not designated as an inspector", inspectorID)
	}
	return nil
}

// GetDashboardAlerts returns commercial clients in pending status with
// upcoming follow-up dates (for the dashboard).
func (s *CommercialService) GetDashboardAlerts(ctx context.Context) ([]dto.CommercialDashboardAlert, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			ccd.client_id::TEXT,
			COALESCE(ccd.company_name, c.client_name, 'Unknown'),
			ccd.next_followup_date,
			COALESCE(u.full_name, u.email),
			(ccd.next_followup_date - CURRENT_DATE) AS days_until
		FROM commercial_client_details ccd
		JOIN clients c ON c.id = ccd.client_id
		JOIN users u   ON u.id = ccd.inspector_id
		WHERE
			ccd.workflow_status = 'pending'
			AND ccd.next_followup_date IS NOT NULL
			AND ccd.next_followup_date >= CURRENT_DATE
		ORDER BY ccd.next_followup_date ASC
		LIMIT 50
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	alerts := []dto.CommercialDashboardAlert{}
	for rows.Next() {
		a := dto.CommercialDashboardAlert{}
		var nextDate time.Time
		rows.Scan(
			&a.ClientID, &a.CompanyName, &nextDate,
			&a.InspectorName, &a.DaysUntilDue,
		)
		a.NextFollowupDate = nextDate.Format(time.DateOnly)
		alerts = append(alerts, a)
	}
	return alerts, nil
}
