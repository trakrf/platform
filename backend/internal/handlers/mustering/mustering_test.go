package mustering

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/muster"
	mustering "github.com/trakrf/platform/backend/internal/mustering"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// ── fakes ───────────────────────────────────────────────────────────────────────

type fakeEngine struct {
	status      mustering.SnapshotPayload
	activateErr error
	activated   *muster.Event
	allClear    *muster.Event
	cancelled   *muster.Event
	verifyErr   error
	verifyEntry *muster.Entry
	verifyCnts  *muster.Counts
	unlockCalls int
	lastEmail   string
}

func (f *fakeEngine) Status(_ context.Context, _ int) (mustering.SnapshotPayload, error) {
	return f.status, nil
}
func (f *fakeEngine) Activate(_ context.Context, _, _, _ int) (*muster.Event, error) {
	if f.activateErr != nil {
		return nil, f.activateErr
	}
	return f.activated, nil
}
func (f *fakeEngine) AllClear(_ context.Context, _, _, _ int) (*muster.Event, error) {
	return f.allClear, nil
}
func (f *fakeEngine) Cancel(_ context.Context, _, _, _ int) (*muster.Event, error) {
	return f.cancelled, nil
}
func (f *fakeEngine) Verify(_ context.Context, _, _, _, _ int) (*muster.Entry, *muster.Counts, error) {
	return f.verifyEntry, f.verifyCnts, f.verifyErr
}
func (f *fakeEngine) MarkSafe(_ context.Context, _, _, _, _ int, _ string) (*muster.Entry, *muster.Counts, error) {
	return f.verifyEntry, f.verifyCnts, f.verifyErr
}
func (f *fakeEngine) Unlock(_ context.Context, _, _, _ int, email string) error {
	f.unlockCalls++
	f.lastEmail = email
	return nil
}

type fakeBroadcaster struct {
	ch chan mustering.Event
}

func (f *fakeBroadcaster) Subscribe(_ int) (<-chan mustering.Event, func()) {
	if f.ch == nil {
		f.ch = make(chan mustering.Event, 8)
	}
	return f.ch, func() {}
}

// routerFor builds a chi router with the handler routes and injects session
// claims (org 1, user 99) into every request.
func routerFor(h *Handler) http.Handler {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		claims := &jwt.Claims{UserID: 99, Email: "op@example.com", CurrentOrgID: intPtr(1)}
		ctx := middleware.WithUserClaimsForTest(req.Context(), claims)
		r.ServeHTTP(w, req.WithContext(ctx))
	})
}

func intPtr(i int) *int { return &i }

func newHandler(e musterEngine, b musterBroadcaster) *Handler {
	return newHandlerForTest(e, b)
}

// ── REST tests ───────────────────────────────────────────────────────────────────

func TestStatus_OK(t *testing.T) {
	eng := &fakeEngine{status: mustering.SnapshotPayload{PersonsOnSite: 7}}
	srv := httptest.NewServer(routerFor(newHandler(eng, &fakeBroadcaster{})))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/mustering/status")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body mustering.SnapshotPayload
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, 7, body.PersonsOnSite)
}

func TestCreateEvent_Conflict409(t *testing.T) {
	eng := &fakeEngine{activateErr: muster.ErrActiveEventExists{}}
	srv := httptest.NewServer(routerFor(newHandler(eng, &fakeBroadcaster{})))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/mustering/events", "application/json", strings.NewReader(`{"window_minutes":15}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestCreateEvent_Created(t *testing.T) {
	eng := &fakeEngine{activated: &muster.Event{ID: 42, Status: "active"}}
	srv := httptest.NewServer(routerFor(newHandler(eng, &fakeBroadcaster{})))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/mustering/events", "application/json", strings.NewReader(`{"window_minutes":15}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestPatchEntry_InvalidTransition409(t *testing.T) {
	eng := &fakeEngine{verifyErr: muster.ErrInvalidTransition{Current: "missing", Action: "verify"}}
	srv := httptest.NewServer(routerFor(newHandler(eng, &fakeBroadcaster{})))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/mustering/events/100/entries/200",
		strings.NewReader(`{"action":"verify"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestPatchEntry_NotFound404(t *testing.T) {
	eng := &fakeEngine{} // verifyEntry nil → 404
	srv := httptest.NewServer(routerFor(newHandler(eng, &fakeBroadcaster{})))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/mustering/events/100/entries/200",
		strings.NewReader(`{"action":"mark_safe","note":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestPatchEntry_UnknownAction400(t *testing.T) {
	eng := &fakeEngine{}
	srv := httptest.NewServer(routerFor(newHandler(eng, &fakeBroadcaster{})))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/mustering/events/100/entries/200",
		strings.NewReader(`{"action":"frobnicate"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPatchEntry_VerifyOK(t *testing.T) {
	eng := &fakeEngine{
		verifyEntry: &muster.Entry{ID: 200, Status: "verified"},
		verifyCnts:  &muster.Counts{Verified: 1},
	}
	srv := httptest.NewServer(routerFor(newHandler(eng, &fakeBroadcaster{})))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/mustering/events/100/entries/200",
		strings.NewReader(`{"action":"verify"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestStatus_MissingOrgContext(t *testing.T) {
	eng := &fakeEngine{}
	h := newHandler(eng, &fakeBroadcaster{})
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	// No claims injected → 422 missing org context.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mustering/status", nil)
	r.ServeHTTP(rr, req)
	require.Equal(t, http.StatusUnprocessableEntity, rr.Code)
}

// ── SSE stream test ───────────────────────────────────────────────────────────────

func TestStream_SnapshotAndDelta(t *testing.T) {
	eng := &fakeEngine{status: mustering.SnapshotPayload{PersonsOnSite: 3}}
	bc := &fakeBroadcaster{ch: make(chan mustering.Event, 8)}
	h := newHandler(eng, bc)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := &jwt.Claims{UserID: 99, Email: "op@example.com", CurrentOrgID: intPtr(1)}
		ctx := middleware.WithUserClaimsForTest(r.Context(), claims)
		h.Stream(w, r.WithContext(ctx))
	}))
	srv.Config.WriteTimeout = 500 * time.Millisecond // would kill the stream if not cleared
	srv.Start()
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream"))

	sc := bufio.NewScanner(resp.Body)
	// Expect a snapshot frame first.
	sawSnapshot := false
	deadline := time.After(2 * time.Second)
	for !sawSnapshot {
		select {
		case <-deadline:
			t.Fatal("no snapshot frame")
		default:
		}
		if !sc.Scan() {
			t.Fatalf("stream closed early: %v", sc.Err())
		}
		if strings.Contains(sc.Text(), `event: snapshot`) {
			sawSnapshot = true
		}
	}

	// Past the WriteTimeout window, push a delta — must arrive (deadline cleared).
	time.Sleep(700 * time.Millisecond)
	bc.ch <- mustering.Event{Type: mustering.EventEntry, Data: []byte(`{"entry":{"id":1}}`)}

	deadline = time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("delta did not arrive after WriteTimeout window")
		default:
		}
		if !sc.Scan() {
			t.Fatalf("stream closed early: %v", sc.Err())
		}
		if strings.Contains(sc.Text(), `event: entry`) {
			return // success
		}
	}
}
