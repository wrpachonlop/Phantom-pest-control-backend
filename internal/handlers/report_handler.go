package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/phantompestcontrol/crm/internal/dto"
	"github.com/phantompestcontrol/crm/internal/services"
	"go.uber.org/zap"
)

// ReportHandler handles HTTP requests for reporting endpoints.
type ReportHandler struct {
	reportService *services.ReportService
	logger        *zap.Logger
}

func NewReportHandler(reportService *services.ReportService, logger *zap.Logger) *ReportHandler {
	return &ReportHandler{reportService: reportService, logger: logger}
}

// GetReport godoc
// GET /reports
// Query params: period (daily|weekly|monthly|custom), date_from, date_to, anchor_date
func (h *ReportHandler) GetReport(c *gin.Context) {
	req := &dto.ReportRequest{}
	if err := c.ShouldBindQuery(req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	report, err := h.reportService.GetPeriodReport(c.Request.Context(), req)
	if err != nil {
		h.logger.Error("get report", zap.Error(err))
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, report)
}

// GetDashboard godoc
// GET /reports/dashboard
func (h *ReportHandler) GetDashboard(c *gin.Context) {
	dashboard, err := h.reportService.GetDashboard(c.Request.Context())
	if err != nil {
		h.logger.Error("get dashboard", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to load dashboard"})
		return
	}

	c.JSON(http.StatusOK, dashboard)
}
