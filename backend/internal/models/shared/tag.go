package shared

const DefaultTagType = "rfid"

type Tag struct {
	ID      int    `json:"id"`
	TagType string `json:"tag_type" validate:"required,oneof=rfid ble barcode" example:"rfid" extensions:"x-extensible-enum=true"`
	Value   string `json:"value" validate:"required,min=1,max=255,no_control_chars"`
}

// TagRequest distinguishes "field omitted" (default to rfid) from "field
// supplied as empty string" (reject as schema-violating). Pointer type
// preserves that distinction across the JSON decoder. TRA-678.
type TagRequest struct {
	TagType *string `json:"tag_type,omitempty" validate:"omitempty,oneof=rfid ble barcode" example:"rfid" default:"rfid" extensions:"x-extensible-enum=true"`
	Value   string  `json:"value" validate:"required,min=1,max=255,no_control_chars"`
}

func (t TagRequest) GetType() string {
	if t.TagType == nil || *t.TagType == "" {
		return DefaultTagType
	}
	return *t.TagType
}
