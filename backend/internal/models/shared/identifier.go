package shared

// DefaultIdentifierType is the default type when not specified.
const DefaultIdentifierType = "rfid"

// TagIdentifier represents a physical tag (RFID, BLE, barcode) linked to an asset or location.
// Used as embedded type in AssetView and LocationView for API responses.
type TagIdentifier struct {
	ID       int    `json:"id,omitempty"`
	Type     string `json:"type" validate:"required,oneof=rfid ble barcode"`
	Value    string `json:"value" validate:"required,min=1,max=255"`
	IsActive bool   `json:"is_active"`
}

// TagIdentifierRequest is used when creating identifiers (no ID field).
// Type defaults to "rfid" if not provided.
type TagIdentifierRequest struct {
	Type  string `json:"type" validate:"omitempty,oneof=rfid ble barcode"`
	Value string `json:"value" validate:"required,min=1,max=255"`
}

// GetType returns the identifier type, defaulting to "rfid" if empty.
func (t TagIdentifierRequest) GetType() string {
	if t.Type == "" {
		return DefaultIdentifierType
	}
	return t.Type
}
