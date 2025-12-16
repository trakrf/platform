package shared

const DefaultIdentifierType = "rfid"

type TagIdentifier struct {
	ID       int    `json:"id,omitempty"`
	Type     string `json:"type" validate:"required,oneof=rfid ble barcode"`
	Value    string `json:"value" validate:"required,min=1,max=255"`
	IsActive bool   `json:"is_active"`
}

type TagIdentifierRequest struct {
	Type  string `json:"type" validate:"omitempty,oneof=rfid ble barcode"`
	Value string `json:"value" validate:"required,min=1,max=255"`
}

func (t TagIdentifierRequest) GetType() string {
	if t.Type == "" {
		return DefaultIdentifierType
	}
	return t.Type
}
