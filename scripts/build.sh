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
    rm -rf backend/frontend/dist
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
# Copy frontend dist to backend for embedding
# ============================================================================
cd "$PROJECT_ROOT"
info "Copying frontend assets to backend..."
mkdir -p backend/frontend
cp -r frontend/dist backend/frontend/
success "Frontend assets copied"

# ============================================================================
# Build backend with embedded frontend
# ============================================================================
cd "$PROJECT_ROOT/backend"
info "Building backend with embedded frontend..."

# Verify frontend/dist exists (required for go:embed)
if [ ! -d "frontend/dist" ]; then
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
