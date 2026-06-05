package alarmdevices

import "testing"

// TestTransportFieldsError covers the transport-aware validation of an alarm
// device's transport-specific fields (TRA-928): base_url is required and must be
// a valid URL only for http transport, and is ignored for mqtt; command_topic is
// required for mqtt.
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
			msg := transportFieldsError(tc.transport, tc.baseURL, tc.commandTopic)
			if tc.wantErr && msg == "" {
				t.Fatalf("expected a validation error, got none")
			}
			if !tc.wantErr && msg != "" {
				t.Fatalf("expected no validation error, got %q", msg)
			}
		})
	}
}
