// Single source for the platform version surfaced in the UI.
//
// Sourced from VITE_APP_VERSION, injected at Docker build time from the
// root VERSION file (see TRA-1126 + the root Dockerfile build-meta stage).
// Same string flows into the Go binary via `-X main.version`, so `/health`
// and the nav header report identical values for a given build. It used to
// be `git describe` output, which made the version a property of the ref
// that built the image rather than of the commit.
//
// `frontend/package.json` `version` is intentionally NOT the source:
// it was the historical display string and drifted into a separate
// `1.0.18` counter while the backend reported `0.1.0-preview`. The
// monolith has one platform version (TRA-485); package.json is now
// only consumed by tooling that needs a SemVer for the workspace
// (none in this private repo).
export const appVersion = import.meta.env.VITE_APP_VERSION || 'dev';
