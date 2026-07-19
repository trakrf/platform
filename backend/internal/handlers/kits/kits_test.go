package kits

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/kit"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

type mockKitStorage struct {
	commissionResult *kit.Kit
	commissionErr    error
	verifyResult     *kit.VerifyResponse
	verifyErr        error
	listResult       []kit.KitSummary
	listErr          error
	getResult        *kit.Kit
	getErr           error

	gotQuery     string
	gotMemberEPC string
	gotEPCs      []string
}

func (m *mockKitStorage) CommissionKit(ctx context.Context, orgID int, req kit.CommissionRequest) (*kit.Kit, error) {
	return m.commissionResult, m.commissionErr
}

func (m *mockKitStorage) VerifyKits(ctx context.Context, orgID int, epcs []string) (*kit.VerifyResponse, error) {
	m.gotEPCs = epcs
	return m.verifyResult, m.verifyErr
}

func (m *mockKitStorage) ListKits(ctx context.Context, orgID int, query, memberEPC string) ([]kit.KitSummary, error) {
	m.gotQuery = query
	m.gotMemberEPC = memberEPC
	return m.listResult, m.listErr
}

func (m *mockKitStorage) GetKitByID(ctx context.Context, orgID, kitID int) (*kit.Kit, error) {
	return m.getResult, m.getErr
}

func newRequest(t *testing.T, method, target string, body any) *http.Request {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	orgID := 42
	claims := &jwt.Claims{UserID: 1, Email: "test@example.com", CurrentOrgID: &orgID}
	return req.WithContext(context.WithValue(req.Context(), middleware.UserClaimsKey, claims))
}

func commissionBody(members ...kit.CommissionMemberRequest) kit.CommissionRequest {
	return kit.CommissionRequest{Label: "1184015", Members: members}
}

func TestCreate_Happy(t *testing.T) {
	role := "coupon"
	mock := &mockKitStorage{commissionResult: &kit.Kit{
		ID: 7, Label: "1184015", Status: kit.StatusActive,
		Members: []kit.Member{{AssetID: 1, Role: &role, Name: "1184015 coupon", EPCs: []string{"AAA1"}}},
	}}
	h := NewHandler(mock)

	req := newRequest(t, http.MethodPost, "/api/v1/kits", commissionBody(
		kit.CommissionMemberRequest{EPC: "AAA1", Role: &role},
		kit.CommissionMemberRequest{EPC: "AAA2"},
	))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/api/v1/kits/7" {
		t.Errorf("Location header: %q", loc)
	}
	var resp kit.KitResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad response: %v", err)
	}
	if resp.Data.ID != 7 || len(resp.Data.Members) != 1 {
		t.Errorf("unexpected payload: %+v", resp.Data)
	}
}

func TestCreate_RequiresTwoMembers(t *testing.T) {
	h := NewHandler(&mockKitStorage{})
	req := newRequest(t, http.MethodPost, "/api/v1/kits", commissionBody(
		kit.CommissionMemberRequest{EPC: "AAA1"},
	))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for 1 member, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreate_ConflictNamesKitLabel(t *testing.T) {
	mock := &mockKitStorage{commissionErr: &kit.ConflictError{AssetName: "1184015 coupon", KitLabel: "1184015"}}
	h := NewHandler(mock)
	req := newRequest(t, http.MethodPost, "/api/v1/kits", commissionBody(
		kit.CommissionMemberRequest{EPC: "AAA1"},
		kit.CommissionMemberRequest{EPC: "AAA2"},
	))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "1184015") {
		t.Errorf("409 detail must name the owning kit label: %s", rec.Body.String())
	}
}

func TestCreate_StorageValidationErrorIs400(t *testing.T) {
	mock := &mockKitStorage{commissionErr: &kit.ValidationError{Detail: `duplicate member epc "AAA1"`}}
	h := NewHandler(mock)
	req := newRequest(t, http.MethodPost, "/api/v1/kits", commissionBody(
		kit.CommissionMemberRequest{EPC: "AAA1"},
		kit.CommissionMemberRequest{EPC: "AAA1"},
	))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestVerify_FrozenTopLevelShape(t *testing.T) {
	mock := &mockKitStorage{verifyResult: &kit.VerifyResponse{
		Kits:        []kit.VerifyKitResult{{KitID: 7, Label: "1184015", Result: kit.ResultComplete, Seen: []kit.VerifySeenMember{}, Missing: []kit.VerifyMissingMember{}}},
		Unexpected:  []kit.VerifyUnexpected{},
		UnknownEPCs: []string{"ZZZZ"},
	}}
	h := NewHandler(mock)
	req := newRequest(t, http.MethodPost, "/api/v1/kits/verify", kit.VerifyRequest{EPCs: []string{"AAA1", "ZZZZ"}})
	rec := httptest.NewRecorder()
	h.Verify(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	for _, key := range []string{"kits", "unexpected", "unknown_epcs"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("frozen contract requires top-level %q: %s", key, rec.Body.String())
		}
	}
	if _, ok := raw["data"]; ok {
		t.Errorf("verify response must NOT be wrapped in a data envelope: %s", rec.Body.String())
	}
	if len(mock.gotEPCs) != 2 {
		t.Errorf("epcs must pass through: %v", mock.gotEPCs)
	}
}

func TestVerify_EmptyEPCsRejected(t *testing.T) {
	h := NewHandler(&mockKitStorage{})
	req := newRequest(t, http.MethodPost, "/api/v1/kits/verify", kit.VerifyRequest{EPCs: []string{}})
	rec := httptest.NewRecorder()
	h.Verify(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty epcs, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestList_PassesFilters(t *testing.T) {
	mock := &mockKitStorage{listResult: []kit.KitSummary{}}
	h := NewHandler(mock)
	req := newRequest(t, http.MethodGet, "/api/v1/kits?query=8401&member_epc=AAA1", nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if mock.gotQuery != "8401" || mock.gotMemberEPC != "AAA1" {
		t.Errorf("filters not passed: query=%q member_epc=%q", mock.gotQuery, mock.gotMemberEPC)
	}
	if !strings.Contains(rec.Body.String(), `"data"`) {
		t.Errorf("list must use the data envelope: %s", rec.Body.String())
	}
}

func TestGet_NotFound(t *testing.T) {
	h := NewHandler(&mockKitStorage{getResult: nil})
	router := chi.NewRouter()
	router.Get("/api/v1/kits/{kit_id}", h.Get)
	req := newRequest(t, http.MethodGet, "/api/v1/kits/12345", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGet_BadID(t *testing.T) {
	h := NewHandler(&mockKitStorage{})
	router := chi.NewRouter()
	router.Get("/api/v1/kits/{kit_id}", h.Get)
	req := newRequest(t, http.MethodGet, "/api/v1/kits/not-a-number", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestStorageErrorIs500(t *testing.T) {
	h := NewHandler(&mockKitStorage{verifyErr: errors.New("boom")})
	req := newRequest(t, http.MethodPost, "/api/v1/kits/verify", kit.VerifyRequest{EPCs: []string{"AAA1"}})
	rec := httptest.NewRecorder()
	h.Verify(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
