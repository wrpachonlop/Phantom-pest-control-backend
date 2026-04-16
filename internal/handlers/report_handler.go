package handlers

import (
	"net/http"
	"time"

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
	loc, _ := time.LoadLocation("America/Vancouver")
	anchorStr := c.Query("anchor_date")
	var anchorDate time.Time
	var err error
	if anchorStr != "" {
		// Intentamos parsear la fecha enviada: "2026-04-15"
		anchorDate, err = time.ParseInLocation("2006-01-02", anchorStr, loc)
		if err != nil {
			h.logger.Warn("invalid anchor_date format", zap.String("val", anchorStr))
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid date format, use YYYY-MM-DD"})
			return
		}
	} else {
		// Si no viene nada, usamos el "Hoy" real de Vancouver
		anchorDate = time.Now().In(loc)
	}

	dashboard, err := h.reportService.GetDashboard(c.Request.Context(), anchorDate)
	if err != nil {
		h.logger.Error("get dashboard", zap.Error(err))
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "failed to load dashboard"})
		return
	}

	c.JSON(http.StatusOK, dashboard)
}
