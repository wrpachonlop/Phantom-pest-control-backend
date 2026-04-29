package services

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/phantompestcontrol/crm/internal/dto"
	"github.com/phantompestcontrol/crm/internal/middleware"
	"github.com/phantompestcontrol/crm/internal/models"
	"github.com/phantompestcontrol/crm/internal/repositories"
	"go.uber.org/zap"
)

// ClientService orchestrates client business logic.
type ClientService struct {
	clientRepo *repositories.ClientRepository
	audit      *middleware.AuditService
	logger     *zap.Logger
}

func NewClientService(
	clientRepo *repositories.ClientRepository,
	audit *middleware.AuditService,
	logger *zap.Logger,
) *ClientService {
	return &ClientService{
		clientRepo: clientRepo,
		audit:      audit,
		logger:     logger,
	}
}

func (s *ClientService) List(
	ctx context.Context,
	req *dto.ClientListRequest,
) ([]*models.Client, int64, error) { // <--- Cambia ClientFull por Client aquí

	// El repositorio ya devuelve []*models.Client, así que esto ahora encajará perfecto
	return s.clientRepo.List(ctx, req)
}

// GetByID retrieves a single client with all its related data (Full view).
func (s *ClientService) GetByID(ctx context.Context, id uuid.UUID) (*models.ClientFull, error) {
	// El Service simplemente actúa como mediador con el Repository
	client, err := s.clientRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("service get client by id: %w", err)
	}

	// Aquí podrías añadir lógica extra en el futuro,
	// como verificar si el usuario tiene permiso para ver este cliente específico.

	return client, nil
}

// Create validates and creates a new client.
func (s *ClientService) Create(
	ctx context.Context,
	req *dto.CreateClientRequest,
	createdBy uuid.UUID,
	ipAddress, userAgent string,
) (*models.ClientFull, error) {

	loc, _ := time.LoadLocation("America/Vancouver")

	// Parse contact date
	contactDate, err := time.ParseInLocation(time.DateOnly, req.ClientContactDate, loc)
	if err != nil {
		return nil, fmt.Errorf("invalid client_contact_date: %w", err)
	}

	// Validate status
	status := models.ClientStatusBlue
	if req.Status != "" {
		status = req.Status
	}

	// Parse sold date
	var soldDate *time.Time
	if req.SoldDate != nil {
		d, err := time.ParseInLocation(time.DateOnly, *req.SoldDate, loc)
		if err != nil {
			return nil, fmt.Errorf("invalid sold_date: %w", err)
		}
		soldDate = &d
	}

	// Validate: if status = green, sold_date is required
	if status == models.ClientStatusGreen && soldDate == nil {
		return nil, fmt.Errorf("sold_date is required when status is 'green'")
	}

	// Normalize phones
	phones := make([]models.Phone, 0, len(req.Phones))
	for _, p := range req.Phones {
		if strings.TrimSpace(p.PhoneNumber) == "" {
			continue
		}
		normalized, err := normalizePhone(p.PhoneNumber)
		if err != nil {
			return nil, fmt.Errorf("invalid phone number %q: %w", p.PhoneNumber, err)
		}
		label := p.Label
		if label == "" {
			label = "primary"
		}
		phones = append(phones, models.Phone{
			PhoneNumber: normalized,
			Label:       label,
		})
	}

	// Validate emails
	emails := make([]models.Email, 0, len(req.Emails))
	for _, e := range req.Emails {
		if !isValidEmail(e.Email) {
			return nil, fmt.Errorf("invalid email %q", e.Email)
		}
		label := e.Label
		if label == "" {
			label = "primary"
		}
		emails = append(emails, models.Email{
			Email: strings.ToLower(e.Email),
			Label: label,
		})
	}

	client := &models.Client{
		ClientName:         req.ClientName,
		ClientType:         req.ClientType,
		PropertyType:       req.PropertyType,
		Status:             status,
		ClientContactDate:  contactDate,
		AfterHours:         req.AfterHours,
		ContactMethodID:    req.ContactMethodID,
		ProblemDescription: req.ProblemDescription,
		LocationType:       req.LocationType,
		LocationValue:      req.LocationValue,
		SaleRange:          req.SaleRange,
		SoldDate:           soldDate,
		CreatedBy:          createdBy,
	}

	created, err := s.clientRepo.Create(ctx, client, phones, emails, req.PestIssues)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	// Write audit log
	ip := &ipAddress
	ua := &userAgent
	s.audit.Log(&createdBy, "create", "client", created.ID, nil, created, ip, ua)

	// Load full client for response
	return s.clientRepo.GetByID(ctx, created.ID)
}

// Update applies partial updates to a client with full validation.
func (s *ClientService) Update(
	ctx context.Context,
	id uuid.UUID,
	req *dto.UpdateClientRequest,
	updatedBy uuid.UUID,
	ipAddress, userAgent string,
) (*models.Client, error) {
	loc, _ := time.LoadLocation("America/Vancouver")

	// Capture old state for audit
	oldSnapshot, _ := s.clientRepo.GetSnapshot(ctx, id)

	updates := map[string]interface{}{}

	if req.ClientName != nil {
		updates["client_name"] = *req.ClientName
	}
	if req.ClientType != nil {
		updates["client_type"] = *req.ClientType
	}
	if req.PropertyType != nil {
		updates["property_type"] = *req.PropertyType
	}
	if req.AfterHours != nil {
		updates["after_hours"] = *req.AfterHours
	}
	if req.ContactMethodID != nil {
		updates["contact_method_id"] = *req.ContactMethodID
	}
	if req.ProblemDescription != nil {
		updates["problem_description"] = *req.ProblemDescription
	}
	if req.LocationType != nil {
		updates["location_type"] = *req.LocationType
	}
	if req.LocationValue != nil {
		updates["location_value"] = *req.LocationValue
	}
	if req.SaleRange != nil {
		updates["sale_range"] = *req.SaleRange
	}
	if req.SoldBy != nil {
		updates["sold_by"] = *req.SoldBy
	}

	// Status transition logic
	if req.Status != nil {
		newStatus := *req.Status

		// If setting to green, sold_date becomes required
		if newStatus == models.ClientStatusGreen {
			if req.SoldDate == nil {
				return nil, fmt.Errorf("sold_date is required when setting status to 'green'")
			}
			d, err := time.ParseInLocation(time.DateOnly, *req.SoldDate, loc)
			if err != nil {
				return nil, fmt.Errorf("invalid sold_date: %w", err)
			}
			updates["sold_date"] = d
		}

		updates["status"] = newStatus
	}

	if req.ClientContactDate != nil {
		d, err := time.ParseInLocation(time.DateOnly, *req.ClientContactDate, loc)
		if err != nil {
			return nil, fmt.Errorf("invalid client_contact_date: %w", err)
		}
		updates["client_contact_date"] = d
	}

	if req.PestIssues != nil {
		updates["pest_issues"] = *req.PestIssues
	}
	if req.Phones != nil {
		updates["phones"] = *req.Phones
	}
	if req.Emails != nil {
		updates["emails"] = *req.Emails
	}

	updated, err := s.clientRepo.Update(
		ctx,
		id,
		updates,
		req.Phones,
		req.Emails,
		req.PestIssues,
	)
	if err != nil {
		return nil, err
	}

	// Write audit
	ip := &ipAddress
	ua := &userAgent
	s.audit.Log(&updatedBy, "update", "client", id, oldSnapshot, updates, ip, ua)

	return updated, nil
}

// Delete removes a client (admin only).
func (s *ClientService) Delete(
	ctx context.Context,
	id uuid.UUID,
	deletedBy uuid.UUID,
	ipAddress, userAgent string,
) error {
	// Capture snapshot before delete
	snapshot, _ := s.clientRepo.GetSnapshot(ctx, id)

	if err := s.clientRepo.Delete(ctx, id); err != nil {
		return err
	}

	ip := &ipAddress
	ua := &userAgent
	s.audit.Log(&deletedBy, "delete", "client", id, snapshot, nil, ip, ua)
	return nil
}

// CheckDuplicates performs duplicate detection.
func (s *ClientService) CheckDuplicates(
	ctx context.Context,
	req *dto.DuplicateCheckRequest,
) (*models.DuplicateCheckResult, error) {
	normalizedPhones := make([]string, 0, len(req.Phones))
	for _, p := range req.Phones {
		n, err := normalizePhone(p)
		if err == nil {
			normalizedPhones = append(normalizedPhones, n)
		}
	}

	normalizedEmails := make([]string, 0, len(req.Emails))
	for _, e := range req.Emails {
		normalizedEmails = append(normalizedEmails, strings.ToLower(e))
	}

	clients, matchOn, err := s.clientRepo.CheckDuplicates(ctx, normalizedPhones, normalizedEmails)
	if err != nil {
		return nil, err
	}

	return &models.DuplicateCheckResult{
		Found:   len(clients) > 0,
		Clients: clients,
		MatchOn: matchOn,
	}, nil
}

// GetAuditLog retrieves the audit history for a client.
func (s *ClientService) GetAuditLog(ctx context.Context, id uuid.UUID) ([]map[string]interface{}, error) {
	// Aquí podrías validar si el usuario actual tiene permisos de "Admin"
	// para ver logs, pero por ahora lo dejamos abierto para que pruebes.
	return s.clientRepo.GetAuditLog(ctx, id)
}

// ─── Helpers ─────────────────────────────────────────────────

// normalizePhone converts a phone number to E.164 format (+1XXXXXXXXXX for CA/US).
// This is basic normalization; for production consider libphonenumber bindings.
func normalizePhone(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	// Strip everything except digits and leading +
	var sb strings.Builder
	for i, ch := range raw {
		if i == 0 && ch == '+' {
			sb.WriteRune(ch)
			continue
		}
		if unicode.IsDigit(ch) {
			sb.WriteRune(ch)
		}
	}
	digits := sb.String()

	// Remove leading + for counting
	numericPart := strings.TrimPrefix(digits, "+")

	switch {
	case len(numericPart) == 10: // Local NANP: assume +1
		return "+1" + numericPart, nil
	case len(numericPart) == 11 && numericPart[0] == '1': // Already +1 prefix
		return "+" + numericPart, nil
	case len(numericPart) >= 7:
		return "+" + numericPart, nil
	default:
		return "", fmt.Errorf("phone number too short after normalization: %q", raw)
	}
}

func isValidEmail(email string) bool {
	parts := strings.Split(email, "@")
	return len(parts) == 2 && len(parts[0]) > 0 && strings.Contains(parts[1], ".")
}
