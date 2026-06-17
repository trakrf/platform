package readerd

import (
	"strings"
	"testing"
)

const multiServerCloudServer = `[
  {"serverId":"EMQX","type":"MQTT","path":"emqx.example.com","port":8883,"clientId":"cs463-999","username":"other","password":"OTHERPW","enableSSL":true,"serverCertFile":"emqx.pem","topic":"emqx/cs463-999/reads"},
  {"serverId":"TrakRF MQTT","type":"MQTT","path":"mqtt.preview.gke.trakrf.id","port":8883,"clientId":"cs463-212","username":"trakrf-mqtt","password":"PW with/special","enableSSL":true,"serverCertFile":"le-fullchain-plus-roots.pem","topic":"trakrf.id/cs463-212/reads"},
  {"serverId":"Some HTTP","type":"HTTP","path":"http.example.com","port":443}
]`

func TestParseCloudServer_PicksByServerID(t *testing.T) {
	bc, err := parseCloudServer([]byte(multiServerCloudServer), "TrakRF MQTT", "/opt/EmbeddedGlassFish/cert/CloudServer")
	if err != nil {
		t.Fatalf("parseCloudServer: %v", err)
	}

	// URL: mqtts (enableSSL true) with urlencoded creds.
	wantURL := "mqtts://trakrf-mqtt:PW%20with%2Fspecial@mqtt.preview.gke.trakrf.id:8883"
	if bc.URL != wantURL {
		t.Errorf("URL = %q, want %q", bc.URL, wantURL)
	}

	// BaseTopic: trailing /reads stripped.
	if bc.BaseTopic != "trakrf.id/cs463-212" {
		t.Errorf("BaseTopic = %q, want trakrf.id/cs463-212", bc.BaseTopic)
	}

	// RPCClientID must differ from the reader's own clientId.
	if bc.RPCClientID != "cs463-212-rpc" {
		t.Errorf("RPCClientID = %q, want cs463-212-rpc", bc.RPCClientID)
	}
	if bc.RPCClientID == "cs463-212" {
		t.Error("RPCClientID must differ from reader clientId")
	}

	// CACertPath: <certDir>/<serverId>/<serverCertFile>.
	wantCert := "/opt/EmbeddedGlassFish/cert/CloudServer/TrakRF MQTT/le-fullchain-plus-roots.pem"
	if bc.CACertPath != wantCert {
		t.Errorf("CACertPath = %q, want %q", bc.CACertPath, wantCert)
	}
}

func TestParseCloudServer_PlainMQTTWhenSSLDisabled(t *testing.T) {
	// A plaintext broker (the demo-box local Mosquitto) has enableSSL=false. Even
	// when a stale serverCertFile lingers in the entry, the daemon must NOT try to
	// load a CA cert — there is no TLS — or it fatals at startup.
	data := `[{"serverId":"TrakRF MQTT","type":"MQTT","path":"host","port":1883,"clientId":"c1","username":"u","password":"p","enableSSL":false,"serverCertFile":"x.pem","topic":"a/b/reads"}]`
	bc, err := parseCloudServer([]byte(data), "TrakRF MQTT", "/certs")
	if err != nil {
		t.Fatalf("parseCloudServer: %v", err)
	}
	if !strings.HasPrefix(bc.URL, "mqtt://") {
		t.Errorf("expected mqtt:// scheme, got %q", bc.URL)
	}
	if bc.CACertPath != "" {
		t.Errorf("CACertPath = %q, want empty (no TLS on a plaintext broker)", bc.CACertPath)
	}
}

func TestParseCloudServer_SSLWithoutCertFileUsesSystemRoots(t *testing.T) {
	// SSL enabled but no serverCertFile: connect over TLS using the system root
	// pool (empty CACertPath), not a bogus "<certDir>/<serverId>/" path.
	data := `[{"serverId":"TrakRF MQTT","type":"MQTT","path":"host","port":8883,"clientId":"c1","username":"u","password":"p","enableSSL":true,"serverCertFile":"","topic":"a/b/reads"}]`
	bc, err := parseCloudServer([]byte(data), "TrakRF MQTT", "/certs")
	if err != nil {
		t.Fatalf("parseCloudServer: %v", err)
	}
	if !strings.HasPrefix(bc.URL, "mqtts://") {
		t.Errorf("expected mqtts:// scheme, got %q", bc.URL)
	}
	if bc.CACertPath != "" {
		t.Errorf("CACertPath = %q, want empty (system roots)", bc.CACertPath)
	}
}

func TestParseCloudServer_NotFound(t *testing.T) {
	_, err := parseCloudServer([]byte(multiServerCloudServer), "Nonexistent", "/certs")
	if err == nil {
		t.Fatal("expected error for missing serverId")
	}
}

const eventList = `[
  {"eventId":"HTTP","profileId":"X","enable":false},
  {"eventId":"MQTT","profileId":"TrakRF","enable":true},
  {"eventId":"Other","profileId":"Y","enable":false}
]`

func TestParseEnabledEvent(t *testing.T) {
	id, err := parseEnabledEvent([]byte(eventList))
	if err != nil {
		t.Fatalf("parseEnabledEvent: %v", err)
	}
	if id != "MQTT" {
		t.Errorf("eventId = %q, want MQTT", id)
	}
}

func TestParseEnabledEvent_NoneEnabled(t *testing.T) {
	data := `[{"eventId":"A","enable":false},{"eventId":"B","enable":false}]`
	_, err := parseEnabledEvent([]byte(data))
	if err == nil {
		t.Fatal("expected error when no event enabled")
	}
}

// brokerEnv sets the direct-broker envs so LoadConfig skips on-reader file reads.
func brokerEnv(t *testing.T) {
	t.Helper()
	t.Setenv("READERD_BROKER_URL", "mqtts://u:p@host:8883")
	t.Setenv("READERD_CA_CERT", "/x.pem")
	t.Setenv("READERD_BASE_TOPIC", "trakrf.id/r")
	t.Setenv("READERD_RPC_CLIENT_ID", "r-rpc")
	t.Setenv("READERD_EVENT_ID", "TrakRF mqtt-rpc Event")
}

func TestReconcileDefaultsOn(t *testing.T) {
	brokerEnv(t)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.Reconcile {
		t.Error("Reconcile should default on")
	}
}

func TestReconcileCanDisable(t *testing.T) {
	brokerEnv(t)
	t.Setenv("READERD_RECONCILE", "false")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Reconcile {
		t.Error("READERD_RECONCILE=false must disable reconcile")
	}
}

func TestReconcileInvalidErrors(t *testing.T) {
	brokerEnv(t)
	t.Setenv("READERD_RECONCILE", "notabool")
	if _, err := LoadConfig(); err == nil {
		t.Error("invalid READERD_RECONCILE must error")
	}
}
