package readstream_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sentryhttp "github.com/getsentry/sentry-go/http"

	rshandler "github.com/trakrf/platform/backend/internal/handlers/readstream"
	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/scanread"
	rs "github.com/trakrf/platform/backend/internal/services/readstream"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func orgClaims(orgID int) *jwt.Claims {
	return &jwt.Claims{UserID: 1, Email: "u@example.com", CurrentOrgID: &orgID}
}

func read(epc string) []scanread.Read {
	return []scanread.Read{{
		EPC:              epc,
		CapturePointName: "cp",
		AntennaPort:      1,
		RSSI:             -50,
		ReaderTimestamp:  time.UnixMilli(1234),
	}}
}

// TestStream_OrgFilteredFramingSurvivesWriteTimeout exercises the three things
// that matter and can't be unit-checked in isolation: SSE framing, server-side
// org filtering, and survival past the server WriteTimeout (proving the per-
// request write deadline is cleared).
func TestStream_OrgFilteredFramingSurvivesWriteTimeout(t *testing.T) {
	b := rs.New()
	h := rshandler.NewHandler(b)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := middleware.WithUserClaimsForTest(r.Context(), orgClaims(1))
		h.Stream(w, r.WithContext(ctx))
	}))
	srv.Config.WriteTimeout = 500 * time.Millisecond // would kill the stream if not cleared
	srv.Start()
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/reads/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}

	// Wait past the WriteTimeout window, then publish: a wrong-org read (must not
	// arrive) and our org's read (must arrive after the deadline would have fired).
	time.Sleep(700 * time.Millisecond)
	b.Publish(2, "trakrf.id/other/reads", read("OTHER-EPC"))
	b.Publish(1, "trakrf.id/dock-9/reads", read("EPC-1"))

	sc := bufio.NewScanner(resp.Body)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("did not receive org-1 data frame after WriteTimeout window")
		default:
		}
		if !sc.Scan() {
			t.Fatalf("stream closed early: %v", sc.Err())
		}
		line := sc.Text()
		if strings.Contains(line, "OTHER-EPC") {
			t.Fatal("received another org's read — org filtering broken")
		}
		if strings.HasPrefix(line, "data:") && strings.Contains(line, "EPC-1") {
			if !strings.Contains(line, `"readerKey":"dock-9"`) {
				t.Fatalf("data frame missing readerKey: %s", line)
			}
			return // success
		}
	}
}

// TestStream_ThroughRealWrapperChain runs the handler behind the same
// ResponseWriter-wrapping middleware as production (logger.Middleware +
// sentryhttp), which is where a non-transparent wrapper would panic the SSE
// flush or block the write-deadline clear. It proves the stream flushes and
// survives a low WriteTimeout end-to-end.
func TestStream_ThroughRealWrapperChain(t *testing.T) {
	b := rs.New()
	h := rshandler.NewHandler(b)

	inject := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := middleware.WithUserClaimsForTest(r.Context(), orgClaims(1))
		h.Stream(w, r.WithContext(ctx))
	})
	// Order mirrors router.go: logger wraps first (responseWriter), then sentry
	// wraps that (fancy writer whose Flush asserts the inner writer is a Flusher).
	chain := logger.Middleware(sentryhttp.New(sentryhttp.Options{}).Handle(inject))

	srv := httptest.NewUnstartedServer(chain)
	srv.Config.WriteTimeout = 500 * time.Millisecond
	srv.Start()
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/reads/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	time.Sleep(700 * time.Millisecond) // past WriteTimeout
	b.Publish(1, "trakrf.id/dock-9/reads", read("EPC-CHAIN"))

	sc := bufio.NewScanner(resp.Body)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("no data frame through the real wrapper chain")
		default:
		}
		if !sc.Scan() {
			t.Fatalf("stream closed early: %v", sc.Err())
		}
		if line := sc.Text(); strings.HasPrefix(line, "data:") && strings.Contains(line, "EPC-CHAIN") {
			return
		}
	}
}

func TestStream_MissingOrgContext(t *testing.T) {
	b := rs.New()
	h := rshandler.NewHandler(b)

	// No claims injected → no org context.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reads/stream", nil)
	h.Stream(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rr.Code)
	}
}
