package shared

// DefaultTagType is the historical default surfaced when callers omitted
// tag_type. TRA-739 (BB42 F2) tightened tag_type to spec-required on the
// public API, so a write request without tag_type now returns 400
// validation_error / code=required. The constant is retained for internal
// callers (e.g. bulkimport) that synthesize TagRequest values server-side
// where the discriminator is always rfid.
const DefaultTagType = "rfid"

type Tag struct {
	ID      int    `json:"id"`
	TagType string `json:"tag_type" validate:"required,oneof=rfid ble barcode" example:"rfid" extensions:"x-extensible-enum=true"`
	Value   string `json:"value" validate:"required,min=1,max=255,no_control_chars"`
}

// TagRequest is the wire shape of a public tag-write body. Pointer
// distinguishes "field omitted" (TRA-678) from "field supplied as null /
// empty string" so the presence-tracking decoder can promote both omitted
// and explicit-null tag_type to code=required.
//
// TRA-739 (BB42 F2): tag_type is required on the public API to match the
// spec's discriminator (rfid / ble / barcode). The handler previously
// silently defaulted an omitted tag_type to rfid, which diverged from the
// spec subtype schemas (each of which lists tag_type as required) and let
// loose clients land on a not-intended discriminator when the body
// targeted a future variant.
type TagRequest struct {
	TagType *string `json:"tag_type" validate:"required,oneof=rfid ble barcode" example:"rfid" extensions:"x-extensible-enum=true"`
	Value   string  `json:"value" validate:"required,min=1,max=255,no_control_chars"`
}

// GetType returns the tag_type for storage. Validation ensures TagType is
// non-nil and one of the known variants before the handler reaches this
// path; the nil guard remains as a defensive zero-value fallback for
// internal callers that bypass the public validator.
func (t TagRequest) GetType() string {
	if t.TagType == nil || *t.TagType == "" {
		return DefaultTagType
	}
	return *t.TagType
}
