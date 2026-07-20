// Package kit models expected-together asset groups (TRA-1032): a commissioned
// set of assets (coupon/tote pairs, AV cases) verified against a dock scan.
// Internal-only endpoints; wire shapes are frozen against the sibling frontend
// ticket — coordinate before changing any JSON field.
package kit

import (
	"fmt"
	"time"
)

const (
	StatusActive = "active"
	StatusClosed = "closed"

	ResultComplete   = "complete"
	ResultIncomplete = "incomplete"
)

type CommissionMemberRequest struct {
	EPC  string  `json:"epc" validate:"required,min=1,max=255" example:"1184015AA01"`
	Role *string `json:"role,omitempty" validate:"omitempty,max=100" example:"coupon"`
	Name *string `json:"name,omitempty" validate:"omitempty,max=255"`
}

type CommissionRequest struct {
	Label   string                    `json:"label" validate:"required,min=1,max=255" example:"1184015"`
	Members []CommissionMemberRequest `json:"members" validate:"required,min=2,dive"`
	// Metadata holds optional QA details (part/heat/operator/date/vendor —
	// TRA-1033). Free-form string map; keys are the frontend's field ids.
	Metadata map[string]string `json:"metadata,omitempty" validate:"omitempty,max=20,dive,keys,min=1,max=50,endkeys,max=500"`
}

type Member struct {
	AssetID int      `json:"asset_id"`
	Role    *string  `json:"role"`
	Name    string   `json:"name"`
	EPCs    []string `json:"epcs"`
}

type VerificationSummary struct {
	Result     string    `json:"result"`
	VerifiedAt time.Time `json:"verified_at"`
}

type Kit struct {
	ID                 int                  `json:"id"`
	Label              string               `json:"label"`
	Status             string               `json:"status"`
	Metadata           map[string]string    `json:"metadata"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
	Members            []Member             `json:"members"`
	LatestVerification *VerificationSummary `json:"latest_verification"`
}

type KitResponse struct {
	Data Kit `json:"data"`
}

type KitSummary struct {
	ID                 int                  `json:"id"`
	Label              string               `json:"label"`
	Status             string               `json:"status"`
	CreatedAt          time.Time            `json:"created_at"`
	MemberCount        int                  `json:"member_count"`
	LatestVerification *VerificationSummary `json:"latest_verification"`
}

type KitListResponse struct {
	Data []KitSummary `json:"data"`
}

type VerifyRequest struct {
	EPCs []string `json:"epcs" validate:"required,min=1,max=1000,dive,min=1,max=255"`
}

type VerifySeenMember struct {
	AssetID int     `json:"asset_id"`
	Role    *string `json:"role"`
	Name    string  `json:"name"`
	// EPCs are the scanned tag values that matched this member in THIS scan
	// (raw as sent, deduped) — the frontend renders them per tag row (TRA-1033).
	EPCs []string `json:"epcs"`
}

type VerifyMissingMember struct {
	AssetID int      `json:"asset_id"`
	Role    *string  `json:"role"`
	Name    string   `json:"name"`
	EPCs    []string `json:"epcs"`
}

type VerifyKitResult struct {
	KitID    int                   `json:"kit_id"`
	Label    string                `json:"label"`
	Result   string                `json:"result"`
	Metadata map[string]string     `json:"metadata"`
	Seen     []VerifySeenMember    `json:"seen"`
	Missing  []VerifyMissingMember `json:"missing"`
}

type VerifyUnexpected struct {
	AssetID           int    `json:"asset_id"`
	EPC               string `json:"epc"`
	Name              string `json:"name"`
	BelongsToKitID    int    `json:"belongs_to_kit_id"`
	BelongsToKitLabel string `json:"belongs_to_kit_label"`
}

// VerifyResponse is the frozen dock-check shape: top-level, no {data} envelope.
type VerifyResponse struct {
	Kits        []VerifyKitResult  `json:"kits"`
	Unexpected  []VerifyUnexpected `json:"unexpected"`
	UnknownEPCs []string           `json:"unknown_epcs"`
}

// ConflictError reports that a commission member is already an active member of
// another active kit (an asset certifies at most one lot). Maps to HTTP 409.
type ConflictError struct {
	AssetName string
	KitLabel  string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("asset %q is already an active member of kit %q", e.AssetName, e.KitLabel)
}

// ValidationError reports a request-content problem detected in the storage
// layer (e.g. duplicate member EPCs). Maps to HTTP 400.
type ValidationError struct {
	Detail string
}

func (e *ValidationError) Error() string {
	return e.Detail
}
