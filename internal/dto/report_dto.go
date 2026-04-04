package dto

import (
	"time"

	"github.com/google/uuid"
)

// DateRange represents a labeled time window for multi-period queries.
type DateRange struct {
	Label string
	From  time.Time
	To    time.Time
}

// PeriodSummaryPoint is a single data point for trend charts.
type PeriodSummaryPoint struct {
	Label          string    `json:"label"`
	From           time.Time `json:"from"`
	To             time.Time `json:"to"`
	TotalReceived  int       `json:"total_received"`
	TotalSold      int       `json:"total_sold"`
	ConversionRate float64   `json:"conversion_rate"`
}

// PerformerRow represents one sales rep's performance.
type PerformerRow struct {
	UserID           uuid.UUID `json:"user_id"`
	FullName         *string   `json:"full_name"`
	Email            string    `json:"email"`
	TotalSales       int       `json:"total_sales"`
	ResidentialSales int       `json:"residential_sales"`
	CommercialSales  int       `json:"commercial_sales"`
}

// DashboardResponse is the full dashboard payload.
type DashboardResponse struct {
	StatusDistribution map[string]int       `json:"status_distribution"`
	WeeklyTrend        []PeriodSummaryPoint `json:"weekly_trend"`
	TopPerformers      []PerformerRow       `json:"top_performers"`
	TodayStats         *ReportPeriodResult  `json:"today_stats"`
	ThisWeekStats      *ReportPeriodResult  `json:"this_week_stats"`
	ThisMonthStats     *ReportPeriodResult  `json:"this_month_stats"`
}
