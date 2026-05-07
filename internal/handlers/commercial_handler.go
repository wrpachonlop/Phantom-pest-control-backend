package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/phantompestcontrol/crm/internal/dto"
	"github.com/phantompestcontrol/crm/internal/middleware"
	"github.com/phantompestcontrol/crm/internal/repositories"
	"github.com/phantompestcontrol/crm/internal/services"
	"go.uber.org/zap"
)

// CommercialHandler handles all /commercial/* routes.
type CommercialHandler struct {
	commercialSvc  *services.CommercialService
	commercialRepo *repositories.CommercialRepository
	logger         *zap.Logger
}

func NewCommercialHandler(
	commercialSvc *services.CommercialService,
	commercialRepo *repositories.CommercialRepository,
	logger *zap.Logger,
) *CommercialHandler {
	return &CommercialHandler{
		commercialSvc:  commercialSvc,
		commercialRepo: commercialRepo,
		logger:         logger,
	}
}

// GetDetails GET /clients/:id/commercial
func (h *CommercialHandler) GetDetails(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	details, err := h.commercialRepo.GetDetailsByClientID(c.Request.Context(), id)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, details)
}

// Transition POST /clients/:id/commercial/transition
// The core workflow engine entry point.
func (h *CommercialHandler) Transition(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	req := &dto.CommercialStatusTransitionRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	userID := middleware.GetUserID(c)
	details, err := h.commercialSvc.TransitionStatus(
		c.Request.Context(), id, req, userID,
		c.ClientIP(), c.Request.UserAgent(),
	)
	if err != nil {
		h.logger.Warn("commercial transition failed",
			zap.String("client_id", id.String()),
			zap.Error(err),
		)
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, details)
}

// UpdateDetails PUT /clients/:id/commercial
// Partial update of business fields (not workflow status).
func (h *CommercialHandler) UpdateDetails(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	req := &dto.UpdateCommercialDetailsRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if req.CompanyName != nil {
		updates["company_name"] = *req.CompanyName
	}
	if req.ContactPersonName != nil {
		updates["contact_person_name"] = *req.ContactPersonName
	}
	if req.ServiceAddress != nil {
		updates["service_address"] = *req.ServiceAddress
	}
	if req.BillingAddress != nil {
		updates["billing_address"] = *req.BillingAddress
	}
	if req.BillingSameAsService != nil {
		updates["billing_same_as_service"] = *req.BillingSameAsService
	}
	if req.BillingTerms != nil {
		updates["billing_terms"] = *req.BillingTerms
	}
	if req.PhoneNumber != nil {
		updates["phone_number"] = *req.PhoneNumber
	}
	if req.Email != nil {
		updates["email"] = *req.Email
	}
	if req.Notes != nil {
		updates["notes"] = *req.Notes
	}

	if err := h.commercialRepo.UpdateDetails(c.Request.Context(), id, updates); err != nil {
		handleServiceError(c, err)
		return
	}

	details, _ := h.commercialRepo.GetDetailsByClientID(c.Request.Context(), id)
	c.JSON(http.StatusOK, details)
}

// ReassignInspector PUT /clients/:id/commercial/inspector
func (h *CommercialHandler) ReassignInspector(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	req := &dto.ReassignInspectorRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	userID := middleware.GetUserID(c)
	if err := h.commercialSvc.ReassignInspector(
		c.Request.Context(), id, req, userID,
		c.ClientIP(), c.Request.UserAgent(),
	); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, dto.SuccessResponse{Message: "inspector reassigned"})
}

// GetTransitionHistory GET /clients/:id/commercial/history
func (h *CommercialHandler) GetTransitionHistory(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	transitions, err := h.commercialRepo.GetTransitionHistory(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to fetch history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": transitions})
}

// GetAssignmentHistory GET /clients/:id/commercial/assignments
func (h *CommercialHandler) GetAssignmentHistory(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	assignments, err := h.commercialRepo.GetAssignmentHistory(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to fetch assignments"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": assignments})
}

// ListInspectors GET /inspectors
func (h *CommercialHandler) ListInspectors(c *gin.Context) {
	inspectors, err := h.commercialRepo.ListInspectors(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to fetch inspectors"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": inspectors})
}

// ListCrewMembers GET /crew-members
func (h *CommercialHandler) ListCrewMembers(c *gin.Context) {
	members, err := h.commercialRepo.ListCrewMembers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to fetch crew members"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": members})
}

// GetDashboardAlerts GET /commercial/alerts
func (h *CommercialHandler) GetDashboardAlerts(c *gin.Context) {
	alerts, err := h.commercialSvc.GetDashboardAlerts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to fetch alerts"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": alerts})
}
