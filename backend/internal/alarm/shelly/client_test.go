package shelly

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_Set_SendsSwitchSetRPC(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"result":{"was_on":false}}`))
	}))
	defer srv.Close()

	c := New(2 * time.Second)
	if err := c.Set(context.Background(), srv.URL, 2, true); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/rpc" {
		t.Errorf("path = %q, want /rpc", gotPath)
	}
	if gotBody["method"] != "Switch.Set" {
		t.Errorf("method field = %v, want Switch.Set", gotBody["method"])
	}
	params, ok := gotBody["params"].(map[string]any)
	if !ok {
		t.Fatalf("params not an object: %v", gotBody["params"])
	}
	// JSON numbers decode to float64.
	if params["id"] != float64(2) {
		t.Errorf("params.id = %v, want 2", params["id"])
	}
	if params["on"] != true {
		t.Errorf("params.on = %v, want true", params["on"])
	}
}

func TestClient_Set_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(2 * time.Second)
	if err := c.Set(context.Background(), srv.URL, 0, false); err == nil {
		t.Fatal("expected error on non-2xx, got nil")
	}
}

func TestClient_Set_TimeoutIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(10 * time.Millisecond)
	if err := c.Set(context.Background(), srv.URL, 0, true); err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
