package middleware

// Canonical 401 detail strings emitted by every auth middleware path
// (Auth, APIKeyAuth, EitherAuth, RequireScope). Routing all auth-failure
// emissions through these constants keeps the public-API surface
// consistent — TRA-724 harmonized the missing-header case across
// `/orgs/me` (APIKeyAuth) and the EitherAuth-fronted endpoints
// (`/assets`, `/locations`, `/reports/...`) that had drifted apart.
//
// The strings themselves are not part of the contract — `error.type` and
// `error.title` are. Docs (`errors.md`) explicitly tell integrators not to
// branch on `detail`. Constants exist so that drift is a code-review-visible
// change and not silent literal duplication.
const (
	Detail401MissingAuthHeader     = "Missing authorization header"
	Detail401InvalidAuthFormat     = "Invalid authorization header format"
	Detail401InvalidOrExpiredToken = "Invalid or expired token"
	Detail401APIKeyRevoked         = "API key has been revoked"
	Detail401APIKeyExpired         = "API key has expired"
	// Detail401UseAuthBearerHint replaces the generic missing-header detail
	// when the request carries X-API-Key without an Authorization header —
	// integrators try X-API-Key first because the credential is called an
	// "API key," and the generic 401 leaves them chasing key-rotation red
	// herrings (TRA-449 D10).
	Detail401UseAuthBearerHint = "Use Authorization: Bearer <token>"
)
