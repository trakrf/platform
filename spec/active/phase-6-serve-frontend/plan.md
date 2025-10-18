# Implementation Plan: Phase 6 - Serve Frontend Assets
Generated: 2025-10-18
Specification: spec.md

## Understanding

This phase integrates the React frontend with the Go backend into a single deployable artifact. The Go server will embed the built frontend assets and serve them alongside the existing API routes, preparing the application for Railway/GKE deployment in Phase 7.

**Key Integration Points**:
- Go embeds `frontend/dist/` using `go:embed` directive
- Three routing tiers: API routes (existing), static assets (new), SPA catch-all (new)
- Existing auth middleware pattern extended to protect frontend routes
- Cache headers enable aggressive caching for hashed assets, no-cache for index.html
- CORS middleware updated to support 12-factor configuration
- Frontend updated with TrakRF branding and Open Graph meta tags

**Critical Design Decision**:
Frontend routes always serve `index.html` to enable React Router. The inventory/scanning functionality remains publicly accessible (shows raw tag EPCs without auth), while asset metadata, persistence, and reporting require authentication. This split is handled client-side by React, not server-side.

## Relevant Files

### Reference Patterns
- `backend/middleware.go` (lines 18-88) - Existing middleware pattern to follow for cache headers
- `backend/main.go` (lines 52-77) - Router setup and middleware application order
- `backend/main.go` (lines 71-77) - Auth middleware group pattern for protected routes
- `backend/errors.go` (lines 34-64) - JSON error response pattern for API 404s
- `frontend/vite.config.ts` (lines 116-130) - Build output configuration with manual chunks

### Files to Create
- `backend/frontend.go` - Frontend serving logic (embed FS, cache headers, SPA handler)
- `scripts/build.sh` - Production build script (frontend → backend → binary)
- `frontend/public/og-image.png` - Open Graph image (1200x630px, branded with tagline)

### Files to Modify
- `backend/main.go` (lines ~52-77) - Add frontend routes to router setup
- `backend/middleware.go` (lines 58-72) - Update CORS to use env var
- `frontend/index.html` (lines 1-37) - Add Open Graph meta tags, update title/description

## Architecture Impact

**Subsystems Affected**:
- Backend (Go routing + embedded filesystem)
- Frontend (meta tags + build integration)
- Build system (new build script)

**New Dependencies**:
- None (using Go stdlib: `embed`, `io/fs`, `net/http`)

**Breaking Changes**:
- None (additive only - existing API routes unchanged)
- CORS behavior changes: respects `BACKEND_CORS_ORIGIN` env var (defaults to `*` for compatibility)

**Deployment Impact**:
- Production: Single binary artifact (backend + embedded frontend)
- Development: Unchanged (Vite dev server on :5173, Go backend on :8080 with CORS)

## Task Breakdown

### Task 1: Create Frontend Serving Infrastructure
**File**: `backend/frontend.go`
**Action**: CREATE
**Pattern**: Reference `backend/middleware.go` for middleware wrapping pattern

**Implementation**:
```go
package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed frontend/dist
var frontendFS embed.FS

// serveFrontend returns an http.Handler that serves embedded frontend assets
// with appropriate cache headers for production
func serveFrontend() http.Handler {
	// Strip "frontend/dist" prefix to serve files from root
	subFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		panic("failed to create sub filesystem: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(subFS))

	// Wrap with cache control middleware
	return cacheControlMiddleware(fileServer)
}

// cacheControlMiddleware applies cache headers based on asset type
// - index.html: no-cache (always fresh for updated asset references)
// - /assets/*: 1 year immutable (content-hashed filenames)
// - other files: 1 hour moderate cache
func cacheControlMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// index.html and SPA routes: NO cache (must check for new asset hashes)
		if path == "/" || path == "/index.html" || !strings.Contains(path, ".") {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		} else if strings.HasPrefix(path, "/assets/") {
			// Hashed assets: LONG cache (1 year immutable)
			// Safe because Vite generates new filename when content changes
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			// Other static files (favicon.ico, icons, etc.): moderate cache
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}

		next.ServeHTTP(w, r)
	})
}

// spaHandler serves index.html for all frontend routes
// This enables React Router to handle client-side routing
func spaHandler(w http.ResponseWriter, r *http.Request) {
	// Read index.html from embedded filesystem
	indexHTML, err := frontendFS.ReadFile("frontend/dist/index.html")
	if err != nil {
		// Should never happen - embedded at build time
		http.Error(w, "Frontend assets not found", http.StatusInternalServerError)
		return
	}

	// Apply no-cache headers for index.html
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	w.WriteHeader(http.StatusOK)
	w.Write(indexHTML)
}
```

**Validation**:
- Run backend tests: `just backend-test`
- Verify no compilation errors: `cd backend && go build .`

---

### Task 2: Update CORS Middleware for 12-Factor Configuration
**File**: `backend/middleware.go`
**Action**: MODIFY (lines 58-72)
**Pattern**: Use `os.Getenv()` pattern from `main.go:29-32`

**Implementation**:
Replace the `corsMiddleware` function:

```go
// corsMiddleware handles CORS headers
// Configurable via BACKEND_CORS_ORIGIN env var (12-factor)
// If not set, defaults to "*" for development compatibility
// In production with same-origin frontend serving, CORS is not needed
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := os.Getenv("BACKEND_CORS_ORIGIN")
		if origin == "" {
			// Default to allow all origins (development mode)
			origin = "*"
		}

		// Only apply CORS headers if configured
		// In production serving frontend from same origin, this can be disabled
		// by not setting BACKEND_CORS_ORIGIN
		if origin != "disabled" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "3600")
		}

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
```

**Note**: Need to add `import "os"` at top of file if not already present.

**Validation**:
- Run backend tests: `just backend-test`
- Verify CORS behavior with curl:
  ```bash
  # Default (no env var) - should allow *
  curl -i http://localhost:8080/api/v1/health -H "Origin: http://example.com"

  # Explicit origin
  BACKEND_CORS_ORIGIN=https://trakrf.com curl -i http://localhost:8080/api/v1/health

  # Disabled
  BACKEND_CORS_ORIGIN=disabled curl -i http://localhost:8080/api/v1/health
  ```

---

### Task 3: Integrate Frontend Routes into Main Router
**File**: `backend/main.go`
**Action**: MODIFY (lines 52-77)
**Pattern**: Follow existing route registration pattern

**Implementation**:

After line 59 (`r.Use(contentTypeMiddleware)`), add frontend route setup:

```go
	// ============================================================================
	// Frontend & Static Asset Routes
	// ============================================================================
	// IMPORTANT: Static assets must be registered BEFORE API routes to prevent
	// the catch-all SPA handler from intercepting API requests

	frontendHandler := serveFrontend()

	// Static assets (public, no auth required)
	// These are served directly from the embedded filesystem with long cache TTLs
	r.Handle("/assets/*", frontendHandler)
	r.Handle("/favicon.ico", frontendHandler)
	r.Handle("/icon-*.png", frontendHandler) // All icon sizes
	r.Handle("/logo.png", frontendHandler)
	r.Handle("/manifest.json", frontendHandler)
	r.Handle("/og-image.png", frontendHandler)

	// ============================================================================
	// API Routes (preserve existing behavior)
	// ============================================================================
```

Then, after the existing API route registration (line ~77), add SPA catch-all:

```go
	slog.Info("Routes registered")

	// ============================================================================
	// SPA Catch-All Handler (must be LAST)
	// ============================================================================
	// Serve index.html for all remaining routes to enable React Router
	// React will handle:
	//   - Public routes: /, /login, /register (inventory without auth)
	//   - Protected routes: /dashboard, /assets, /settings (redirects to login)
	r.HandleFunc("/*", spaHandler)
```

**Critical**: Order matters!
1. Health checks (existing)
2. Frontend static assets (new)
3. API routes (existing)
4. SPA catch-all (new - MUST BE LAST)

**Validation**:
- Run backend tests: `just backend-test`
- Manually test routing order:
  ```bash
  # API routes still work
  curl -i http://localhost:8080/healthz
  curl -i http://localhost:8080/api/v1/health

  # Static assets work
  curl -I http://localhost:8080/favicon.ico
  curl -I http://localhost:8080/assets/index-*.js

  # SPA catch-all works
  curl -i http://localhost:8080/
  curl -i http://localhost:8080/dashboard
  curl -i http://localhost:8080/some-random-path
  ```

---

### Task 4: Update Frontend Meta Tags
**File**: `frontend/index.html`
**Action**: MODIFY (lines 22-25)
**Pattern**: Add Open Graph and Twitter Card meta tags

**Implementation**:

Replace lines 22-25 with:

```html
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />

    <!-- Primary Meta Tags -->
    <meta name="description" content="Real-time RFID asset tracking and inventory management platform" />
    <meta name="theme-color" content="#3b82f6" />
    <title>TrakRF - Real-Time Asset Tracking</title>

    <!-- Open Graph / Facebook -->
    <meta property="og:type" content="website" />
    <meta property="og:url" content="https://trakrf.com/" />
    <meta property="og:title" content="TrakRF - Real-Time Asset Tracking" />
    <meta property="og:description" content="Real-time RFID asset tracking and inventory management platform" />
    <meta property="og:image" content="https://trakrf.com/og-image.png" />

    <!-- Twitter -->
    <meta property="twitter:card" content="summary_large_image" />
    <meta property="twitter:url" content="https://trakrf.com/" />
    <meta property="twitter:title" content="TrakRF - Real-Time Asset Tracking" />
    <meta property="twitter:description" content="Real-time RFID asset tracking and inventory management platform" />
    <meta property="twitter:image" content="https://trakrf.com/og-image.png" />
```

Also update line 25 to remove "Handheld" (now a general platform):

**Before**: `<title>TrakRF Handheld</title>`
**After**: Already in new meta tags above

**Validation**:
- Build frontend: `cd frontend && pnpm build`
- Check dist/index.html contains new meta tags
- Verify no build errors

---

### Task 5: Create Open Graph Image
**File**: `frontend/public/og-image.png`
**Action**: CREATE
**Pattern**: 1200x630px branded graphic with tagline

**Implementation**:

Create a branded image with:
- **Dimensions**: 1200x630px (Open Graph standard)
- **Background**: TrakRF brand color gradient (blue theme from existing UI)
- **Content**:
  - TrakRF logo (if available) or text "TrakRF"
  - Tagline: "Real-Time Asset Tracking"
  - Optional: RFID wave icon or abstract tech graphic
- **Format**: PNG with transparency support
- **File size**: < 1MB (optimize for fast sharing)

**Design Approach**:
Use a simple, professional design with high contrast text for readability in social media previews. The design should be recognizable at small sizes (Facebook preview is ~500x261px).

**Placeholder Option**:
If design tools aren't immediately available, create a simple solid color background (brand blue #3b82f6) with white text:
- "TrakRF" (large, bold)
- "Real-Time Asset Tracking" (medium, regular)
- Centered, high contrast

**Validation**:
- File exists at `frontend/public/og-image.png`
- File size < 1MB
- Dimensions exactly 1200x630px
- Test in frontend build: `cd frontend && pnpm build`
- Verify file copied to `frontend/dist/og-image.png`

---

### Task 6: Create Production Build Script
**File**: `scripts/build.sh`
**Action**: CREATE
**Pattern**: Clean-first approach for reproducible builds

**Implementation**:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Production build script for TrakRF platform
# Builds frontend → embeds in backend → produces single binary artifact
#
# Usage:
#   ./scripts/build.sh              # Build everything
#   ./scripts/build.sh --skip-clean # Skip clean step (faster, but less reproducible)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Parse arguments
SKIP_CLEAN=false
if [[ "${1:-}" == "--skip-clean" ]]; then
    SKIP_CLEAN=true
fi

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Helper functions
info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

# Change to project root
cd "$PROJECT_ROOT"

# ============================================================================
# Clean previous builds (POLS: reproducible builds)
# ============================================================================
if [ "$SKIP_CLEAN" = false ]; then
    info "Cleaning previous builds..."
    rm -rf frontend/dist
    rm -rf backend/bin
    success "Clean complete"
else
    info "Skipping clean step (--skip-clean flag)"
fi

# ============================================================================
# Build frontend
# ============================================================================
info "Building frontend..."
cd frontend

# Verify pnpm is available
if ! command -v pnpm &> /dev/null; then
    error "pnpm not found. Install with: npm install -g pnpm"
fi

# Build frontend (outputs to frontend/dist/)
pnpm build

# Verify build output exists
if [ ! -d "dist" ] || [ ! -f "dist/index.html" ]; then
    error "Frontend build failed - dist/index.html not found"
fi

success "Frontend build complete ($(du -sh dist | cut -f1))"

# ============================================================================
# Build backend with embedded frontend
# ============================================================================
cd "$PROJECT_ROOT/backend"
info "Building backend with embedded frontend..."

# Verify frontend/dist exists (required for go:embed)
if [ ! -d "../frontend/dist" ]; then
    error "Frontend dist not found - run frontend build first"
fi

# Create bin directory if it doesn't exist
mkdir -p bin

# Build backend binary with version info
VERSION="${VERSION:-dev}"
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

go build -o bin/trakrf \
    -ldflags "-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT}" \
    .

# Verify binary was created
if [ ! -f "bin/trakrf" ]; then
    error "Backend build failed - bin/trakrf not found"
fi

# Make binary executable
chmod +x bin/trakrf

success "Backend build complete ($(du -sh bin/trakrf | cut -f1))"

# ============================================================================
# Build summary
# ============================================================================
echo ""
info "Build complete! 🚀"
echo ""
echo "Artifacts:"
echo "  Backend binary:    backend/bin/trakrf"
echo "  Frontend assets:   Embedded in binary"
echo "  Total binary size: $(du -sh backend/bin/trakrf | cut -f1)"
echo ""
echo "Run locally:"
echo "  cd backend && ./bin/trakrf"
echo ""
echo "Deploy to Railway/GKE:"
echo "  docker build -t trakrf:latest ."
echo "  docker push <registry>/trakrf:latest"
echo ""
```

Make script executable:
```bash
chmod +x scripts/build.sh
```

**Validation**:
- Run build script: `./scripts/build.sh`
- Verify frontend/dist/ created
- Verify backend/bin/trakrf created
- Test binary runs: `cd backend && ./bin/trakrf`
- Verify frontend assets served correctly

---

### Task 7: Build Frontend for Testing
**File**: N/A
**Action**: BUILD
**Pattern**: Ensure frontend/dist/ exists before testing backend

**Implementation**:

```bash
cd frontend
pnpm build
```

This creates the `frontend/dist/` directory that the backend `go:embed` directive requires. Without this, the backend will not compile.

**Validation**:
- `frontend/dist/index.html` exists
- `frontend/dist/assets/` directory exists with hashed JS/CSS files
- All static files copied to dist/ (favicon, icons, og-image.png)

---

### Task 8: Test Integrated Application
**File**: N/A
**Action**: TEST
**Pattern**: Manual testing of all route types and cache headers

**Test Matrix**:

```bash
# 1. Build and run integrated server
cd backend
go run .

# 2. Test API routes (should return JSON)
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/api/v1/health

# 3. Test static assets (should have long cache headers)
curl -I http://localhost:8080/favicon.ico
# Expected: Cache-Control: public, max-age=3600

curl -I http://localhost:8080/assets/index-[hash].js
# Expected: Cache-Control: public, max-age=31536000, immutable

# 4. Test index.html (should have no-cache headers)
curl -I http://localhost:8080/
# Expected: Cache-Control: no-cache, no-store, must-revalidate

# 5. Test SPA catch-all (should return index.html)
curl -i http://localhost:8080/dashboard
curl -i http://localhost:8080/some-random-route
# Expected: HTML content, no-cache headers

# 6. Test in browser
# Open http://localhost:8080
# - Should see React app
# - Check Network tab: assets have correct cache headers
# - Navigate to /dashboard directly (should work via React Router)
# - Verify inventory screen accessible without login
# - Verify asset/location features redirect to login

# 7. Test CORS behavior
# With BACKEND_CORS_ORIGIN unset
curl -i http://localhost:8080/api/v1/health -H "Origin: http://localhost:5173"
# Expected: Access-Control-Allow-Origin: *

# With BACKEND_CORS_ORIGIN set
BACKEND_CORS_ORIGIN=https://trakrf.com go run .
curl -i http://localhost:8080/api/v1/health -H "Origin: http://localhost:5173"
# Expected: Access-Control-Allow-Origin: https://trakrf.com

# 8. Test meta tags
curl -s http://localhost:8080/ | grep -i "og:title"
# Expected: <meta property="og:title" content="TrakRF - Real-Time Asset Tracking" />

# 9. Test build script
./scripts/build.sh
cd backend && ./bin/trakrf
# Expected: Same behavior as `go run .`
```

**Validation Criteria**:
- ✅ API routes return JSON with correct headers
- ✅ Static assets return correct cache headers
- ✅ index.html has no-cache headers
- ✅ SPA catch-all serves index.html for all non-API/asset routes
- ✅ React app loads and renders correctly
- ✅ CORS respects env var configuration
- ✅ Meta tags present in HTML
- ✅ Built binary runs standalone

---

### Task 9: Create Unit Tests for Cache Headers
**File**: `backend/frontend_test.go`
**Action**: CREATE
**Pattern**: Follow test pattern from `backend/health_test.go`

**Implementation**:

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCacheControlMiddleware(t *testing.T) {
	// Create a test handler that just returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with cache control middleware
	handler := cacheControlMiddleware(testHandler)

	tests := []struct {
		name           string
		path           string
		expectedCache  string
		expectedPragma string
	}{
		{
			name:           "index.html has no-cache",
			path:           "/",
			expectedCache:  "no-cache, no-store, must-revalidate",
			expectedPragma: "no-cache",
		},
		{
			name:           "explicit index.html has no-cache",
			path:           "/index.html",
			expectedCache:  "no-cache, no-store, must-revalidate",
			expectedPragma: "no-cache",
		},
		{
			name:           "SPA route has no-cache",
			path:           "/dashboard",
			expectedCache:  "no-cache, no-store, must-revalidate",
			expectedPragma: "no-cache",
		},
		{
			name:          "hashed JS asset has long cache",
			path:          "/assets/index-abc123.js",
			expectedCache: "public, max-age=31536000, immutable",
		},
		{
			name:          "hashed CSS asset has long cache",
			path:          "/assets/index-xyz789.css",
			expectedCache: "public, max-age=31536000, immutable",
		},
		{
			name:          "favicon has moderate cache",
			path:          "/favicon.ico",
			expectedCache: "public, max-age=3600",
		},
		{
			name:          "icon has moderate cache",
			path:          "/icon-192.png",
			expectedCache: "public, max-age=3600",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			cacheControl := rec.Header().Get("Cache-Control")
			if cacheControl != tt.expectedCache {
				t.Errorf("Expected Cache-Control: %s, got: %s", tt.expectedCache, cacheControl)
			}

			// Only check Pragma for no-cache scenarios
			if tt.expectedPragma != "" {
				pragma := rec.Header().Get("Pragma")
				if pragma != tt.expectedPragma {
					t.Errorf("Expected Pragma: %s, got: %s", tt.expectedPragma, pragma)
				}
			}
		})
	}
}

func TestSPAHandler(t *testing.T) {
	// Note: This test requires frontend/dist/index.html to exist
	// Run `pnpm build` in frontend/ before running tests

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	spaHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got: %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("Expected Content-Type: text/html; charset=utf-8, got: %s", contentType)
	}

	cacheControl := rec.Header().Get("Cache-Control")
	if cacheControl != "no-cache, no-store, must-revalidate" {
		t.Errorf("Expected no-cache headers, got: %s", cacheControl)
	}
}
```

**Validation**:
- Run tests: `cd backend && go test -v -run TestCacheControl`
- Run tests: `cd backend && go test -v -run TestSPAHandler`
- All tests should pass

---

### Task 10: Update Development Documentation
**File**: `backend/README.md` (if exists) or create it
**Action**: CREATE or MODIFY
**Pattern**: Document the two development modes

**Implementation**:

Add or create a section on running the application:

```markdown
# TrakRF Backend

## Development Modes

### Mode 1: Separate Frontend + Backend (Hot Reload)
Use this mode for active frontend development:

```bash
# Terminal 1: Frontend dev server
cd frontend
pnpm dev
# Frontend runs on http://localhost:5173 with hot reload

# Terminal 2: Backend API server
cd backend
go run .
# Backend runs on http://localhost:8080
# CORS enabled by default (BACKEND_CORS_ORIGIN=*)
```

### Mode 2: Integrated (Production Preview)
Use this mode to test the production deployment locally:

```bash
# Build frontend first
cd frontend
pnpm build

# Run backend with embedded frontend
cd backend
go run .
# Full app runs on http://localhost:8080
# Frontend assets served from embedded filesystem
```

### Mode 3: Production Binary
Use this mode to build the final deployment artifact:

```bash
# Build everything
./scripts/build.sh

# Run the binary
cd backend
./bin/trakrf
# Full app runs on http://localhost:8080
```

## Environment Variables

- `BACKEND_PORT` - Server port (default: 8080)
- `BACKEND_CORS_ORIGIN` - CORS allowed origin (default: * for dev, set to domain in production)
- `JWT_SECRET` - JWT signing secret (required for auth)
- `DATABASE_URL` - PostgreSQL connection string (required)

## Testing

```bash
# Run backend tests
just backend-test

# Run full validation
just validate
```
```

**Validation**:
- Documentation is clear and accurate
- Both dev modes are explained
- Environment variables documented

---

## Risk Assessment

### Risk: Router Order Mistakes
**Description**: Incorrect route order could cause API routes to be caught by SPA handler, or static assets to return JSON errors.

**Mitigation**:
- Follow strict ordering: health checks → static assets → API → SPA catch-all
- Test each route type independently
- Commit after router changes and verify all routes work

**Likelihood**: Medium | **Impact**: High | **Priority**: 🔴 Critical

### Risk: Cache Header Configuration Errors
**Description**: Wrong cache headers could break cache busting, causing users to see stale JavaScript after deployments.

**Mitigation**:
- Unit tests for cache header logic
- Manual verification with curl -I for each asset type
- Test cache busting: build twice, verify different hashes, confirm browser gets new assets

**Likelihood**: Low | **Impact**: High | **Priority**: 🟡 Medium

### Risk: CORS Breaking Development Workflow
**Description**: Changing CORS behavior could break the separate frontend/backend development setup.

**Mitigation**:
- Default to `*` when env var not set (preserves current behavior)
- Document both dev modes clearly
- Test both integrated and separate modes

**Likelihood**: Low | **Impact**: Medium | **Priority**: 🟢 Low

### Risk: go:embed Path Mistakes
**Description**: Wrong embed path or missing frontend/dist could cause build failures.

**Mitigation**:
- Build frontend FIRST in Task 7 (before testing backend)
- go:embed will fail at compile time if path wrong (fail fast)
- Build script enforces correct order

**Likelihood**: Low | **Impact**: Low | **Priority**: 🟢 Low

### Risk: Meta Tags Not Rendering in Previews
**Description**: Open Graph tags might not work if image URL is wrong or tags are malformed.

**Mitigation**:
- Test with curl to verify tags present in HTML
- Use Open Graph debugger: https://developers.facebook.com/tools/debug/
- Verify og-image.png is accessible at /og-image.png

**Likelihood**: Low | **Impact**: Low | **Priority**: 🟢 Low

## Integration Points

### Router Integration
- **What**: Add frontend route handlers to existing chi router
- **Where**: `backend/main.go` after middleware setup, before API routes
- **Impact**: No changes to existing API routes, additive only

### Middleware Stack
- **What**: CORS middleware updated to use env var
- **Where**: `backend/middleware.go:58-72`
- **Impact**: Behavior change only when `BACKEND_CORS_ORIGIN` set, defaults to current behavior

### Build System
- **What**: New build script integrates frontend build with backend build
- **Where**: `scripts/build.sh`
- **Impact**: New workflow, doesn't affect existing `go build` or `pnpm build` commands

### Frontend Assets
- **What**: Meta tags updated, OG image added
- **Where**: `frontend/index.html`, `frontend/public/og-image.png`
- **Impact**: No functional changes, only metadata

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are blocking gates that must pass before proceeding.

After EVERY code change, run the following commands:

### Gate 1: Backend Lint & Format
```bash
just backend-lint
```
**Expected**: No errors, all files formatted

### Gate 2: Backend Tests
```bash
just backend-test
```
**Expected**: All tests passing, including new frontend_test.go

### Gate 3: Frontend Build
```bash
just frontend-build
```
**Expected**: Build succeeds, dist/ directory created

### Gate 4: Full Stack Validation
```bash
just validate
```
**Expected**: All checks pass (lint, typecheck, test, build)

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts on same gate → Stop and ask for help
- Do not proceed to next task until current task passes all relevant gates

## Validation Sequence

### After Each Task
Run relevant gates based on files modified:
- Backend file modified → Gates 1, 2
- Frontend file modified → Gate 3
- Both modified → Gates 1, 2, 3

### After All Tasks Complete
Run full validation sequence:

```bash
# 1. Full stack validation
just validate

# 2. Build production binary
./scripts/build.sh

# 3. Test integrated application (Task 8 test matrix)
cd backend && ./bin/trakrf
# Run through full test matrix from Task 8

# 4. Manual browser testing
# - Open http://localhost:8080
# - Test inventory screen (public)
# - Test asset features (require login)
# - Test direct navigation to routes
# - Check Network tab for cache headers
# - Verify meta tags in HTML source

# 5. Test both development modes
# Mode 1: Separate (hot reload)
cd frontend && pnpm dev  # Terminal 1
cd backend && go run .   # Terminal 2

# Mode 2: Integrated (production preview)
cd frontend && pnpm build
cd backend && go run .
```

## Plan Quality Assessment

**Complexity Score**: 5/10 (MEDIUM-LOW)

**Complexity Factors**:
- 📁 **File Impact**: Creating 3 files, modifying 3 files (6 files total) - Well-scoped
- 🔗 **Subsystems**: Touching 2 subsystems (Backend routing, Frontend meta tags)
- 🔢 **Task Estimate**: ~10 atomic subtasks - Manageable size
- 📦 **Dependencies**: 0 new packages (Go stdlib only)
- 🆕 **Pattern Novelty**: Existing patterns (go:embed well-documented, middleware pattern already in codebase)

**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Similar patterns found in codebase (`middleware.go`, `main.go` router setup)
✅ All clarifying questions answered
✅ go:embed is well-documented Go stdlib feature
✅ Chi router v5 patterns well-established
✅ Frontend build process already working
✅ Existing test patterns to follow

⚠️ Minor uncertainty: OG image design (requires visual design, but can use simple placeholder)
⚠️ First integration of frontend with backend (new pattern for this project, but standard practice)

**Assessment**: High confidence implementation. The patterns are well-established, the codebase already has the middleware infrastructure, and all integration points are clear. The main complexity is ensuring correct router ordering and cache headers, both of which have clear testing strategies.

**Estimated one-pass success probability**: 85%

**Reasoning**: This is primarily integration work following established Go and React patterns. The codebase already has the middleware pattern, chi router setup, and build tooling. The main risks (router order, cache headers) are mitigated by comprehensive testing and validation gates. The 15% uncertainty accounts for potential edge cases in route matching and cache header behavior that may require iteration.
