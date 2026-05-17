//go:build integration
// +build integration

// TRA-765 (BB56 F1): URLs emitted in spec-level metadata
// (`info.description`, `info.contact.url`, `servers[].url`) appear in
// Redoc, Swagger UI, every generated SDK's docstring/JSDoc, and any
// LLM-assisted client tooling that ingests the spec. A drifted URL on
// this surface is functionally identical to a drifted URL in a per-
// request error envelope — the class TRA-748 (BB45 F1) caught and
// guarded for error envelopes. BB56 surfaced that spec metadata is a
// separate emission surface needing the same guard: F1 here was the
// info.description carrying `/api/http-method-coverage` (404 on both
// origins) where the Docusaurus location is `/docs/api/
// http-method-coverage`.
//
// This test loads the embedded public spec, walks the metadata URL
// surface, and asserts each emitted URL resolves to 200 on the docs
// origin (which canonically publishes the Redoc page and serves the
// /api/openapi.{yaml,json} 302 redirect). Site-relative paths are
// joined against the docs origin; absolute URLs are fetched as-is.
// Drift on either side — service template literal change or docs site
// page move — fails this test.

package swaggerspec

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// docsOrigin is the canonical host for site-relative URLs emitted in
// spec metadata. The published spec lives on docs.trakrf.id; resolving
// against that origin mirrors what a reader of the Redoc page sees when
// they click a site-relative link.
const docsOrigin = "https://docs.trakrf.id"

// metadataURLRegex matches absolute http(s) URLs and site-relative
// paths under `/api/` or `/docs/`. Site-relative paths under other
// roots are deliberately not matched — info.description does not
// emit them today and any future addition should explicitly opt in
// here (or be flagged by the allowlist below if a URL gets added
// outside these prefixes).
var metadataURLRegex = regexp.MustCompile(`https?://[^\s\)\]\"]+|/(?:api|docs)/[A-Za-z0-9._/?=&%+-]+`)

// trailingPunct trims punctuation that markdown prose pulls in when the
// URL appears mid-sentence (period, comma, semicolon, colon, closing
// paren or bracket).
var trailingPunct = ".,;:)]\""

func TestPublicSpecMetadataURLsResolve(t *testing.T) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(publicYAML)
	require.NoError(t, err, "embedded public spec must parse")
	require.NotNil(t, doc.Info, "spec must have info block")

	urls := collectMetadataURLs(doc)
	require.NotEmpty(t, urls, "expected at least one URL in spec metadata; if metadata stopped emitting URLs, retire this test rather than silently passing on an empty set")

	client := &http.Client{Timeout: 10 * time.Second}

	for _, u := range urls {
		u := u
		t.Run(u, func(t *testing.T) {
			target := resolveAgainstDocsOrigin(u)
			req, err := http.NewRequest(http.MethodGet, target, nil)
			require.NoError(t, err)
			req.Header.Set("User-Agent", "trakrf-platform-spec-metadata-test/1.0")

			resp, err := client.Do(req)
			require.NoErrorf(t, err, "fetch %s (resolved %s)", u, target)
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, resp.Body)

			assert.Equalf(t, http.StatusOK, resp.StatusCode,
				"metadata URL %s (resolved %s) must return 200; spec metadata surfaces in Redoc, Swagger UI, generated SDK docstrings, and LLM-ingested specs — a 404 here is a contract failure (TRA-748 class for error envelopes; TRA-765 / BB56 F1 for spec metadata)",
				u, target)
		})
	}
}

func collectMetadataURLs(doc *openapi3.T) []string {
	seen := map[string]struct{}{}
	var out []string

	add := func(raw string) {
		for _, u := range metadataURLRegex.FindAllString(raw, -1) {
			u = strings.TrimRight(u, trailingPunct)
			if _, ok := seen[u]; ok {
				continue
			}
			seen[u] = struct{}{}
			out = append(out, u)
		}
	}

	add(doc.Info.Description)
	if doc.Info.Contact != nil && doc.Info.Contact.URL != "" {
		add(doc.Info.Contact.URL)
	}
	if doc.Info.License != nil && doc.Info.License.URL != "" {
		add(doc.Info.License.URL)
	}
	for _, srv := range doc.Servers {
		if srv != nil && srv.URL != "" {
			add(srv.URL)
		}
	}
	return out
}

func resolveAgainstDocsOrigin(u string) string {
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	return docsOrigin + u
}
