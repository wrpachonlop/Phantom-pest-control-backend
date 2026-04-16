package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/phantompestcontrol/crm/internal/dto"
	"github.com/phantompestcontrol/crm/internal/middleware"
	"github.com/phantompestcontrol/crm/internal/services"
	"go.uber.org/zap"
)

const (
	IDPhoneCall    = "756de075-6e1d-48d5-8748-c732833d281b"
	IDText         = "76c97267-350d-4b82-897d-c39589c60d7e"
	IDEstimateForm = "acb52b34-f000-4bb9-8479-2cba40ed4706"
	IDMail         = "deb6c0f8-fa84-4550-bb8d-fe946718188f"
)

// ClientHandler handles HTTP requests for client endpoints.
type ClientHandler struct {
	clientService *services.ClientService
	logger        *zap.Logger
}

func NewClientHandler(clientService *services.ClientService, logger *zap.Logger) *ClientHandler {
	return &ClientHandler{clientService: clientService, logger: logger}
}

// List godoc
// GET /clients
func (h *ClientHandler) List(c *gin.Context) {
	req := &dto.ClientListRequest{
		Page:     1,
		PageSize: 25,
	}
	if err := c.ShouldBindQuery(req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	clients, total, err := h.clientService.List(c.Request.Context(), req)
	if err != nil {
		h.logger.Error("list clients", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to list clients"})
		return
	}

	totalPages := int(total) / req.PageSize
	if int(total)%req.PageSize != 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, dto.PaginatedResponse{
		Data:       clients,
		Total:      total,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: totalPages,
	})
}

// Get godoc
// GET /clients/:id
func (h *ClientHandler) Get(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	client, err := h.clientService.GetByID(c.Request.Context(), id)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, client)
}

// Create godoc
// POST /clients
func (h *ClientHandler) Create(c *gin.Context) {

	req := &dto.CreateClientRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	methodId := req.ContactMethodID.String()
	switch methodId {
	case IDPhoneCall, IDText:
		if len(req.Phones) == 0 || req.Phones[0].PhoneNumber == "" {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "phone number is required for phone calls or text messages"})
			return
		}
	case IDEstimateForm, IDMail:
		if len(req.Emails) == 0 || req.Emails[0].Email == "" {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "email is required for estimate forms or mail"})
			return
		}
		if !isValidEmail(req.Emails[0].Email) {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "the provided email format is invalid"})
			return
		}
	}
	isSpam := req.ClientType == "spam"
	isInitial := req.ClientType == "initial"

	if !isSpam && !isInitial {
		if len(req.PestIssues) == 0 {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "at least one pest issue must be selected"})
			return
		}
	}

	userID := middleware.GetUserID(c)
	client, err := h.clientService.Create(
		c.Request.Context(), req, userID,
		c.ClientIP(), c.Request.UserAgent(),
	)
	if err != nil {
		h.logger.Warn("create client failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, client)
}

func isValidEmail(s string) bool {
	parts := strings.Split(s, "@")
	return len(parts) == 2 && len(parts[0]) > 0 && strings.Contains(parts[1], ".")
}

// Update godoc
// PUT /clients/:id
func (h *ClientHandler) Update(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	req := &dto.UpdateClientRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	userID := middleware.GetUserID(c)
	client, err := h.clientService.Update(
		c.Request.Context(), id, req, userID,
		c.ClientIP(), c.Request.UserAgent(),
	)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, client)
}

// Delete godoc
// DELETE /clients/:id (admin only)
func (h *ClientHandler) Delete(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	userID := middleware.GetUserID(c)
	if err := h.clientService.Delete(
		c.Request.Context(), id, userID,
		c.ClientIP(), c.Request.UserAgent(),
	); err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.SuccessResponse{Message: "client deleted"})
}

// CheckDuplicates godoc
// POST /clients/check-duplicates
func (h *ClientHandler) CheckDuplicates(c *gin.Context) {
	req := &dto.DuplicateCheckRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	result, err := h.clientService.CheckDuplicates(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "duplicate check failed"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetAuditLog godoc
// GET /clients/:id/audit
func (h *ClientHandler) GetAuditLog(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	logs, err := h.clientService.GetAuditLog(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to fetch audit log"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": logs})
}

// ─── Helpers ─────────────────────────────────────────────────

func parseUUID(c *gin.Context, param string) (uuid.UUID, error) {
	raw := c.Param(param)
	id, err := uuid.Parse(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid UUID"})
		return uuid.Nil, err
	}
	return id, nil
}

func handleServiceError(c *gin.Context, err error) {
	// Map known sentinel errors to HTTP codes
	switch err {
	case nil:
		return
	default:
		// Could add typed errors here; for now, check string
		if err.Error() == "record not found" {
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "not found"})
		} else {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		}
	}
}
