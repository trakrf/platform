package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// fakeAPI is a minimal stand-in for the TrakRF API exercising the token grant
// plus a couple of read endpoints, recording what the CLI actually sent.
type fakeAPI struct {
	mu        sync.Mutex
	authSeen  string
	listQuery string
}

func (f *fakeAPI) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"the-jwt","token_type":"Bearer","expires_in":900,"refresh_token":"rt"}`))
	})
	mux.HandleFunc("/api/v1/assets", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.authSeen = r.Header.Get("Authorization")
		f.listQuery = r.URL.RawQuery
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":7,"external_key":"ABC","name":"Forklift","is_active":true,"description":null,"valid_from":"2026-01-01T00:00:00Z"}],"limit":50,"offset":0,"total_count":1}`))
	})
	return mux
}

// runCLI runs the command tree with the given args, capturing stdout.
func runCLI(t *testing.T, env map[string]string, args ...string) (string, error) {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}

	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	err := NewApp("test").Run(context.Background(), append([]string{"trakrf"}, args...))

	_ = w.Close()
	os.Stdout = orig
	return <-done, err
}

func TestAssetsListEndToEnd(t *testing.T) {
	api := &fakeAPI{}
	srv := httptest.NewServer(api.handler(t))
	defer srv.Close()

	env := map[string]string{
		"TRAKRF_API_URL":     srv.URL,
		"TRAKRF_API_KEY":     "cid:secret",
		"TRAKRF_CONFIG_HOME": t.TempDir(),
	}

	out, err := runCLI(t, env, "--json", "assets", "list", "--limit", "50", "--search", "fork")
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// The minted bearer token must have reached the assets endpoint.
	if api.authSeen != "Bearer the-jwt" {
		t.Fatalf("Authorization = %q, want Bearer the-jwt", api.authSeen)
	}
	// Flags must have been translated into query params.
	if !strings.Contains(api.listQuery, "limit=50") || !strings.Contains(api.listQuery, "q=fork") {
		t.Fatalf("query = %q, want limit=50 & q=fork", api.listQuery)
	}
	// JSON output must carry the asset.
	var payload struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
		TotalCount int `json:"total_count"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if len(payload.Data) != 1 || payload.Data[0].Name != "Forklift" {
		t.Fatalf("unexpected payload: %s", out)
	}
}

func TestAssetsListTableEndToEnd(t *testing.T) {
	api := &fakeAPI{}
	srv := httptest.NewServer(api.handler(t))
	defer srv.Close()

	env := map[string]string{
		"TRAKRF_API_URL":     srv.URL,
		"TRAKRF_API_KEY":     "cid:secret",
		"TRAKRF_CONFIG_HOME": t.TempDir(),
	}

	out, err := runCLI(t, env, "assets", "list")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{"EXTERNAL_KEY", "Forklift", "ABC"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table output missing %q:\n%s", want, out)
		}
	}
}

func TestUnknownFormatErrors(t *testing.T) {
	env := map[string]string{
		"TRAKRF_API_KEY":     "cid:secret",
		"TRAKRF_CONFIG_HOME": t.TempDir(),
	}
	if _, err := runCLI(t, env, "--format", "xml", "assets", "list"); err == nil {
		t.Fatal("want error for unknown format")
	}
}

func TestAssetsGetRejectsNonNumericID(t *testing.T) {
	env := map[string]string{
		"TRAKRF_API_KEY":     "cid:secret",
		"TRAKRF_CONFIG_HOME": t.TempDir(),
	}
	_, err := runCLI(t, env, "assets", "get", "ABC123")
	if err == nil || !strings.Contains(err.Error(), "external-key") {
		t.Fatalf("want a helpful non-numeric id error, got %v", err)
	}
}
