package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/phantompestcontrol/crm/internal/models"
)

// =============================================================
// CLIENT DTOs
// =============================================================

// CreateClientRequest is the payload for POST /clients
type CreateClientRequest struct {
	ClientName         *string             `json:"client_name"`
	ClientType         models.ClientType   `json:"client_type" validate:"required,oneof=new existing recurrent spam"`
	PropertyType       models.PropertyType `json:"property_type" validate:"required,oneof=residential commercial"`
	Status             models.ClientStatus `json:"status" validate:"omitempty,oneof=blue white yellow purple green red"`
	ClientContactDate  string              `json:"client_contact_date" validate:"required"` // YYYY-MM-DD
	AfterHours         bool                `json:"after_hours"`
	ContactMethodID    uuid.UUID           `json:"contact_method_id" validate:"required"`
	ProblemDescription *string             `json:"problem_description"`
	LocationType       models.LocationType `json:"location_type" validate:"required,oneof=address city"`
	LocationValue      *string             `json:"location_value"`
	SaleRange          *string             `json:"sale_range"`
	SoldDate           *string             `json:"sold_date"` // YYYY-MM-DD, required if status=green
	// Related entities (created atomically)
	Phones     []PhoneInput `json:"phones"`
	Emails     []EmailInput `json:"emails"`
	PestIssues []uuid.UUID  `json:"pest_issues" validate:"required,min=1"`
}

// UpdateClientRequest is the payload for PUT /clients/:id
type UpdateClientRequest struct {
	ClientName         *string              `json:"client_name"`
	ClientType         *models.ClientType   `json:"client_type" validate:"omitempty,oneof=new existing recurrent spam"`
	PropertyType       *models.PropertyType `json:"property_type" validate:"omitempty,oneof=residential commercial"`
	Status             *models.ClientStatus `json:"status" validate:"omitempty,oneof=blue white yellow purple green red"`
	ClientContactDate  *string              `json:"client_contact_date"`
	AfterHours         *bool                `json:"after_hours"`
	ContactMethodID    *uuid.UUID           `json:"contact_method_id"`
	ProblemDescription *string              `json:"problem_description"`
	LocationType       *models.LocationType `json:"location_type" validate:"omitempty,oneof=address city"`
	LocationValue      *string              `json:"location_value"`
	SoldBy             *uuid.UUID           `json:"sold_by"`
	SaleRange          *string              `json:"sale_range"`
	SoldDate           *string              `json:"sold_date"`
	Phones             *[]PhoneInput        `json:"phones"`
	Emails             *[]EmailInput        `json:"emails"`
	PestIssues         *[]uuid.UUID         `json:"pest_issues"`
}

// ClientListRequest has query params for listing clients
type ClientListRequest struct {
	Page         int                  `form:"page" validate:"min=1"`
	PageSize     int                  `form:"page_size" validate:"min=1,max=100"`
	Status       *models.ClientStatus `form:"status"`
	PropertyType *models.PropertyType `form:"property_type"`
	AfterHours   *bool                `form:"after_hours"`
	Search       string               `form:"search"`    // fuzzy search on name/location
	DateFrom     string               `form:"date_from"` // YYYY-MM-DD
	DateTo       string               `form:"date_to"`   // YYYY-MM-DD
	SortBy       string               `form:"sort_by"`   // created_at, client_contact_date, etc.
	SortDir      string               `form:"sort_dir"`  // asc | desc
}

// =============================================================
// PHONE / EMAIL INPUT
// =============================================================

type PhoneInput struct {
	PhoneNumber string `json:"phone_number"`
	Label       string `json:"label"`
}

type EmailInput struct {
	Email string `json:"email"`
	Label string `json:"label"`
}

// =============================================================
// DUPLICATE CHECK
// =============================================================

type DuplicateCheckRequest struct {
	Phones []string `json:"phones"`
	Emails []string `json:"emails"`
}

// =============================================================
// FOLLOW-UP DTOs
// =============================================================

type CreateFollowUpRequest struct {
	ClientID    uuid.UUID           `json:"client_id" validate:"required"`
	Date        string              `json:"date" validate:"required"` // YYYY-MM-DD
	Type        models.FollowUpType `json:"type" validate:"required,oneof=inbound outbound sold"`
	Description *string             `json:"description"`
}

type UpdateFollowUpRequest struct {
	Date        *string              `json:"date"`
	Type        *models.FollowUpType `json:"type" validate:"omitempty,oneof=inbound outbound sold"`
	Description *string              `json:"description"`
}

// =============================================================
// NOTE DTOs
// =============================================================

type CreateNoteRequest struct {
	ClientID uuid.UUID `json:"client_id" validate:"required"`
	Note     string    `json:"note" validate:"required,min=1"`
}

type UpdateNoteRequest struct {
	Note string `json:"note" validate:"required,min=1"`
}

// =============================================================
// CONTACT METHOD DTOs
// =============================================================

type CreateContactMethodRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}

type UpdateContactMethodRequest struct {
	Name     *string `json:"name" validate:"omitempty,min=1,max=100"`
	IsActive *bool   `json:"is_active"`
}

// =============================================================
// PEST ISSUE DTOs
// =============================================================

type CreatePestIssueRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}

type UpdatePestIssueRequest struct {
	Name     *string `json:"name" validate:"omitempty,min=1,max=100"`
	IsActive *bool   `json:"is_active"`
}

// =============================================================
// REPORT DTOs
// =============================================================

// ReportRequest defines parameters for any report
type ReportRequest struct {
	Period   string `form:"period" validate:"required,oneof=daily weekly monthly custom"`
	DateFrom string `form:"date_from"` // YYYY-MM-DD, required for custom
	DateTo   string `form:"date_to"`   // YYYY-MM-DD, required for custom
	// For daily/weekly/monthly, these are auto-computed from the anchor date:
	AnchorDate string `form:"anchor_date"` // YYYY-MM-DD, e.g. "today" period
}

// ReportPeriodResult is returned for a single time window
type ReportPeriodResult struct {
	PeriodLabel string                    `json:"period_label"` // e.g. "Week of Jan 1–7"
	DateFrom    time.Time                 `json:"date_from"`
	DateTo      time.Time                 `json:"date_to"`
	Residential *ResidentialReportSection `json:"residential"`
	Commercial  *CommercialReportSection  `json:"commercial"`
	AfterHours  *AfterHoursSection        `json:"after_hours"`
	Totals      *ReportTotals             `json:"totals"`
}

// ResidentialReportSection breaks down by contact method
type ResidentialReportSection struct {
	ByContactMethod []ContactMethodBreakdown `json:"by_contact_method"`
	TotalReceived   int                      `json:"total_received"`
	TotalSold       int                      `json:"total_sold"`
	ConversionRate  float64                  `json:"conversion_rate"`
}

type ContactMethodBreakdown struct {
	ContactMethodID   uuid.UUID `json:"contact_method_id"`
	ContactMethodName string    `json:"contact_method_name"`
	Received          int       `json:"received"`
	Sold              int       `json:"sold"`
	ConversionRate    float64   `json:"conversion_rate"`
}

// CommercialReportSection is simpler (no contact method breakdown)
type CommercialReportSection struct {
	TotalReceived  int     `json:"total_received"`
	TotalSold      int     `json:"total_sold"`
	ConversionRate float64 `json:"conversion_rate"`
}

type AfterHoursSection struct {
	TotalReceived  int     `json:"total_received"`
	TotalSold      int     `json:"total_sold"`
	ConversionRate float64 `json:"conversion_rate"`
}

type ReportTotals struct {
	TotalReceived  int     `json:"total_received"`
	TotalSold      int     `json:"total_sold"`
	ConversionRate float64 `json:"conversion_rate"`
}

// =============================================================
// PAGINATION
// =============================================================

type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

// =============================================================
// COMMON RESPONSES
// =============================================================

type ErrorResponse struct {
	Error   string      `json:"error"`
	Details interface{} `json:"details,omitempty"`
}

type SuccessResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
