Architecture & Technology Decisions
Core Stack:

Backend: Go (rejected Python, TypeScript, Rust)
Database: TimescaleDB with community features (continuous aggregates, compression)
Frontend: React (moving away from Next.js)
Deployment: Railway initially, then ECS/K8s at scale
MQTT: Direct ingestion in Go backend (removing Redpanda Connect)

Architecture Pattern:

Monolith with embedded React build (single origin, no CORS)
Two-container deployment: TimescaleDB + Go/React app
Docker-compose for self-hosting and development

Licensing: Business Source License

Protects against potential hosting competitors
Allows customer self-hosting
Can relax later, but starting restrictive

Development Priorities
MVP Features:

JWT auth (skip OAuth initially)
Basic CRUD for assets, tags, locations
MQTT → events table pipeline
Core query: "Where is tag X?"

Defer Until Later:
Continuous aggregates (add when needed)
RLS (handle multi-tenancy in Go)
API keys (build when customers ask)
Redpanda Connect integration

Technical Decisions
Keep From Existing Work:

Permuted ID system (32-bit vs UUIDs matters)
Core schema design with temporal validity
Account-based multi-tenancy (not container-per-customer)
React handheld UI

Tooling Choices:

golang-migrate for migrations
swaggo/swag for API documentation
JWT + bcrypt for auth (no Supabase)
Single binary deployment with embedded assets

Project Structure:

Monorepo containing backend/, frontend/, database/, marketing/
Single CI/CD pipeline
Atomic commits across stack changes

Key Insights

Time-to-market over architecture: Ship on Railway, optimize later
Use DevOps expertise to know when (not) to optimize
TimescaleDB features (continuous aggregates) are perfect for RFID use case
Start with boring choices, add complexity only when customers demand it

The overarching theme: You have solid technical foundations already built. Stop second-guessing, ship the MVP with Go + TimescaleDB + React, and iterate based on actual customer feedback.