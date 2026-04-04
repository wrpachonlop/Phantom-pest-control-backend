package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/phantompestcontrol/crm/internal/dto"
	"github.com/phantompestcontrol/crm/internal/middleware"
	"github.com/phantompestcontrol/crm/internal/services"
	"go.uber.org/zap"
)

// FollowUpHandler handles HTTP requests for follow-up endpoints.
type FollowUpHandler struct {
	followUpService *services.FollowUpService
	logger          *zap.Logger
}

func NewFollowUpHandler(followUpService *services.FollowUpService, logger *zap.Logger) *FollowUpHandler {
	return &FollowUpHandler{followUpService: followUpService, logger: logger}
}

// GetByClient godoc
// GET /clients/:id/follow-ups
func (h *FollowUpHandler) GetByClient(c *gin.Context) {
	clientID, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	followUps, err := h.followUpService.GetByClient(c.Request.Context(), clientID)
	if err != nil {
		h.logger.Error("get follow-ups", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to fetch follow-ups"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": followUps})
}

// Create godoc
// POST /follow-ups
func (h *FollowUpHandler) Create(c *gin.Context) {
	req := &dto.CreateFollowUpRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	userID := middleware.GetUserID(c)
	fu, err := h.followUpService.Create(
		c.Request.Context(), req, userID,
		c.ClientIP(), c.Request.UserAgent(),
	)
	if err != nil {
		h.logger.Warn("create follow-up failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, fu)
}

// Delete godoc
// DELETE /follow-ups/:id  (admin only)
func (h *FollowUpHandler) Delete(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	userID := middleware.GetUserID(c)
	if err := h.followUpService.Delete(
		c.Request.Context(), id, userID,
		c.ClientIP(), c.Request.UserAgent(),
	); err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.SuccessResponse{Message: "follow-up deleted"})
}

// ─── Admin-only: list all follow-ups across clients ──────────

// ListAll godoc
// GET /follow-ups  (admin)
func (h *FollowUpHandler) ListAll(c *gin.Context) {
	clientIDStr := c.Query("client_id")
	if clientIDStr != "" {
		clientID, err := uuid.Parse(clientIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid client_id"})
			return
		}
		followUps, err := h.followUpService.GetByClient(c.Request.Context(), clientID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to fetch follow-ups"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": followUps})
		return
	}

	c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "client_id query parameter required"})
}
