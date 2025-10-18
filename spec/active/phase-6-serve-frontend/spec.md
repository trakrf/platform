# Feature: Phase 6 - Serve Frontend Assets

## Origin
Linear Issue: [TRA-80 - Phase 6: Serve Frontend Assets](https://linear.app/trakrf/issue/TRA-80/phase-6-serve-frontend-assets)

This is the critical integration phase that unifies the React frontend and Go backend into a single production-ready application, preparing for Railway deployment in Phase 7.

## Outcome
The Go backend will serve the React frontend as a Single Page Application (SPA), handling all routing for both API endpoints and client-side routes, with proper authentication flow and meta tag support for link sharing.

## User Story
**As a** platform operator
**I want** the Go backend to serve the React frontend directly
**So that** we have a single deployment artifact ready for production with unified authentication and proper link previews

## Context

**Current State**:
- React frontend runs on separate dev server (typically :5173 Vite)
- Go backend runs on :8080
- Frontend and backend are developed independently
- Production deployment requires separate hosting for each
- URL sharing shows shipfa.st branding (template preview)

**Desired State**:
- Single Go server serves both API and frontend
- Go embeds built React assets into binary
- API routes available at `/api/*`
- Frontend SPA routing works seamlessly
- Authentication flow unified (no CORS issues)
- Custom meta tags for link sharing
- Production-ready single artifact for deployment

**Dependencies**:
- ✅ Phase 5 (Authentication) complete
- React frontend builds successfully via `pnpm build`
- Go backend has middleware stack ready

## Technical Requirements

### 1. Static Asset Serving Strategy

**Decision Required**: Choose between two approaches:

#### Option A: `go:embed` (RECOMMENDED for Production)
```go
//go:embed frontend/dist
var frontendFS embed.FS

// Serve embedded files
http.FileServer(http.FS(subFS))
```

**Pros**:
- Single binary artifact (easy deployment)
- No external dependencies at runtime
- Fast serving from memory
- Ideal for Railway deployment

**Cons**:
- Requires rebuild for frontend changes
- Larger binary size

#### Option B: Runtime Static Serving
```go
http.FileServer(http.Dir("./frontend/dist"))
```

**Pros**:
- No rebuild needed for frontend changes
- Smaller binary

**Cons**:
- Must deploy binary + static folder
- More complex deployment process

**Recommendation**: Use `go:embed` for production deployment. The rebuild requirement is acceptable since we'll have a proper CI/CD pipeline.

### 2. Routing Architecture

The server must handle three distinct route types:

#### API Routes (`/api/*`)
- Handled by existing API handlers
- Return JSON responses
- Protected by JWT middleware (Phase 5)
- 404s return JSON error responses

#### Static Assets (`/assets/*`, `/favicon.ico`, etc.)
- Served from embedded filesystem
- Proper cache headers (long TTL for hashed assets)
- No authentication required
- 404s return HTTP 404

#### SPA Catch-All Routes (everything else)
- Return `index.html` for client-side routing
- React Router handles actual routing
- Authenticated routes protected by auth middleware
- Enables direct navigation to `/dashboard`, `/assets`, etc.

### 3. Middleware Stack Order

**Critical**: Middleware order determines behavior

```
Request
  ↓
1. CORS (if needed)
  ↓
2. Logging
  ↓
3. Static asset check → Serve if match
  ↓
4. API route check → Route to API handlers
  ↓
5. Authentication middleware → Protect routes
  ↓
6. SPA catch-all → Return index.html
```

**Key Points**:
- Static assets served BEFORE auth middleware
- API routes handled BEFORE catch-all
- Auth middleware only on protected routes

### 4. React Build Integration

**Build Process**:
```bash
cd frontend
pnpm build  # Output to frontend/dist/
```

**Go Integration**:
```go
//go:embed frontend/dist
var frontendFS embed.FS

func serveFrontend() http.Handler {
    // Strip "frontend/dist" prefix to serve from root
    subFS, _ := fs.Sub(frontendFS, "frontend/dist")
    return http.FileServer(http.FS(subFS))
}
```

**Build Output Structure**:
```
frontend/dist/
├── index.html
├── assets/
│   ├── index-[hash].js
│   ├── index-[hash].css
│   └── ...
└── favicon.ico
```

### 5. Pre-Auth vs Post-Auth Routing

**Pre-Auth Routes** (No middleware protection):
- `/` (landing page)
- `/login`
- `/register`
- `/forgot-password`
- Static assets (`/assets/*`)

**Post-Auth Routes** (Require JWT):
- `/dashboard`
- `/assets` (asset tracking page)
- `/settings`
- `/api/*` (all API endpoints)

**Implementation**:
```go
// Public routes - no auth
publicMux.Handle("/", spaHandler)
publicMux.Handle("/login", spaHandler)
publicMux.Handle("/register", spaHandler)

// Protected routes - require auth
protectedMux.Use(jwtAuthMiddleware)
protectedMux.Handle("/dashboard", spaHandler)
protectedMux.Handle("/api/", apiHandler)
```

### 6. Meta Tags for Link Sharing

**Problem**: URL sharing currently shows shipfa.st preview

**Solution**: Serve custom `index.html` with proper Open Graph tags

```html
<!-- frontend/index.html -->
<head>
  <title>TrakRF - Real-Time Asset Tracking</title>

  <!-- Open Graph / Facebook -->
  <meta property="og:type" content="website">
  <meta property="og:url" content="https://trakrf.com/">
  <meta property="og:title" content="TrakRF - Real-Time Asset Tracking">
  <meta property="og:description" content="Track your assets in real-time with TrakRF">
  <meta property="og:image" content="https://trakrf.com/og-image.png">

  <!-- Twitter -->
  <meta property="twitter:card" content="summary_large_image">
  <meta property="twitter:url" content="https://trakrf.com/">
  <meta property="twitter:title" content="TrakRF - Real-Time Asset Tracking">
  <meta property="twitter:description" content="Track your assets in real-time with TrakRF">
  <meta property="twitter:image" content="https://trakrf.com/og-image.png">
</head>
```

**Requirements**:
- Create `public/og-image.png` (1200x630px recommended)
- Update all meta tags to reflect TrakRF branding
- Remove any shipfa.st references

### 7. Development Workflow

**Development Mode** (keep existing setup):
- Frontend: `pnpm dev` on :5173 with hot reload
- Backend: `go run .` on :8080
- CORS enabled for cross-origin requests

**Production Mode** (integrated):
- Build: `pnpm build` in frontend/
- Run: `go run .` serves everything on :8080
- Test production build before deployment

**Build Script** (recommended):
```bash
#!/bin/bash
# scripts/build.sh

echo "Building frontend..."
cd frontend && pnpm build && cd ..

echo "Building backend..."
go build -o bin/trakrf .

echo "Build complete: bin/trakrf"
```

## Implementation Plan

### Phase 6A: Static File Serving
- [ ] Implement `go:embed` for frontend/dist
- [ ] Create file server handler
- [ ] Add static asset route handlers
- [ ] Test serving index.html

### Phase 6B: Routing Strategy
- [ ] Implement API route prefix matching
- [ ] Add SPA catch-all handler (returns index.html)
- [ ] Configure middleware stack order
- [ ] Test direct navigation to SPA routes

### Phase 6C: Build Integration
- [ ] Verify `pnpm build` output
- [ ] Ensure embed path matches build output
- [ ] Create build script
- [ ] Test embedded assets load correctly

### Phase 6D: Pre-Auth vs Post-Auth
- [ ] Define public route list
- [ ] Define protected route list
- [ ] Configure auth middleware to skip public routes
- [ ] Test login flow without auth middleware

### Phase 6E: Meta Tags & Branding
- [ ] Create OG image (1200x630px)
- [ ] Update index.html meta tags
- [ ] Remove shipfa.st references
- [ ] Test URL sharing preview

## Code Examples

### Main Router Setup
```go
package main

import (
    "embed"
    "io/fs"
    "net/http"
)

//go:embed frontend/dist
var frontendFS embed.FS

func main() {
    mux := http.NewServeMux()

    // Serve embedded frontend
    frontendHandler := serveFrontend()

    // API routes (protected)
    mux.Handle("/api/", jwtAuthMiddleware(apiRouter))

    // Static assets (public)
    mux.Handle("/assets/", frontendHandler)
    mux.Handle("/favicon.ico", frontendHandler)

    // SPA catch-all (mixed auth)
    mux.HandleFunc("/", spaHandler)

    http.ListenAndServe(":8080", mux)
}

func serveFrontend() http.Handler {
    subFS, err := fs.Sub(frontendFS, "frontend/dist")
    if err != nil {
        panic(err)
    }
    return http.FileServer(http.FS(subFS))
}

func spaHandler(w http.ResponseWriter, r *http.Request) {
    // Serve index.html for all non-asset routes
    // This enables client-side routing

    // Check if route requires auth
    if requiresAuth(r.URL.Path) {
        // Apply JWT middleware
        jwtAuthMiddleware(http.HandlerFunc(serveIndexHTML)).ServeHTTP(w, r)
    } else {
        serveIndexHTML(w, r)
    }
}

func serveIndexHTML(w http.ResponseWriter, r *http.Request) {
    indexHTML, _ := frontendFS.ReadFile("frontend/dist/index.html")
    w.Header().Set("Content-Type", "text/html")
    w.Write(indexHTML)
}

func requiresAuth(path string) bool {
    publicRoutes := []string{"/", "/login", "/register", "/forgot-password"}
    for _, route := range publicRoutes {
        if path == route {
            return false
        }
    }
    return true
}
```

### Cache Headers for Static Assets

**Cache Busting Strategy**: Vite automatically generates content-hashed filenames (e.g., `index-a1b2c3d4.js`). When you rebuild:
1. `pnpm build` creates NEW hashed filenames
2. `go build` embeds the NEW files into binary
3. Browser sees different URLs = automatic cache invalidation

**Critical**: `index.html` must NEVER be cached, as it contains the references to hashed assets.

```go
func serveStaticWithCache(h http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        path := r.URL.Path

        // index.html: NO cache (always fresh)
        // This ensures browsers always get the latest asset references
        if path == "/" || path == "/index.html" || !strings.Contains(path, ".") {
            w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
            w.Header().Set("Pragma", "no-cache")
            w.Header().Set("Expires", "0")
        } else if strings.HasPrefix(path, "/assets/") {
            // Hashed assets: LONG cache (1 year immutable)
            // Safe because filename changes when content changes
            w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
        } else {
            // Other static files (favicon, etc.): moderate cache
            w.Header().Set("Cache-Control", "public, max-age=3600")
        }

        h.ServeHTTP(w, r)
    })
}
```

**Cache Busting Flow**:
```
Deploy v1:
  index.html → references → index-a1b2c3d4.js (cached 1 year)

Code change + deploy v2:
  index.html → references → index-9z8y7x6w.js (NEW file)

User visits after v2 deploy:
  1. Fetches index.html (no-cache = always fresh)
  2. Sees reference to index-9z8y7x6w.js
  3. Browser downloads NEW JS (different URL = not in cache)
  4. Old index-a1b2c3d4.js remains cached but unused
```

## Validation Criteria

### Functional Tests
- [ ] Navigate to `http://localhost:8080` shows React app
- [ ] Direct navigation to `http://localhost:8080/dashboard` works (returns index.html)
- [ ] API calls to `http://localhost:8080/api/assets` return JSON
- [ ] 404 for non-existent API route returns JSON error
- [ ] 404 for non-existent frontend route shows React 404 page
- [ ] Login flow works without CORS errors
- [ ] favicon.ico loads correctly

### Cache Headers & Busting
- [ ] `index.html` returns `Cache-Control: no-cache, no-store, must-revalidate`
- [ ] Hashed assets (`/assets/index-*.js`) return `Cache-Control: public, max-age=31536000, immutable`
- [ ] Favicon returns `Cache-Control: public, max-age=3600`
- [ ] After frontend rebuild, new hashed filenames generated
- [ ] Browser receives new assets after deployment (not cached version)

### Authentication Flow
- [ ] Unauthenticated user can access `/login`
- [ ] Unauthenticated user redirected from `/dashboard`
- [ ] Authenticated user can access `/dashboard`
- [ ] JWT token stored and sent correctly
- [ ] Token refresh works on same origin

### Link Sharing
- [ ] Sharing `http://localhost:8080` shows TrakRF preview (not shipfa.st)
- [ ] OG image displays correctly in link preview
- [ ] Title and description are custom TrakRF branding

### Build & Deployment
- [ ] `pnpm build` completes successfully
- [ ] `go build` embeds frontend assets
- [ ] Resulting binary runs standalone (no external files needed)
- [ ] Binary size is reasonable (<50MB)

### Development Workflow
- [ ] Can still run `pnpm dev` for hot reload during development
- [ ] Can test production build locally before deploying
- [ ] Build script works end-to-end

## Performance Considerations

### Asset Loading
- Gzip/Brotli compression for text assets
- Long cache TTL for hashed assets (1 year)
- No cache for index.html (to get latest routing)

### Binary Size
- Monitor embedded asset size
- Consider excluding source maps from production build
- Frontend build should use production optimizations

## Security Considerations

### Path Traversal
- Use `fs.Sub()` to prevent directory traversal attacks
- Never directly serve user-provided paths

### CORS
- Same-origin serving eliminates CORS issues
- Remove CORS middleware for production (not needed)
- Keep CORS for development if frontend runs on different port

### Authentication
- Ensure auth middleware doesn't block static assets
- Validate JWT on all `/api/*` routes
- Client-side routing protection (redirect to login)

## Open Questions

1. **Build Strategy**: Confirm go:embed vs runtime serving (recommend go:embed)
2. **OG Image**: What should the TrakRF preview image show?
3. **Public Routes**: Are there other routes that should be publicly accessible?
4. **Error Pages**: Should we have custom 404/500 pages in React?
5. **Environment Config**: How do we handle API URLs in frontend (build-time env vars)?

## References

### Linear Issue
- [TRA-80 - Phase 6: Serve Frontend Assets](https://linear.app/trakrf/issue/TRA-80/phase-6-serve-frontend-assets)

### Related Documentation
- CLAUDE.md: Package manager rules (pnpm), git workflow, architecture principles
- PLANNING.md: Overall project architecture and goals
- Phase 5: Authentication (dependency)
- Phase 7: Railway Deployment (next phase)

### Technical Resources
- [Go embed package](https://pkg.go.dev/embed)
- [SPA serving patterns in Go](https://github.com/gorilla/mux#serving-single-page-applications)
- [Open Graph protocol](https://ogp.me/)

## Success Metrics

**This phase is successful when**:
1. Single `go run .` command serves entire application
2. SPA routing works for direct navigation
3. Authentication flow is seamless (no CORS issues)
4. Link sharing shows TrakRF branding
5. Production build creates single binary artifact
6. Ready for Railway deployment (Phase 7)

## Next Steps

After spec approval:
1. Run `/plan spec/active/phase-6-serve-frontend/spec.md`
2. Implement Phase 6A-6E in sequence
3. Test thoroughly against validation criteria
4. Create PR for review
5. Merge and proceed to Phase 7 (Railway Deployment)
