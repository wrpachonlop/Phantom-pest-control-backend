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
	"go.uber.org/zap"
)

// FollowUpService handles follow-up business logic.
type FollowUpService struct {
	db     *pgxpool.Pool
	audit  *middleware.AuditService
	logger *zap.Logger
}

func NewFollowUpService(db *pgxpool.Pool, audit *middleware.AuditService, logger *zap.Logger) *FollowUpService {
	return &FollowUpService{db: db, audit: audit, logger: logger}
}

// Create creates a new follow-up.
// The DB trigger handles:
//   - first_contact_date auto-set
//   - status blue -> white transition
//
// This service handles:
//   - Business validation
//   - Sold follow-up requiring client status = green
func (s *FollowUpService) Create(
	ctx context.Context,
	req *dto.CreateFollowUpRequest,
	createdBy uuid.UUID,
	ipAddress, userAgent string,
) (*models.FollowUp, error) {
	loc, _ := time.LoadLocation("America/Vancouver")

	// Parse follow-up date
	date, err := time.ParseInLocation(time.DateOnly, req.Date, loc)
	if err != nil {
		return nil, fmt.Errorf("invalid date: %w", err)
	}

	// Validate: type "sold" should reflect client being green
	if req.Type == models.FollowUpTypeSold {
		var status string
		err := s.db.QueryRow(ctx,
			`SELECT status FROM clients WHERE id = $1`, req.ClientID,
		).Scan(&status)
		if err != nil {
			return nil, fmt.Errorf("client not found: %w", err)
		}
		if status != string(models.ClientStatusGreen) {
			return nil, fmt.Errorf("sold follow-up requires client status to be 'green'")
		}
	}

	fu := &models.FollowUp{}
	err = s.db.QueryRow(ctx, `
		INSERT INTO follow_ups (client_id, date, type, description, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, client_id, date, type, description, created_by, created_at
	`, req.ClientID, date, req.Type, req.Description, createdBy,
	).Scan(
		&fu.ID, &fu.ClientID, &fu.Date, &fu.Type,
		&fu.Description, &fu.CreatedBy, &fu.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create follow-up: %w", err)
	}

	ip := &ipAddress
	ua := &userAgent
	s.audit.Log(&createdBy, "create", "follow_up", fu.ID, nil, fu, ip, ua)

	return fu, nil
}

// GetByClient returns all follow-ups for a client ordered by date desc.
func (s *FollowUpService) GetByClient(ctx context.Context, clientID uuid.UUID) ([]*models.FollowUp, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			f.id, f.client_id, f.date, f.type, f.description, f.created_by, f.created_at,
			u.id, u.full_name, u.email
		FROM follow_ups f
		JOIN users u ON u.id = f.created_by
		WHERE f.client_id = $1
		ORDER BY f.date DESC, f.created_at DESC
	`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	followUps := []*models.FollowUp{}
	for rows.Next() {
		fu := &models.FollowUp{
			CreatedByUser: &models.User{},
		}
		rows.Scan(
			&fu.ID, &fu.ClientID, &fu.Date, &fu.Type,
			&fu.Description, &fu.CreatedBy, &fu.CreatedAt,
			&fu.CreatedByUser.ID, &fu.CreatedByUser.FullName, &fu.CreatedByUser.Email,
		)
		followUps = append(followUps, fu)
	}
	return followUps, nil
}

// Delete removes a follow-up (admin only).
func (s *FollowUpService) Delete(
	ctx context.Context,
	id uuid.UUID,
	deletedBy uuid.UUID,
	ipAddress, userAgent string,
) error {
	// Snapshot before delete
	fu := &models.FollowUp{}
	err := s.db.QueryRow(ctx,
		`SELECT id, client_id, date, type, description FROM follow_ups WHERE id = $1`, id,
	).Scan(&fu.ID, &fu.ClientID, &fu.Date, &fu.Type, &fu.Description)
	if err != nil {
		return fmt.Errorf("follow-up not found: %w", err)
	}

	_, err = s.db.Exec(ctx, `DELETE FROM follow_ups WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete follow-up: %w", err)
	}

	ip := &ipAddress
	ua := &userAgent
	s.audit.Log(&deletedBy, "delete", "follow_up", id, fu, nil, ip, ua)
	return nil
}

// Update modifies an existing follow-up. Only fields provided in the request will be updated.
func (s *FollowUpService) Update(
	ctx context.Context,
	id uuid.UUID,
	req *dto.UpdateFollowUpRequest,
	updatedBy uuid.UUID,
	ipAddress, userAgent string,
) (*models.FollowUp, error) {

	// 1. Obtener el registro actual para mantener los valores que no vienen en el request
	current := &models.FollowUp{}
	err := s.db.QueryRow(ctx,
		`SELECT date, type, description FROM follow_ups WHERE id = $1`, id,
	).Scan(&current.Date, &current.Type, &current.Description)
	if err != nil {
		return nil, fmt.Errorf("follow-up not found: %w", err)
	}

	// 2. Lógica de "Parche": Solo actualizar si el puntero no es nil
	loc, _ := time.LoadLocation("America/Vancouver")

	finalDate := current.Date
	if req.Date != nil {
		d, err := time.ParseInLocation(time.DateOnly, *req.Date, loc)
		if err != nil {
			return nil, fmt.Errorf("invalid date: %w", err)
		}
		finalDate = d
	}

	finalType := current.Type
	if req.Type != nil {
		finalType = *req.Type
	}

	finalDesc := current.Description
	if req.Description != nil {
		finalDesc = req.Description // Aquí pasamos el puntero directamente
	}

	// 3. Ejecutar el UPDATE
	fu := &models.FollowUp{}
	err = s.db.QueryRow(ctx, `
        UPDATE follow_ups 
        SET date = $1, type = $2, description = $3
        WHERE id = $4
        RETURNING id, client_id, date, type, description, created_by, created_at
    `, finalDate, finalType, finalDesc, id,
	).Scan(
		&fu.ID, &fu.ClientID, &fu.Date, &fu.Type,
		&fu.Description, &fu.CreatedBy, &fu.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("update follow-up: %w", err)
	}

	// Auditoría
	s.audit.Log(&updatedBy, "update", "follow_up", fu.ID, nil, nil, &ipAddress, &userAgent)

	return fu, nil
}
