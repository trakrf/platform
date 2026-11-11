package cs463

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

const probeSecret = "pr0be-secret"

// Bodies captured from a healthy CS463 (Main Entrance, 2026-11-08).
const (
	inputReply = `<?xml version="1.0" encoding="UTF-8" standalone="no"?><CSL><Command>directIOInput</Command><Input input_logic_list="0,0,0,0" port_list="1,2,3,4"/></CSL>`
	errorReply = `<?xml version="1.0" encoding="UTF-8" standalone="no"?><CSL><Command>directIOInput</Command><Error code="-10" msg="Password not corrent!"/></CSL>`
)

func probeServer(t *testing.T, status int, body string, seen *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			*seen = r.URL.RawQuery
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestProbeAPI_Classification(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"input reply is healthy", 200, inputReply, readerrpc.APIHealthy},
		{"empty 200 is the wedge", 200, "", readerrpc.APIDegraded},
		{"html 200 is degraded", 200, "<html><body>Login</body></html>", readerrpc.APIDegraded},
		{"another command echoed is degraded", 200, `<CSL><Command>login</Command><Ack>OK</Ack></CSL>`, readerrpc.APIDegraded},
		{"no Input and no Error is degraded", 200, `<CSL><Command>directIOInput</Command></CSL>`, readerrpc.APIDegraded},
		{"502 from nginx is unreachable", 502, "<html>Bad Gateway</html>", readerrpc.APIUnreachable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := probeServer(t, c.status, c.body, nil)
			got, detail := New(srv.URL, "trakrf", probeSecret, 0).ProbeAPI(context.Background())
			if got != c.want {
				t.Errorf("state = %q (detail %q), want %q", got, detail, c.want)
			}
			if got != readerrpc.APIHealthy && detail == "" {
				t.Error("an unhealthy probe must say why")
			}
		})
	}
}

// A wrong password is answered with an <Error>: the servlet is processing requests,
// so this is not a wedge, and nothing a restart or reboot does can fix it.
func TestProbeAPI_ErrorReplyIsHealthy(t *testing.T) {
	srv := probeServer(t, 200, errorReply, nil)
	got, detail := New(srv.URL, "trakrf", "wrong", 0).ProbeAPI(context.Background())
	if got != readerrpc.APIHealthy {
		t.Fatalf("state = %q, want healthy", got)
	}
	if !strings.Contains(detail, "Password not corrent") {
		t.Errorf("detail = %q, want the reader's error message", detail)
	}
}

func TestProbeAPI_ClosedPortIsUnreachable(t *testing.T) {
	got, detail := New(closedURL(t), "trakrf", probeSecret, 0).ProbeAPI(context.Background())
	if got != readerrpc.APIUnreachable {
		t.Fatalf("state = %q, want unreachable", got)
	}
	if strings.Contains(detail, probeSecret) {
		t.Fatalf("detail leaks the password: %q", detail)
	}
}

// The probe must be sessionless: a login takes the reader's single session slot.
func TestProbeAPI_IsSessionlessDirectIOInput(t *testing.T) {
	var query string
	srv := probeServer(t, 200, inputReply, &query)
	New(srv.URL, "trakrf", probeSecret, 0).ProbeAPI(context.Background())
	if !strings.Contains(query, "command=directIOInput") {
		t.Errorf("query = %q, want command=directIOInput", query)
	}
	if strings.Contains(query, "login") || strings.Contains(query, "session_id") {
		t.Errorf("query = %q: the probe must never log in", query)
	}
}

func TestProbeAPI_DetailNeverCarriesPassword(t *testing.T) {
	for _, body := range []string{"", "<html>" + probeSecret + "</html>", errorReply} {
		srv := probeServer(t, 200, body, nil)
		_, detail := New(srv.URL, "trakrf", probeSecret, 0).ProbeAPI(context.Background())
		if strings.Contains(detail, probeSecret) {
			t.Errorf("body %q: detail leaks the password: %q", body, detail)
		}
	}
}

func TestAdapterProbeHealth_Delegates(t *testing.T) {
	srv := probeServer(t, 200, "", nil)
	a := NewAdapter(New(srv.URL, "trakrf", probeSecret, 0), AdapterConfig{AntennaCount: 4})
	if got, _ := a.ProbeHealth(context.Background()); got != readerrpc.APIDegraded {
		t.Errorf("ProbeHealth = %q, want degraded", got)
	}
}
