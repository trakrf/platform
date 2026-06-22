package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFormatAPIError(t *testing.T) {
	body := []byte(`{"error":{"type":"https://trakrf.id/errors/unauthorized","title":"unauthorized","detail":"invalid client credentials","status":401,"instance":"/api/v1/oauth/token","request_id":"req_123"}}`)
	err := formatAPIError(401, body)
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	for _, want := range []string{"401", "unauthorized", "invalid client credentials", "req_123"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
}

func TestFormatAPIErrorNonJSONFallback(t *testing.T) {
	err := formatAPIError(503, []byte("upstream down"))
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("want a 503 error, got %v", err)
	}
}

func TestMintSuccess(t *testing.T) {
	var gotBody, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/oauth/token" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		gotCT = r.Header.Get("Content-Type")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"the-jwt","token_type":"Bearer","expires_in":900,"refresh_token":"rt"}`))
	}))
	defer srv.Close()

	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	m := &Minter{BaseURL: srv.URL, Now: func() time.Time { return now }}

	tok, err := m.Mint(context.Background(), "cid", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "the-jwt" {
		t.Fatalf("access token = %q, want the-jwt", tok.AccessToken)
	}
	if want := now.Add(900 * time.Second); !tok.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", tok.ExpiresAt, want)
	}
	if !strings.Contains(gotBody, "client_credentials") || !strings.Contains(gotBody, "cid") {
		t.Fatalf("request body missing grant/credentials: %q", gotBody)
	}
	if !strings.Contains(gotCT, "application/json") {
		t.Fatalf("content-type = %q, want json", gotCT)
	}
}

func TestMintRejectsBadCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"type":"x","title":"unauthorized","detail":"bad creds","status":401,"instance":"/","request_id":"r1"}}`))
	}))
	defer srv.Close()

	m := &Minter{BaseURL: srv.URL}
	if _, err := m.Mint(context.Background(), "cid", "nope"); err == nil {
		t.Fatal("want error on 401")
	} else if !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("error should surface the envelope title, got %v", err)
	}
}
