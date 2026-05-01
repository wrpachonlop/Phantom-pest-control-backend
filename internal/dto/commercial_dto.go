package dto

import (
	"github.com/google/uuid"
	"github.com/phantompestcontrol/crm/internal/models"
)

// =============================================================
// CREATE COMMERCIAL CLIENT
// Used when property_type = commercial on client creation
// =============================================================

type CreateCommercialDetailsRequest struct {
	// Required immediately
	LeadSource   models.LeadSource `json:"lead_source" validate:"required,oneof=office crew_member"`
	CrewMemberID *uuid.UUID        `json:"crew_member_id"` // required when lead_source = crew_member
	InspectorID  uuid.UUID         `json:"inspector_id" validate:"required"`

	// Business info (optional at creation, required before approval)
	CompanyName          *string              `json:"company_name"`
	ContactPersonName    *string              `json:"contact_person_name"`
	ServiceAddress       *string              `json:"service_address"`
	BillingAddress       *string              `json:"billing_address"`
	BillingSameAsService bool                 `json:"billing_same_as_service"`
	BillingTerms         *models.BillingTerms `json:"billing_terms" validate:"omitempty,oneof=on_completion credit_card_on_file net_15 net_30 net_60"`
	PhoneNumber          *string              `json:"phone_number"`
	Email                *string              `json:"email" validate:"omitempty,email"`
	Notes                *string              `json:"notes"`
}

// =============================================================
// UPDATE COMMERCIAL DETAILS
// Partial update — all fields optional
// =============================================================

type UpdateCommercialDetailsRequest struct {
	CompanyName          *string              `json:"company_name"`
	ContactPersonName    *string              `json:"contact_person_name"`
	ServiceAddress       *string              `json:"service_address"`
	BillingAddress       *string              `json:"billing_address"`
	BillingSameAsService *bool                `json:"billing_same_as_service"`
	BillingTerms         *models.BillingTerms `json:"billing_terms" validate:"omitempty,oneof=on_completion credit_card_on_file net_15 net_30 net_60"`
	PhoneNumber          *string              `json:"phone_number"`
	Email                *string              `json:"email" validate:"omitempty,email"`
	Notes                *string              `json:"notes"`
}

// =============================================================
// STATUS TRANSITION REQUEST
// Payload varies by target status
// =============================================================

type CommercialStatusTransitionRequest struct {
	// The target status — always required
	ToStatus models.CommercialStatus `json:"to_status" validate:"required,oneof=assigned pending approved declined installed cancelled"`

	// Required for pending/approved/declined
	ProposalDriveLink *string `json:"proposal_drive_link"`

	// Required for pending
	NextFollowupDate *string `json:"next_followup_date"` // YYYY-MM-DD

	// Required for approved
	ApprovedByName       *string                  `json:"approved_by_name"`
	ApprovedDate         *string                  `json:"approved_date"` // YYYY-MM-DD
	InitialSetupCost     *float64                 `json:"initial_setup_cost"`
	RecurringServiceCost *float64                 `json:"recurring_service_cost"`
	ServiceFrequency     *models.ServiceFrequency `json:"service_frequency" validate:"omitempty,oneof=daily weekly monthly bi_monthly quarterly tri_annual semi_annual seasonal yearly"`
	FrequencyInterval    *int                     `json:"frequency_interval"`
	BillingTerms         *models.BillingTerms     `json:"billing_terms" validate:"omitempty,oneof=on_completion credit_card_on_file net_15 net_30 net_60"`

	// Required for installed
	InstallationDate  *string `json:"installation_date"` // YYYY-MM-DD
	InstallationNotes *string `json:"installation_notes"`

	// Required for cancelled
	CancelledDate *string `json:"cancelled_date"` // YYYY-MM-DD
	CancelReason  *string `json:"cancel_reason"`

	// Optional always
	TransitionNotes *string `json:"notes"`
}

// =============================================================
// REASSIGN INSPECTOR
// =============================================================

type ReassignInspectorRequest struct {
	InspectorID uuid.UUID `json:"inspector_id" validate:"required"`
	Notes       *string   `json:"notes"`
}

// =============================================================
// CREW MEMBER DTOs
// =============================================================

type CreateCrewMemberRequest struct {
	FullName   string  `json:"full_name"   validate:"required,min=1,max=200"`
	EmployeeID *string `json:"employee_id"`
}

type UpdateCrewMemberRequest struct {
	FullName   *string `json:"full_name"   validate:"omitempty,min=1,max=200"`
	EmployeeID *string `json:"employee_id"`
	IsActive   *bool   `json:"is_active"`
}

// =============================================================
// LOCATION DTOs
// =============================================================

type CreateClientLocationRequest struct {
	Label         string              `json:"label"          validate:"required"`
	LocationType  models.LocationType `json:"location_type"  validate:"required,oneof=address city"`
	LocationValue string              `json:"location_value" validate:"required"`
	IsPrimary     bool                `json:"is_primary"`
}

// =============================================================
// NOTIFICATION RECIPIENT DTOs
// =============================================================

type CreateNotificationRecipientRequest struct {
	EventType string `json:"event_type" validate:"required"`
	Name      string `json:"name"       validate:"required"`
	Email     string `json:"email"      validate:"required,email"`
}

// =============================================================
// DASHBOARD ALERT (returned in dashboard payload)
// =============================================================

type CommercialDashboardAlert struct {
	ClientID         string `json:"client_id"`
	CompanyName      string `json:"company_name"`
	NextFollowupDate string `json:"next_followup_date"`
	InspectorName    string `json:"inspector_name"`
	DaysUntilDue     int    `json:"days_until_due"`
}
