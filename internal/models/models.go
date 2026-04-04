package models

import (
	"time"

	"github.com/google/uuid"
)

// =============================================================
// ENUMS
// =============================================================

type ClientType string

const (
	ClientTypeNew       ClientType = "new"
	ClientTypeExisting  ClientType = "existing"
	ClientTypeRecurrent ClientType = "recurrent"
	ClientTypeSpam      ClientType = "spam"
)

type PropertyType string

const (
	PropertyTypeResidential PropertyType = "residential"
	PropertyTypeCommercial  PropertyType = "commercial"
)

type ClientStatus string

const (
	ClientStatusBlue   ClientStatus = "blue"   // Initial, no contact yet
	ClientStatusWhite  ClientStatus = "white"  // Contacted
	ClientStatusYellow ClientStatus = "yellow" // In progress
	ClientStatusPurple ClientStatus = "purple" // Potential, needs follow-up
	ClientStatusGreen  ClientStatus = "green"  // Sold
	ClientStatusRed    ClientStatus = "red"    // Not sold
)

type LocationType string

const (
	LocationTypeAddress LocationType = "address"
	LocationTypeCity    LocationType = "city"
)

type FollowUpType string

const (
	FollowUpTypeInbound  FollowUpType = "inbound"
	FollowUpTypeOutbound FollowUpType = "outbound"
	FollowUpTypeSold     FollowUpType = "sold"
)

type AuditAction string

const (
	AuditActionCreate AuditAction = "create"
	AuditActionUpdate AuditAction = "update"
	AuditActionDelete AuditAction = "delete"
)

type UserRole string

const (
	UserRoleAdmin UserRole = "admin"
	UserRoleUser  UserRole = "user"
)

// =============================================================
// USER
// =============================================================

type User struct {
	ID        uuid.UUID `db:"id" json:"id"`
	Email     string    `db:"email" json:"email"`
	FullName  *string   `db:"full_name" json:"full_name"`
	Role      UserRole  `db:"role" json:"role"`
	AvatarURL *string   `db:"avatar_url" json:"avatar_url"`
	IsActive  bool      `db:"is_active" json:"is_active"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// =============================================================
// CONTACT METHOD
// =============================================================

type ContactMethod struct {
	ID        uuid.UUID `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	IsActive  bool      `db:"is_active" json:"is_active"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// =============================================================
// PEST ISSUE
// =============================================================

type PestIssue struct {
	ID        uuid.UUID `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	IsActive  bool      `db:"is_active" json:"is_active"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// =============================================================
// CLIENT
// =============================================================

type Client struct {
	ID                 uuid.UUID    `db:"id" json:"id"`
	ClientName         *string      `db:"client_name" json:"client_name"`
	ClientType         ClientType   `db:"client_type" json:"client_type"`
	PropertyType       PropertyType `db:"property_type" json:"property_type"`
	Status             ClientStatus `db:"status" json:"status"`
	ClientContactDate  time.Time    `db:"client_contact_date" json:"client_contact_date"`
	FirstContactDate   *time.Time   `db:"first_contact_date" json:"first_contact_date"`
	SoldDate           *time.Time   `db:"sold_date" json:"sold_date"`
	AfterHours         bool         `db:"after_hours" json:"after_hours"`
	ContactMethodID    uuid.UUID    `db:"contact_method_id" json:"contact_method_id"`
	ProblemDescription *string      `db:"problem_description" json:"problem_description"`
	LocationType       LocationType `db:"location_type" json:"location_type"`
	LocationValue      *string      `db:"location_value" json:"location_value"`
	SoldBy             *uuid.UUID   `db:"sold_by" json:"sold_by"`
	SaleRange          *string      `db:"sale_range" json:"sale_range"`
	CreatedBy          uuid.UUID    `db:"created_by" json:"created_by"`
	CreatedAt          time.Time    `db:"created_at" json:"created_at"`
	UpdatedAt          time.Time    `db:"updated_at" json:"updated_at"`
}

// ClientFull is the enriched client view with related entities.
type ClientFull struct {
	Client
	ContactMethod *ContactMethod `json:"contact_method"`
	PestIssues    []PestIssue   `json:"pest_issues"`
	Phones        []Phone       `json:"phones"`
	Emails        []Email       `json:"emails"`
	SoldByUser    *User         `json:"sold_by_user,omitempty"`
	CreatedByUser *User         `json:"created_by_user,omitempty"`
}

// =============================================================
// PHONE
// =============================================================

type Phone struct {
	ID          uuid.UUID `db:"id" json:"id"`
	ClientID    uuid.UUID `db:"client_id" json:"client_id"`
	PhoneNumber string    `db:"phone_number" json:"phone_number"`
	Label       string    `db:"label" json:"label"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

// =============================================================
// EMAIL
// =============================================================

type Email struct {
	ID        uuid.UUID `db:"id" json:"id"`
	ClientID  uuid.UUID `db:"client_id" json:"client_id"`
	Email     string    `db:"email" json:"email"`
	Label     string    `db:"label" json:"label"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// =============================================================
// FOLLOW UP
// =============================================================

type FollowUp struct {
	ID          uuid.UUID    `db:"id" json:"id"`
	ClientID    uuid.UUID    `db:"client_id" json:"client_id"`
	Date        time.Time    `db:"date" json:"date"`
	Type        FollowUpType `db:"type" json:"type"`
	Description *string      `db:"description" json:"description"`
	CreatedBy   uuid.UUID    `db:"created_by" json:"created_by"`
	CreatedAt   time.Time    `db:"created_at" json:"created_at"`
	// Joined
	CreatedByUser *User `json:"created_by_user,omitempty"`
}

// =============================================================
// NOTE
// =============================================================

type Note struct {
	ID        uuid.UUID `db:"id" json:"id"`
	ClientID  uuid.UUID `db:"client_id" json:"client_id"`
	UserID    uuid.UUID `db:"user_id" json:"user_id"`
	Note      string    `db:"note" json:"note"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	// Joined
	User *User `json:"user,omitempty"`
}

// =============================================================
// AUDIT LOG
// =============================================================

type AuditLog struct {
	ID         uuid.UUID   `db:"id" json:"id"`
	UserID     *uuid.UUID  `db:"user_id" json:"user_id"`
	Action     AuditAction `db:"action" json:"action"`
	EntityType string      `db:"entity_type" json:"entity_type"`
	EntityID   uuid.UUID   `db:"entity_id" json:"entity_id"`
	OldValues  interface{} `db:"old_values" json:"old_values"`
	NewValues  interface{} `db:"new_values" json:"new_values"`
	IPAddress  *string     `db:"ip_address" json:"ip_address"`
	UserAgent  *string     `db:"user_agent" json:"user_agent"`
	CreatedAt  time.Time   `db:"created_at" json:"created_at"`
	// Joined
	User *User `json:"user,omitempty"`
}

// =============================================================
// DUPLICATE DETECTION RESULT
// =============================================================

type DuplicateCheckResult struct {
	Found   bool      `json:"found"`
	Clients []*Client `json:"clients"`
	MatchOn string    `json:"match_on"` // "phone", "email", or "both"
}
