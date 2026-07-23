package outputdevices

import (
	"testing"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
)

// TestTransportFieldsError covers the transport-aware validation of a
// shelly_gen4 device's transport-specific fields (TRA-928): base_url is
// required and must be a valid URL only for http transport, and is ignored
// for mqtt; command_topic is required for mqtt. switch_id 0 is the common
// case for a single-relay Gen4 and must stay valid.
func TestTransportFieldsError(t *testing.T) {
	cases := []struct {
		name         string
		transport    string
		baseURL      string
		commandTopic string
		wantErr      bool
	}{
		{name: "http valid base_url", transport: "http", baseURL: "http://192.168.50.66", wantErr: false},
		{name: "http https base_url", transport: "http", baseURL: "https://device.local", wantErr: false},
		{name: "http empty base_url", transport: "http", baseURL: "", wantErr: true},
		{name: "http invalid base_url", transport: "http", baseURL: "not-a-url", wantErr: true},
		{name: "default (blank) transport treated as http", transport: "", baseURL: "http://192.168.50.66", wantErr: false},
		{name: "default (blank) transport requires base_url", transport: "", baseURL: "", wantErr: true},
		// mqtt: base_url is irrelevant — an empty (TRA-928) or even garbage value must be accepted.
		{name: "mqtt with command_topic ignores empty base_url", transport: "mqtt", baseURL: "", commandTopic: "trakrf.id/dock-strobe", wantErr: false},
		{name: "mqtt with command_topic ignores stale base_url", transport: "mqtt", baseURL: "not-a-url", commandTopic: "trakrf.id/dock-strobe", wantErr: false},
		{name: "mqtt without command_topic", transport: "mqtt", baseURL: "", commandTopic: "", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := deviceFieldsError(outputdevice.TypeShellyGen4, tc.transport, tc.baseURL, tc.commandTopic, 0)
			if tc.wantErr && msg == "" {
				t.Fatalf("expected a validation error, got none")
			}
			if !tc.wantErr && msg != "" {
				t.Fatalf("expected no validation error, got %q", msg)
			}
		})
	}
}

// TestDeviceFieldsError_CS463GPO covers the type-aware validation added for
// csl_cs463_gpo (TRA-1028): mqtt-only and switch_id (the GPO port) must be
// 1-4. command_topic is NOT required for GPO (the reader is addressed by
// scan_device_id, checked separately against storage in the handler, not in
// this pure function) — a device may carry a stale command_topic harmlessly,
// or none at all. Shelly rules must stay untouched, including a switch_id of 0.
func TestDeviceFieldsError_CS463GPO(t *testing.T) {
	tests := []struct {
		name         string
		typ          string
		transport    string
		baseURL      string
		commandTopic string
		switchID     int
		wantErr      bool
	}{
		{"valid gpo", outputdevice.TypeCS463GPO, outputdevice.TransportMQTT, "", "trakrf.id/cs463-212", 1, false},
		{"valid gpo port 4", outputdevice.TypeCS463GPO, outputdevice.TransportMQTT, "", "trakrf.id/cs463-212", 4, false},
		{"gpo on http transport", outputdevice.TypeCS463GPO, outputdevice.TransportHTTP, "http://1.2.3.4", "", 1, true},
		{"gpo with no command_topic is fine", outputdevice.TypeCS463GPO, outputdevice.TransportMQTT, "", "", 1, false},
		{"gpo with stale command_topic is fine", outputdevice.TypeCS463GPO, outputdevice.TransportMQTT, "", "trakrf.id/stale", 1, false},
		{"gpo port 0", outputdevice.TypeCS463GPO, outputdevice.TransportMQTT, "", "trakrf.id/cs463-212", 0, true},
		{"gpo port 5", outputdevice.TypeCS463GPO, outputdevice.TransportMQTT, "", "trakrf.id/cs463-212", 5, true},
		// Shelly rules must be untouched, including switch_id 0.
		{"shelly mqtt switch 0", outputdevice.TypeShellyGen4, outputdevice.TransportMQTT, "", "trakrf.id/dock", 0, false},
		{"shelly http", outputdevice.TypeShellyGen4, outputdevice.TransportHTTP, "http://1.2.3.4", "", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := deviceFieldsError(tc.typ, tc.transport, tc.baseURL, tc.commandTopic, tc.switchID)
			if (msg != "") != tc.wantErr {
				t.Errorf("deviceFieldsError = %q, wantErr = %v", msg, tc.wantErr)
			}
		})
	}
}
