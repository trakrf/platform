package shared

const DefaultIdentifierType = "rfid"

type Tag struct {
	ID       int    `json:"id"`
	TagType  string `json:"tag_type" validate:"required,oneof=rfid ble barcode" example:"rfid" extensions:"x-extensible-enum=true"`
	Value    string `json:"value" validate:"required,min=1,max=255"`
	IsActive bool   `json:"is_active"`
}

type TagRequest struct {
	TagType string `json:"tag_type" validate:"omitempty,oneof=rfid ble barcode" example:"rfid" default:"rfid" extensions:"x-extensible-enum=true"`
	Value   string `json:"value" validate:"required,min=1,max=255"`
}

func (t TagRequest) GetType() string {
	if t.TagType == "" {
		return DefaultIdentifierType
	}
	return t.TagType
}
