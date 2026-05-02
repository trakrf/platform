# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial project documentation structure
- Business Source License 1.1 with Additional Use Grant
- Code of Conduct (Contributor Covenant 2.1)
- Security policy with vulnerability reporting procedures
- Contributing guidelines with code examples and testing requirements
- CLAUDE.md for AI assistant guidance

### Changed
- Migrating handheld React app as frontend component
- TRA-578 Public API surface cleanup:
  - `POST/GET/DELETE /api/v1/orgs/{id}/api-keys*` removed from the public OpenAPI spec. Key minting remains browser-mediated by design (see Authentication docs). The endpoints are still implemented and used by the SPA's avatar menu.
  - Renamed scope `scans:read` → `history:read` to align with the `/assets/{id}/history` and `/locations/current` endpoint vocabulary. Existing keys are migrated by `000039_rename_scans_read_scope`. JWTs minted before the migration with a literal `scans:read` claim will return 403 — pre-launch hard cut, no production keys exist.
  - SPA "Scans" row in the new-key form is renamed to "History" to match the new scope name.

## [0.1.0] - 2025-10-11

### Added
- Initial project structure and licensing
- Core documentation for open source project
- .gitignore with Go backend and Node.js frontend support
