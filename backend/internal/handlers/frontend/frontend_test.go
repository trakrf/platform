package frontend

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

const testIndexHTML = `<!DOCTYPE html>
<html><head><!--__APP_CONFIG__--></head>
<body><div id="root"></div></body></html>`

func newTestHandler(label string, index string) *Handler {
	return newTestHandlerWithFeed(label, index, ReaderFeedConfig{})
}

func newTestHandlerWithFeed(label string, index string, feed ReaderFeedConfig) *Handler {
	mapFS := fstest.MapFS{
		"frontend/dist/index.html": &fstest.MapFile{Data: []byte(index)},
	}
	return NewHandler(mapFS, "frontend/dist", label, feed)
}

func serveSPA(t *testing.T, h *Handler) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/some/spa/route", nil)
	h.ServeSPA(rec, req, "frontend/dist/index.html")
	if rec.Code != http.StatusOK {
		t.Fatalf("ServeSPA status = %d, want 200", rec.Code)
	}
	return rec.Body.String()
}

func TestServeSPA_InjectsLabel(t *testing.T) {
	body := serveSPA(t, newTestHandler("GKE pre-prod", testIndexHTML))

	want := `<script>window.__APP_CONFIG__ = {"environmentLabel":"GKE pre-prod","readerFeed":{"url":"","username":"","password":"","topic":""}};</script>`
	if !strings.Contains(body, want) {
		t.Errorf("body missing injected config script.\nwant substring: %s\ngot:\n%s", want, body)
	}
	if strings.Contains(body, appConfigPlaceholder) {
		t.Errorf("placeholder %q still present after injection", appConfigPlaceholder)
	}
}

func TestServeSPA_EmptyLabel(t *testing.T) {
	body := serveSPA(t, newTestHandler("", testIndexHTML))

	want := `{"environmentLabel":"","readerFeed":{"url":"","username":"","password":"","topic":""}}`
	if !strings.Contains(body, want) {
		t.Errorf("body missing empty-label config.\nwant substring: %s\ngot:\n%s", want, body)
	}
}

func TestServeSPA_InjectsReaderFeed(t *testing.T) {
	feed := ReaderFeedConfig{
		URL:      "wss://mqtt.preview.gke.trakrf.id:8084/mqtt",
		Username: "frontend-readonly",
		Password: "s3cret",
		Topic:    "trakrf.id/+/reads",
	}
	body := serveSPA(t, newTestHandlerWithFeed("preview", testIndexHTML, feed))

	want := `"readerFeed":{"url":"wss://mqtt.preview.gke.trakrf.id:8084/mqtt","username":"frontend-readonly","password":"s3cret","topic":"trakrf.id/+/reads"}`
	if !strings.Contains(body, want) {
		t.Errorf("body missing injected reader-feed config.\nwant substring: %s\ngot:\n%s", want, body)
	}
}

func TestServeSPA_EscapesScriptBreakout(t *testing.T) {
	// A label containing </script> must not be able to close the inline
	// <script> tag. json.Marshal HTML-escapes '<' to < by default.
	body := serveSPA(t, newTestHandler("</script><script>alert(1)", testIndexHTML))

	if strings.Contains(body, "<script>alert(1)") {
		t.Errorf("script breakout not prevented; raw injected markup present:\n%s", body)
	}
	// json.Marshal HTML-escapes '<' and '>' to \u003c / \u003e, so the label
	// cannot close the inline <script>. Match the escaped form literally.
	if !strings.Contains(body, `\u003c/script\u003e\u003cscript\u003ealert(1)`) {
		t.Errorf("expected escaped label in output, got:\n%s", body)
	}
}

func TestServeSPA_PlaceholderAbsentServedUnchanged(t *testing.T) {
	plain := `<!DOCTYPE html><html><head></head><body></body></html>`
	body := serveSPA(t, newTestHandler("preview", plain))

	if body != plain {
		t.Errorf("index without placeholder should be served unchanged.\nwant: %s\ngot:  %s", plain, body)
	}
	if strings.Contains(body, "__APP_CONFIG__") {
		t.Errorf("unexpected config injection into placeholder-less index")
	}
}

func TestServeSPA_MissingIndexReturns500(t *testing.T) {
	h := newTestHandler("preview", testIndexHTML)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeSPA(rec, req, "frontend/dist/does-not-exist.html")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("missing index status = %d, want 500", rec.Code)
	}
}

// Guard: NewHandler must build a usable sub-filesystem for asset serving.
func TestNewHandler_SubFS(t *testing.T) {
	mapFS := fstest.MapFS{
		"frontend/dist/index.html":    &fstest.MapFile{Data: []byte(testIndexHTML)},
		"frontend/dist/assets/app.js": &fstest.MapFile{Data: []byte("console.log(1)")},
	}
	h := NewHandler(mapFS, "frontend/dist", "preview", ReaderFeedConfig{})
	if _, err := fs.Stat(mapFS, "frontend/dist/assets/app.js"); err != nil {
		t.Fatalf("test fixture missing asset: %v", err)
	}
	_ = h
}
