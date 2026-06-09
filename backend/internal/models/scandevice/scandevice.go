// Package scandevice models fixed-reader / gateway scan devices (CS463, GL-S10,
// ESP32 BLE, CS108) and the requests for their internal CRUD endpoints.
package scandevice

import "time"

// Device type / transport mirror the PG enums scan_device_type / scan_transport
// (migration 000011). Design supports all four device types; only the CS463
// (csl_cs463) path is implemented end to end this cycle (TRA-899).
const (
	DeviceTypeCS463    = "csl_cs463"
	DeviceTypeGLS10    = "gl_s10"
	DeviceTypeMK107    = "moko_mk107"
	DeviceTypeESP32BLE = "esp32_ble_generic"
	DeviceTypeCS108    = "csl_cs108"

	TransportMQTT   = "mqtt"
	TransportWebBLE = "web_ble"
)

type ScanDevice struct {
	ID           int        `json:"id"`
	OrgID        int        `json:"org_id"`
	Name         string     `json:"name"`
	Type         string     `json:"type"`
	Transport    string     `json:"transport"`
	PublishTopic *string    `json:"publish_topic,omitempty"`
	SerialNumber *string    `json:"serial_number,omitempty"`
	Model        *string    `json:"model,omitempty"`
	Description  string     `json:"description"`
	Metadata     any        `json:"metadata"`
	ValidFrom    time.Time  `json:"valid_from"`
	ValidTo      *time.Time `json:"valid_to,omitempty"`
	IsActive     bool       `json:"is_active"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

type CreateScanDeviceRequest struct {
	Name         string         `json:"name" validate:"required,min=1,max=255" example:"Dock Door Reader"`
	Type         string         `json:"type" validate:"required,oneof=csl_cs463 gl_s10 moko_mk107 esp32_ble_generic csl_cs108" example:"csl_cs463"`
	Transport    string         `json:"transport,omitempty" validate:"omitempty,oneof=mqtt web_ble" example:"mqtt"`
	PublishTopic *string        `json:"publish_topic,omitempty" validate:"omitempty,min=1,max=255" example:"trakrf.id/cs463-214/reads"`
	SerialNumber *string        `json:"serial_number,omitempty" validate:"omitempty,max=255"`
	Model        *string        `json:"model,omitempty" validate:"omitempty,max=100"`
	Description  *string        `json:"description,omitempty" validate:"omitempty,max=1024"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	IsActive     *bool          `json:"is_active,omitempty"`
}

type UpdateScanDeviceRequest struct {
	Name         *string         `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	Type         *string         `json:"type,omitempty" validate:"omitempty,oneof=csl_cs463 gl_s10 moko_mk107 esp32_ble_generic csl_cs108"`
	Transport    *string         `json:"transport,omitempty" validate:"omitempty,oneof=mqtt web_ble"`
	PublishTopic *string         `json:"publish_topic,omitempty" validate:"omitempty,min=1,max=255"`
	SerialNumber *string         `json:"serial_number,omitempty" validate:"omitempty,max=255"`
	Model        *string         `json:"model,omitempty" validate:"omitempty,max=100"`
	Description  *string         `json:"description,omitempty" validate:"omitempty,max=1024"`
	Metadata     *map[string]any `json:"metadata,omitempty"`
	IsActive     *bool           `json:"is_active,omitempty"`
}

type ScanDeviceResponse struct {
	Data ScanDevice `json:"data"`
}
