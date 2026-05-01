package models

import (
	"time"

	"github.com/google/uuid"
)

// =============================================================
// NEW ENUMS — Commercial Workflow
// =============================================================

type CommercialStatus string

const (
	CommercialStatusAssigned  CommercialStatus = "assigned"
	CommercialStatusPending   CommercialStatus = "pending"
	CommercialStatusApproved  CommercialStatus = "approved"
	CommercialStatusDeclined  CommercialStatus = "declined"
	CommercialStatusInstalled CommercialStatus = "installed"
	CommercialStatusCancelled CommercialStatus = "cancelled"
)

// TerminalStatuses cannot be transitioned out of.
var CommercialTerminalStatuses = map[CommercialStatus]bool{
	CommercialStatusDeclined:  true,
	CommercialStatusCancelled: true,
}

type LeadSource string

const (
	LeadSourceOffice     LeadSource = "office"
	LeadSourceCrewMember LeadSource = "crew_member"
)

type BillingTerms string

const (
	BillingTermsOnCompletion     BillingTerms = "on_completion"
	BillingTermsCreditCardOnFile BillingTerms = "credit_card_on_file"
	BillingTermsNet15            BillingTerms = "net_15"
	BillingTermsNet30            BillingTerms = "net_30"
	BillingTermsNet60            BillingTerms = "net_60"
)

type ServiceFrequency string

const (
	ServiceFrequencyDaily      ServiceFrequency = "daily"
	ServiceFrequencyWeekly     ServiceFrequency = "weekly"
	ServiceFrequencyMonthly    ServiceFrequency = "monthly"
	ServiceFrequencyBiMonthly  ServiceFrequency = "bi_monthly"
	ServiceFrequencyQuarterly  ServiceFrequency = "quarterly"
	ServiceFrequencyTriAnnual  ServiceFrequency = "tri_annual"
	ServiceFrequencySemiAnnual ServiceFrequency = "semi_annual"
	ServiceFrequencySeasonal   ServiceFrequency = "seasonal"
	ServiceFrequencyYearly     ServiceFrequency = "yearly"
)

// FrequencySupportsInterval returns true for frequencies that allow
// a custom interval (e.g. "every 2 weeks").
func FrequencySupportsInterval(f ServiceFrequency) bool {
	return f == ServiceFrequencyDaily ||
		f == ServiceFrequencyWeekly ||
		f == ServiceFrequencyMonthly
}

// =============================================================
// ALLOWED TRANSITIONS — enforced at service layer
// =============================================================

var CommercialAllowedTransitions = map[CommercialStatus][]CommercialStatus{
	CommercialStatusAssigned:  {CommercialStatusPending, CommercialStatusApproved, CommercialStatusDeclined},
	CommercialStatusPending:   {CommercialStatusApproved, CommercialStatusDeclined},
	CommercialStatusApproved:  {CommercialStatusInstalled, CommercialStatusCancelled},
	CommercialStatusInstalled: {CommercialStatusCancelled},
	CommercialStatusDeclined:  {},
	CommercialStatusCancelled: {},
}

// IsTransitionAllowed validates a status transition.
func IsTransitionAllowed(from, to CommercialStatus) bool {
	allowed, ok := CommercialAllowedTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// StatusRequiresProposalLink returns true for statuses that require
// a Google Drive proposal link before transitioning.
func StatusRequiresProposalLink(to CommercialStatus) bool {
	return to == CommercialStatusPending ||
		to == CommercialStatusApproved ||
		to == CommercialStatusDeclined
}

// =============================================================
// CREW MEMBER
// =============================================================

type CrewMember struct {
	ID         uuid.UUID  `db:"id"          json:"id"`
	FullName   string     `db:"full_name"   json:"full_name"`
	EmployeeID *string    `db:"employee_id" json:"employee_id"`
	IsActive   bool       `db:"is_active"   json:"is_active"`
	CreatedBy  *uuid.UUID `db:"created_by"  json:"created_by"`
	CreatedAt  time.Time  `db:"created_at"  json:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at"  json:"updated_at"`
}

// =============================================================
// COMMERCIAL CLIENT DETAILS (1:1 with clients)
// =============================================================

type CommercialClientDetails struct {
	ID       uuid.UUID `db:"id"                     json:"id"`
	ClientID uuid.UUID `db:"client_id"              json:"client_id"`

	// Workflow
	WorkflowStatus CommercialStatus `db:"workflow_status"        json:"workflow_status"`
	LeadSource     LeadSource       `db:"lead_source"            json:"lead_source"`
	CrewMemberID   *uuid.UUID       `db:"crew_member_id"         json:"crew_member_id"`
	InspectorID    uuid.UUID        `db:"inspector_id"           json:"inspector_id"`

	// Business identity
	CompanyName       *string `db:"company_name"           json:"company_name"`
	ContactPersonName *string `db:"contact_person_name"    json:"contact_person_name"`

	// Addresses
	ServiceAddress       *string `db:"service_address"        json:"service_address"`
	BillingAddress       *string `db:"billing_address"        json:"billing_address"`
	BillingSameAsService bool    `db:"billing_same_as_service" json:"billing_same_as_service"`

	// Financial
	BillingTerms         *BillingTerms     `db:"billing_terms"          json:"billing_terms"`
	InitialSetupCost     *float64          `db:"initial_setup_cost"     json:"initial_setup_cost"`
	RecurringServiceCost *float64          `db:"recurring_service_cost" json:"recurring_service_cost"`
	ServiceFrequency     *ServiceFrequency `db:"service_frequency"     json:"service_frequency"`
	FrequencyInterval    *int              `db:"frequency_interval"     json:"frequency_interval"`

	// Contact
	PhoneNumber *string `db:"phone_number"           json:"phone_number"`
	Email       *string `db:"email"                  json:"email"`
	Notes       *string `db:"notes"                  json:"notes"`

	// Proposal
	ProposalDriveLink *string `db:"proposal_drive_link"    json:"proposal_drive_link"`

	// Approval
	ApprovedByName   *string    `db:"approved_by_name"       json:"approved_by_name"`
	ApprovedDate     *time.Time `db:"approved_date"          json:"approved_date"`
	ApprovedByUserID *uuid.UUID `db:"approved_by_user_id"    json:"approved_by_user_id"`

	// Pending
	NextFollowupDate *time.Time `db:"next_followup_date"     json:"next_followup_date"`

	// Installation
	InstallationDate  *time.Time `db:"installation_date"      json:"installation_date"`
	InstallationNotes *string    `db:"installation_notes"     json:"installation_notes"`

	// Cancellation
	CancelledDate *time.Time `db:"cancelled_date"         json:"cancelled_date"`
	CancelReason  *string    `db:"cancel_reason"          json:"cancel_reason"`

	// Meta
	CreatedAt time.Time `db:"created_at"             json:"created_at"`
	UpdatedAt time.Time `db:"updated_at"             json:"updated_at"`

	// Joined fields (not stored in this table)
	Inspector  *User       `json:"inspector,omitempty"`
	CrewMember *CrewMember `json:"crew_member,omitempty"`
}

// =============================================================
// COMMERCIAL INSPECTOR ASSIGNMENT
// =============================================================

type CommercialInspectorAssignment struct {
	ID           uuid.UUID  `db:"id"            json:"id"`
	ClientID     uuid.UUID  `db:"client_id"     json:"client_id"`
	InspectorID  uuid.UUID  `db:"inspector_id"  json:"inspector_id"`
	AssignedBy   *uuid.UUID `db:"assigned_by"   json:"assigned_by"`
	AssignedAt   time.Time  `db:"assigned_at"   json:"assigned_at"`
	UnassignedAt *time.Time `db:"unassigned_at" json:"unassigned_at"`
	Notes        *string    `db:"notes"         json:"notes"`
	// Joined
	Inspector *User `json:"inspector,omitempty"`
}

// =============================================================
// COMMERCIAL STATUS TRANSITION
// =============================================================

type CommercialStatusTransition struct {
	ID           uuid.UUID         `db:"id"            json:"id"`
	ClientID     uuid.UUID         `db:"client_id"     json:"client_id"`
	FromStatus   *CommercialStatus `db:"from_status"  json:"from_status"` // null = initial
	ToStatus     CommercialStatus  `db:"to_status"     json:"to_status"`
	ChangedBy    *uuid.UUID        `db:"changed_by"    json:"changed_by"`
	ProposalLink *string           `db:"proposal_link" json:"proposal_link"`
	Notes        *string           `db:"notes"         json:"notes"`
	CreatedAt    time.Time         `db:"created_at"    json:"created_at"`
	// Joined
	ChangedByUser *User `json:"changed_by_user,omitempty"`
}

// =============================================================
// CLIENT LOCATION
// =============================================================

type ClientLocation struct {
	ID            uuid.UUID    `db:"id"             json:"id"`
	ClientID      uuid.UUID    `db:"client_id"      json:"client_id"`
	Label         string       `db:"label"          json:"label"`
	LocationType  LocationType `db:"location_type"  json:"location_type"`
	LocationValue string       `db:"location_value" json:"location_value"`
	IsPrimary     bool         `db:"is_primary"     json:"is_primary"`
	CreatedAt     time.Time    `db:"created_at"     json:"created_at"`
	UpdatedAt     time.Time    `db:"updated_at"     json:"updated_at"`
}

// =============================================================
// NOTIFICATION RECIPIENT
// =============================================================

type NotificationRecipient struct {
	ID        uuid.UUID `db:"id"         json:"id"`
	EventType string    `db:"event_type" json:"event_type"`
	Name      string    `db:"name"       json:"name"`
	Email     string    `db:"email"      json:"email"`
	IsActive  bool      `db:"is_active"  json:"is_active"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// =============================================================
// COMPOSITE VIEW — full commercial client
// =============================================================

type CommercialClientFull struct {
	// Embedded base client
	Client      *Client                      `json:"client"`
	Details     *CommercialClientDetails     `json:"details"`
	Transitions []CommercialStatusTransition `json:"transitions"`
	Locations   []ClientLocation             `json:"locations"`
}
