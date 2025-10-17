# Epic: TrakRF Platform Migration

## Vision
Migrate from Next.js (trakrf-web) and standalone React (trakrf-handheld) to unified Go + React monorepo platform. Replace production deployments with consolidated stack.

## Current State (What We're Replacing)

**Production (To Be Replaced):**
- **trakrf.id** - Next.js marketing/content site (Railway)
- **handheld.trakrf.id** - Standalone React RFID reader app (Railway)
- **Status**: Two separate deployments, no users, no uptime SLA
- **Replacement goal**: Unified platform, single Railway instance, cost savings

**Source Repositories:**
- `trakrf-web` - Next.js marketing + content site
- `trakrf-handheld` - Standalone React RFID reader app
- Both → Merged into `platform/` monorepo
- **Current state**: Frontend from trakrf-handheld already baselined (Phase 1)

## Target Architecture

### URL Structure (DECIDED)

**Subdomain split with Cloudflare:**
```
trakrf.id           → Astro marketing (Cloudflare Pages)
app.trakrf.id       → Go + React platform (Railway)
handheld.trakrf.id  → [RETIRE] replaced by app.trakrf.id
```

**Why this architecture:**
- Marketing on Cloudflare edge (fast globally)
- Platform near database (Railway, same as current)
- User familiar with Cloudflare Pages (mikestankavich.com)
- Proven Astro + CF Pages pattern available
- Cost effective (CF free tier + Railway single instance)

**Reference:**
- Astro + CF Pages example: https://github.com/mikestankavich/mikestankavich.com

### Stack
- **Backend:** Go 1.21+ (HTTP server, API, database)
- **Frontend:** React + TypeScript (from trakrf-handheld baseline)
- **Content:** Astro static site (port from trakrf-web)
- **Database:** TimescaleDB (already migrated)
- **Hosting:** Railway (platform) + Cloudflare Pages (marketing)

### Auth Strategy

**Two-tier access model:**

**Pre-auth (Public Demo):**
- RFID reading features (handheld core)
- "Try it, see, we can read tags"
- No account required
- **Goal**: Low friction demo, instant value

**Post-auth (Registered Users):**
- Asset/location CRUD screens
- Account management
- Data persistence
- Requires registration

## Migration Phases (Roadmap)

### ✅ Phase 0: Monorepo Setup (COMPLETE)
- Workspace structure
- Frontend baseline (trakrf-handheld)
- Validation commands

### ✅ Phase 1: Docker Dev Environment (COMPLETE)
- TimescaleDB + schema
- 12 SQL init scripts
- Just docker commands

### 🔄 Phase 2: Go Backend Baseline (ACTIVE)
- HTTP server hello world
- Health check endpoint
- Just backend commands
- Version: 0.1.0-dev

### 📋 Phase 3: Database Migrations
- go-migrate setup
- Port 12 SQL scripts to migrations
- Schema version tracking
- `just db-migrate up/down`

### 📋 Phase 4: Basic REST API
- API framework (chi/gorilla/echo?)
- Simple endpoints (health, version, accounts)
- JSON responses
- Error handling

### 📋 Phase 5: Authentication
- Port next-auth to Go (JWT)
- Use existing schema (users, accounts)
- Session management
- Protected routes

### 📋 Phase 6: Serve Frontend Assets
- Static file serving from Go
- React build integration
- Index.html routing (SPA)
- Pre-auth vs post-auth routing

### 📋 Phase 7: Deploy to Railway
- Dockerfile (multi-stage build)
- railway.json config
- Environment variables
- DNS cutover preparation

### 📋 Phase 8: Marketing Site Migration (Cloudflare Pages)
- Port trakrf-web content to Astro
- Static site generation
- Deploy to Cloudflare Pages (following mikestankavich.com pattern)
- Configure trakrf.id DNS → CF Pages
- SEO optimization
- Reference: github.com/mikestankavich/mikestankavich.com

## Definition of Done

**When can we retire old deployments and flip DNS?**

**Technical requirements:**
- [ ] Platform deployed to Railway (app.trakrf.id)
- [ ] Marketing deployed to Cloudflare Pages (trakrf.id)
- [ ] All frontend features working (handheld baseline)
- [ ] Auth working (login, session, protected routes)
- [ ] Database migrations applied
- [ ] Marketing content migrated (Astro)
- [ ] DNS configured (Cloudflare or current provider)
- [ ] SSL certificates active (CF auto, Railway auto)
- [ ] handheld.trakrf.id redirected or retired
- [ ] Old trakrf.id deployment stopped (cost savings)

**Business requirements:**
- [ ] No users on old stack (safe to deprecate)
- [ ] Smoke tests passing
- [ ] Monitoring/health checks working
- [ ] Rollback plan documented

**Migration milestone: 0.1.0**
- Tag version after Phase 7 deploy
- Represents "production ready platform"
- Marketing migration can be 0.2.0 if needed

## Migration Strategy

### What We're Keeping
- ✅ Handheld frontend (React/TypeScript)
- ✅ Database schema (12 tables)
- ✅ EMQX Cloud MQTT broker
- ✅ Railway deployment (same host, new stack)

### What We're Consolidating
- 🔄 Two Railway deployments → One Railway deployment
- 🔄 handheld.trakrf.id + trakrf.id → app.trakrf.id + trakrf.id
- 🔄 Split hosting (marketing on Cloudflare, platform on Railway)

### What We're Leaving Behind
- ❌ Next.js (both app and content)
- ❌ Separate repositories (unified monorepo)
- ❌ next-auth (port to Go)
- ❌ Dual Railway instances (cost savings)

### What We're Porting
- 🔄 Marketing content (Next.js → Astro)
- 🔄 Auth logic (next-auth → Go JWT)
- 🔄 Database schema (SQL init → go-migrate)
- 🔄 Handheld app (standalone → monorepo frontend)

## Open Decisions

**Cloudflare DNS:**
- [ ] Using Cloudflare DNS for trakrf.id?
- [ ] Or just CF Pages for hosting?
- Affects: DNS migration complexity

**Railway deployment:**
- [ ] Do both current deployments share Railway project?
- [ ] Or separate Railway projects?
- Affects: Migration cutover strategy

**API framework:**
- [ ] chi vs gorilla vs echo vs stdlib
- Decide by: Phase 4 (REST API)

**Frontend build:**
- [ ] Go embed vs runtime static serving
- Decide by: Phase 6 (serve frontend)

## Success Metrics

**0.1.0 Launch:**
- Platform running on Railway (app.trakrf.id)
- Users can demo RFID reading (pre-auth)
- Users can register and manage assets (post-auth)
- Marketing content accessible (trakrf.id)
- Old deployments retired (handheld.trakrf.id, old trakrf.id)
- Cost savings: 2 Railway instances → 1

**Performance:**
- Health check < 10ms
- API responses < 100ms
- Frontend load < 2s
- Database queries < 50ms

## Cost Savings

**Current:**
- Railway: 2 instances × $5-10/month = $10-20/month
- Total: ~$10-20/month

**After Migration:**
- Railway: 1 instance × $5-10/month = $5-10/month
- Cloudflare Pages: Free tier
- Total: ~$5-10/month (50% savings)

## Notes & Context

**Why this epic:**
- No users = freedom to iterate
- No uptime SLA = can break things
- Clean slate = right patterns from start
- Unified repo = easier maintenance
- Cost optimization = half the Railway costs

**What's different from old stack:**
- Real frontend (handheld, not Next.js SSR)
- Go backend (not Next.js API routes)
- Proper migrations (not ad-hoc SQL)
- Monorepo (not scattered repos)
- Split hosting (marketing edge, platform near DB)

**Timeline:**
- No deadline (iterative development)
- Each phase = 1-3 days work
- 0.1.0 target: ~2-3 weeks from Phase 2

## References

- Astro + Cloudflare Pages: https://github.com/mikestankavich/mikestankavich.com
- SHIPPED.md: Track completed phases
- spec/active/: Current phase work
