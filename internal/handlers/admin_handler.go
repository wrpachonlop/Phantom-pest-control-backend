package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/phantompestcontrol/crm/internal/dto"
	"github.com/phantompestcontrol/crm/internal/middleware"
	"github.com/phantompestcontrol/crm/internal/models"
	"github.com/phantompestcontrol/crm/internal/services"
	"go.uber.org/zap"
)

// AdminHandler handles dynamic lookup table management and user admin.

type AdminHandler struct {
	db           *pgxpool.Pool
	audit        *middleware.AuditService
	logger       *zap.Logger
	reminderCron *services.ReminderCron
}

func NewAdminHandler(db *pgxpool.Pool, audit *middleware.AuditService, logger *zap.Logger, cron *services.ReminderCron) *AdminHandler {
	return &AdminHandler{db: db, audit: audit, logger: logger, reminderCron: cron}
}

// ──────────────────────────────────────────
// CONTACT METHODS
// ──────────────────────────────────────────

// ListContactMethods GET /contact-methods
func (h *AdminHandler) ListContactMethods(c *gin.Context) {
	rows, err := h.db.Query(c.Request.Context(),
		`SELECT id, name, is_active, created_at FROM contact_methods ORDER BY name`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to fetch contact methods"})
		return
	}
	defer rows.Close()

	methods := []*models.ContactMethod{}
	for rows.Next() {
		m := &models.ContactMethod{}
		rows.Scan(&m.ID, &m.Name, &m.IsActive, &m.CreatedAt)
		methods = append(methods, m)
	}
	c.JSON(http.StatusOK, gin.H{"data": methods})
}

// CreateContactMethod POST /contact-methods
func (h *AdminHandler) CreateContactMethod(c *gin.Context) {
	req := &dto.CreateContactMethodRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	m := &models.ContactMethod{}
	err := h.db.QueryRow(c.Request.Context(),
		`INSERT INTO contact_methods (name) VALUES ($1) RETURNING id, name, is_active, created_at`,
		req.Name,
	).Scan(&m.ID, &m.Name, &m.IsActive, &m.CreatedAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: fmt.Sprintf("contact method already exists or invalid: %v", err)})
		return
	}

	userID := middleware.GetUserID(c)
	h.audit.Log(&userID, "create", "contact_method", m.ID, nil, m, strPtr(c.ClientIP()), strPtr(c.Request.UserAgent()))
	c.JSON(http.StatusCreated, m)
}

// UpdateContactMethod PUT /contact-methods/:id
func (h *AdminHandler) UpdateContactMethod(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}
	req := &dto.UpdateContactMethodRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	m := &models.ContactMethod{}
	err = h.db.QueryRow(c.Request.Context(), `
		UPDATE contact_methods
		SET
			name      = COALESCE($2, name),
			is_active = COALESCE($3, is_active)
		WHERE id = $1
		RETURNING id, name, is_active, created_at
	`, id, req.Name, req.IsActive,
	).Scan(&m.ID, &m.Name, &m.IsActive, &m.CreatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "contact method not found"})
		return
	}

	userID := middleware.GetUserID(c)
	h.audit.Log(&userID, "update", "contact_method", m.ID, nil, m, strPtr(c.ClientIP()), strPtr(c.Request.UserAgent()))
	c.JSON(http.StatusOK, m)
}

// ──────────────────────────────────────────
// PEST ISSUES
// ──────────────────────────────────────────

// ListPestIssues GET /pest-issues
func (h *AdminHandler) ListPestIssues(c *gin.Context) {
	rows, err := h.db.Query(c.Request.Context(),
		`SELECT id, name, is_active, created_at FROM pest_issues ORDER BY name`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to fetch pest issues"})
		return
	}
	defer rows.Close()

	issues := []*models.PestIssue{}
	for rows.Next() {
		pi := &models.PestIssue{}
		rows.Scan(&pi.ID, &pi.Name, &pi.IsActive, &pi.CreatedAt)
		issues = append(issues, pi)
	}
	c.JSON(http.StatusOK, gin.H{"data": issues})
}

// CreatePestIssue POST /pest-issues
func (h *AdminHandler) CreatePestIssue(c *gin.Context) {
	req := &dto.CreatePestIssueRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	pi := &models.PestIssue{}
	err := h.db.QueryRow(c.Request.Context(),
		`INSERT INTO pest_issues (name) VALUES ($1) RETURNING id, name, is_active, created_at`,
		req.Name,
	).Scan(&pi.ID, &pi.Name, &pi.IsActive, &pi.CreatedAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "pest issue already exists or invalid"})
		return
	}

	userID := middleware.GetUserID(c)
	h.audit.Log(&userID, "create", "pest_issue", pi.ID, nil, pi, strPtr(c.ClientIP()), strPtr(c.Request.UserAgent()))
	c.JSON(http.StatusCreated, pi)
}

// UpdatePestIssue PUT /pest-issues/:id
func (h *AdminHandler) UpdatePestIssue(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}
	req := &dto.UpdatePestIssueRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	pi := &models.PestIssue{}
	err = h.db.QueryRow(c.Request.Context(), `
		UPDATE pest_issues
		SET
			name      = COALESCE($2, name),
			is_active = COALESCE($3, is_active)
		WHERE id = $1
		RETURNING id, name, is_active, created_at
	`, id, req.Name, req.IsActive,
	).Scan(&pi.ID, &pi.Name, &pi.IsActive, &pi.CreatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "pest issue not found"})
		return
	}

	userID := middleware.GetUserID(c)
	h.audit.Log(&userID, "update", "pest_issue", pi.ID, nil, pi, strPtr(c.ClientIP()), strPtr(c.Request.UserAgent()))
	c.JSON(http.StatusOK, pi)
}

// ──────────────────────────────────────────
// USERS (Admin panel)
// ──────────────────────────────────────────

// ListUsers GET /admin/users
func (h *AdminHandler) ListUsers(c *gin.Context) {
	rows, err := h.db.Query(c.Request.Context(),
		`SELECT id, email, full_name, role, is_active, created_at, updated_at FROM users ORDER BY created_at DESC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to fetch users"})
		return
	}
	defer rows.Close()

	users := []*models.User{}
	for rows.Next() {
		u := &models.User{}
		rows.Scan(&u.ID, &u.Email, &u.FullName, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
		users = append(users, u)
	}
	c.JSON(http.StatusOK, gin.H{"data": users})
}

// UpdateUserRole PUT /admin/users/:id/role
func (h *AdminHandler) UpdateUserRole(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	var req struct {
		Role models.UserRole `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	if req.Role != models.UserRoleAdmin && req.Role != models.UserRoleUser {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "role must be 'admin' or 'user'"})
		return
	}

	ctx := c.Request.Context()
	_, err = h.db.Exec(ctx,
		`UPDATE users SET role = $2, updated_at = NOW() WHERE id = $1`, id, req.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to update role"})
		return
	}

	userID := middleware.GetUserID(c)
	h.audit.Log(&userID, "update", "user_role", id, nil, map[string]interface{}{"role": req.Role},
		strPtr(c.ClientIP()), strPtr(c.Request.UserAgent()))

	c.JSON(http.StatusOK, dto.SuccessResponse{Message: "role updated"})
}

// ──────────────────────────────────────────
// AUDIT LOG
// ──────────────────────────────────────────

// GetAuditLog GET /audit-logs  (admin)
func (h *AdminHandler) GetAuditLog(c *gin.Context) {
	entityType := c.Query("entity_type")
	entityIDStr := c.Query("entity_id")

	query := `
		SELECT al.id, al.user_id, al.action, al.entity_type, al.entity_id,
			   al.old_values, al.new_values, al.ip_address, al.user_agent, al.created_at,
			   u.full_name, u.email
		FROM audit_logs al
		LEFT JOIN users u ON u.id = al.user_id
		WHERE 1=1
	`
	args := []interface{}{}
	argIdx := 1

	if entityType != "" {
		query += fmt.Sprintf(" AND al.entity_type = $%d", argIdx)
		args = append(args, entityType)
		argIdx++
	}
	if entityIDStr != "" {
		entityID, err := uuid.Parse(entityIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid entity_id"})
			return
		}
		query += fmt.Sprintf(" AND al.entity_id = $%d", argIdx)
		args = append(args, entityID)
		argIdx++
	}

	query += " ORDER BY al.created_at DESC LIMIT 200"

	rows, err := h.db.Query(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to fetch audit logs"})
		return
	}
	defer rows.Close()

	type AuditRow struct {
		models.AuditLog
		UserFullName *string `json:"user_full_name"`
		UserEmail    *string `json:"user_email"`
	}

	logs := []AuditRow{}
	for rows.Next() {
		var row AuditRow
		var ipStr, uaStr *string
		rows.Scan(
			&row.ID, &row.UserID, &row.Action, &row.EntityType, &row.EntityID,
			&row.OldValues, &row.NewValues, &ipStr, &uaStr, &row.CreatedAt,
			&row.UserFullName, &row.UserEmail,
		)
		row.IPAddress = ipStr
		row.UserAgent = uaStr
		logs = append(logs, row)
	}

	c.JSON(http.StatusOK, gin.H{"data": logs})
}

// ──────────────────────────────────────────
// NOTES
// ──────────────────────────────────────────

// CreateNote POST /notes
func (h *AdminHandler) CreateNote(c *gin.Context) {
	req := &dto.CreateNoteRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	userID := middleware.GetUserID(c)
	note := &models.Note{}
	err := h.db.QueryRow(c.Request.Context(), `
		INSERT INTO notes (client_id, user_id, note)
		VALUES ($1, $2, $3)
		RETURNING id, client_id, user_id, note, created_at
	`, req.ClientID, userID, req.Note,
	).Scan(&note.ID, &note.ClientID, &note.UserID, &note.Note, &note.CreatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to create note"})
		return
	}

	c.JSON(http.StatusCreated, note)
}

// GetNotesByClient GET /clients/:id/notes
func (h *AdminHandler) GetNotesByClient(c *gin.Context) {
	clientID, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT n.id, n.client_id, n.user_id, n.note, n.created_at,
			   u.full_name, u.email
		FROM notes n
		JOIN users u ON u.id = n.user_id
		WHERE n.client_id = $1
		ORDER BY n.created_at DESC
	`, clientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to fetch notes"})
		return
	}
	defer rows.Close()

	type NoteRow struct {
		models.Note
		UserFullName *string `json:"user_full_name"`
		UserEmail    string  `json:"user_email"`
	}

	notes := []NoteRow{}
	for rows.Next() {
		var n NoteRow
		rows.Scan(
			&n.ID, &n.ClientID, &n.UserID, &n.Note, &n.CreatedAt,
			&n.UserFullName, &n.UserEmail,
		)
		notes = append(notes, n)
	}

	c.JSON(http.StatusOK, gin.H{"data": notes})
}

// DeleteNote DELETE /notes/:id  (admin only)
func (h *AdminHandler) DeleteNote(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	ctx := context.Background()
	var note models.Note
	err = h.db.QueryRow(ctx,
		`SELECT id, client_id, user_id, note FROM notes WHERE id = $1`, id,
	).Scan(&note.ID, &note.ClientID, &note.UserID, &note.Note)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "note not found"})
		return
	}

	h.db.Exec(ctx, `DELETE FROM notes WHERE id = $1`, id)

	userID := middleware.GetUserID(c)
	h.audit.Log(&userID, "delete", "note", id, note, nil,
		strPtr(c.ClientIP()), strPtr(c.Request.UserAgent()))

	c.JSON(http.StatusOK, dto.SuccessResponse{Message: "note deleted"})
}

// ──────────────────────────────────────────
// ME endpoint
// ──────────────────────────────────────────

// GetMe GET /me
func (h *AdminHandler) GetMe(c *gin.Context) {
	userID := middleware.GetUserID(c)
	u := &models.User{}
	err := h.db.QueryRow(c.Request.Context(),
		`SELECT id, email, full_name, role, avatar_url, is_active, created_at, updated_at FROM users WHERE id = $1`,
		userID,
	).Scan(&u.ID, &u.Email, &u.FullName, &u.Role, &u.AvatarURL, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "user not found"})
		return
	}
	c.JSON(http.StatusOK, u)
}

// UpsertMe is called on first login to sync Supabase user into our users table.
// POST /me/sync
func (h *AdminHandler) UpsertMe(c *gin.Context) {
	userID := middleware.GetUserID(c)
	email, _ := c.Get(middleware.ContextKeyEmail)

	var req struct {
		FullName  *string `json:"full_name"`
		AvatarURL *string `json:"avatar_url"`
	}
	c.ShouldBindJSON(&req)

	u := &models.User{}
	err := h.db.QueryRow(c.Request.Context(), `
		INSERT INTO users (id, email, full_name, avatar_url)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET
			email      = EXCLUDED.email,
			full_name  = COALESCE($3, users.full_name),
			avatar_url = COALESCE($4, users.avatar_url),
			updated_at = NOW()
		RETURNING id, email, full_name, role, avatar_url, is_active, created_at, updated_at
	`, userID, email, req.FullName, req.AvatarURL,
	).Scan(&u.ID, &u.Email, &u.FullName, &u.Role, &u.AvatarURL, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to sync user"})
		return
	}

	c.JSON(http.StatusOK, u)
}

// ──────────────────────────────────────────
// Health check
// ──────────────────────────────────────────

// HealthCheck GET /health
func (h *AdminHandler) HealthCheck(c *gin.Context) {
	ctx := c.Request.Context()
	dbOk := "ok"
	if err := h.db.Ping(ctx); err != nil {
		dbOk = "error: " + err.Error()
	}
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"database":  dbOk,
		"timestamp": time.Now().UTC(),
	})
}

func strPtr(s string) *string { return &s }

// SetInspectorFlag PUT /admin/users/:id/inspector
func (h *AdminHandler) SetInspectorFlag(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	var req struct {
		IsInspector bool `json:"is_inspector"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	_, err = h.db.Exec(c.Request.Context(),
		`UPDATE users SET is_inspector = $2, updated_at = NOW() WHERE id = $1`,
		id, req.IsInspector,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to update inspector flag"})
		return
	}

	userID := middleware.GetUserID(c)
	h.audit.Log(&userID, "update", "user_inspector", id, nil,
		map[string]interface{}{"is_inspector": req.IsInspector},
		strPtr(c.ClientIP()), strPtr(c.Request.UserAgent()),
	)

	c.JSON(http.StatusOK, dto.SuccessResponse{Message: "inspector flag updated"})
}

// CreateCrewMember POST /crew-members
func (h *AdminHandler) CreateCrewMember(c *gin.Context) {
	req := &dto.CreateCrewMemberRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	userID := middleware.GetUserID(c)
	var m models.CrewMember
	err := h.db.QueryRow(c.Request.Context(),
		`INSERT INTO crew_members (full_name, employee_id, created_by)
		 VALUES ($1, $2, $3)
		 RETURNING id, full_name, employee_id, is_active, created_by, created_at, updated_at`,
		req.FullName, req.EmployeeID, userID,
	).Scan(&m.ID, &m.FullName, &m.EmployeeID, &m.IsActive, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to create crew member"})
		return
	}
	c.JSON(http.StatusCreated, m)
}

// TriggerReminders POST /commercial/run-reminders  (admin, manual trigger for testing)
func (h *AdminHandler) TriggerReminders(c *gin.Context) {
	// reminderCron needs to be wired in — inject via AdminHandler or global ref
	h.reminderCron.RunNow(c.Request.Context())
	c.JSON(http.StatusOK, dto.SuccessResponse{Message: "reminder run triggered"})
}

// PUT /admin/crew-members/:id
func (h *AdminHandler) UpdateCrewMember(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		FullName string `json:"full_name"`
		IsActive bool   `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Actualizamos basándonos en el STEP 3 de tu SQL
	_, err := h.db.Exec(c.Request.Context(),
		"UPDATE crew_members SET full_name = $1, is_active = $2, updated_at = NOW() WHERE id = $3",
		req.FullName, req.IsActive, id)

	if err != nil {
		h.logger.Error("failed to update crew member", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update database"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Crew member updated successfully"})
}

// POST /admin/notification-recipients
func (h *AdminHandler) CreateNotificationRecipient(c *gin.Context) {
	var req struct {
		EventType string `json:"event_type"`
		Name      string `json:"name"`
		Email     string `json:"email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Insertamos basándonos en el STEP 6 de tu SQL
	_, err := h.db.Exec(c.Request.Context(),
		"INSERT INTO notification_recipients (event_type, name, email) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING",
		req.EventType, req.Name, req.Email)

	if err != nil {
		h.logger.Error("failed to create recipient", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Recipient created successfully"})
}

// DELETE /admin/notification-recipients/:id
func (h *AdminHandler) DeleteNotificationRecipient(c *gin.Context) {
	id := c.Param("id")

	_, err := h.db.Exec(c.Request.Context(), "DELETE FROM notification_recipients WHERE id = $1", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete recipient"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Recipient deleted"})
}

// GET /notification-recipients
func (h *AdminHandler) ListNotificationRecipients(c *gin.Context) {
	// Consultamos la tabla creada en el STEP 6 de tu migración
	rows, err := h.db.Query(c.Request.Context(),
		"SELECT id, event_type, name, email, is_active, created_at FROM notification_recipients ORDER BY event_type ASC")

	if err != nil {
		h.logger.Error("failed to list notification recipients", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	var recipients []any // Recomiendo crear un DTO para esto
	for rows.Next() {
		var r struct {
			ID        string `json:"id"`
			EventType string `json:"event_type"`
			Name      string `json:"name"`
			Email     string `json:"email"`
			IsActive  bool   `json:"is_active"`
			CreatedAt string `json:"created_at"`
		}
		if err := rows.Scan(&r.ID, &r.EventType, &r.Name, &r.Email, &r.IsActive, &r.CreatedAt); err != nil {
			continue
		}
		recipients = append(recipients, r)
	}

	c.JSON(http.StatusOK, recipients)
}
