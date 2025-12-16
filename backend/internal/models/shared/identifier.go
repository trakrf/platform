package shared

// TagIdentifier represents a physical tag (RFID, BLE, barcode) linked to an asset or location.
// Used as embedded type in AssetView and LocationView for API responses.
type TagIdentifier struct {
	ID       int    `json:"id,omitempty"`
	Type     string `json:"type" validate:"required,oneof=rfid ble barcode"`
	Value    string `json:"value" validate:"required,min=1,max=255"`
	IsActive bool   `json:"is_active"`
}

// TagIdentifierRequest is used when creating identifiers (no ID field).
type TagIdentifierRequest struct {
	Type  string `json:"type" validate:"required,oneof=rfid ble barcode"`
	Value string `json:"value" validate:"required,min=1,max=255"`
}
