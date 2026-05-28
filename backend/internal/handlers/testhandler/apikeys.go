package testhandler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/trakrf/platform/backend/internal/models/apikey"
	"github.com/trakrf/platform/backend/internal/util/apisecret"
	"github.com/trakrf/platform/backend/internal/util/httputil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// bbTestOrgIdentifier / bbTestUserEmail are the natural-key handles for the
// Schemathesis contract-test fixture seeded by
// backend/database/seeds/contract_test_seed.sql.
const (
	bbTestOrgIdentifier = "bb-test-org"
	bbTestUserEmail     = "bb-test@trakrf.invalid"
	mintedKeyName       = apikey.SchemathesisMintKeyName
)

// MintAPIKeyRequest is the body for POST /test/apikeys.
type MintAPIKeyRequest struct {
	Scopes []string `json:"scopes"`
}

// MintAPIKeyResponse is returned on success. The token is the freshly-signed JWT
// and the only place the caller can read it; we do not persist or echo it later.
type MintAPIKeyResponse struct {
	Token string `json:"token"`
	JTI   string `json:"jti"`
	Name  string `json:"name"`
}

// MintAPIKey mints an API-key JWT scoped to the seeded bb-test-org. Used by the
// Schemathesis contract-test loop as its bearer token. This route is only
// mounted when APP_ENV != "production" (see cmd/serve/router.go).
//
// POST /test/apikeys
func (h *Handler) MintAPIKey(w http.ResponseWriter, r *http.Request) {
	var req MintAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if len(req.Scopes) == 0 {
		http.Error(w, "scopes must not be empty", http.StatusBadRequest)
		return
	}
	for _, s := range req.Scopes {
		if !apikey.ValidScopes[s] {
			http.Error(w, "unknown scope: "+s, http.StatusBadRequest)
			return
		}
	}

	ctx := r.Context()

	org, err := h.storage.GetOrganizationByIdentifier(ctx, bbTestOrgIdentifier)
	if err != nil {
		http.Error(w, "Failed to look up bb-test-org", http.StatusInternalServerError)
		return
	}
	if org == nil {
		http.Error(w, "bb-test-org not seeded — run backend/database/seeds/contract_test_seed.sql", http.StatusFailedDependency)
		return
	}

	user, err := h.storage.GetUserByEmail(ctx, bbTestUserEmail)
	if err != nil {
		http.Error(w, "Failed to look up bb-test user", http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.Error(w, "bb-test user not seeded — run backend/database/seeds/contract_test_seed.sql", http.StatusFailedDependency)
		return
	}

	secret, err := apisecret.Generate()
	if err != nil {
		http.Error(w, "Failed to generate secret", http.StatusInternalServerError)
		return
	}
	creator := apikey.Creator{UserID: &user.ID}
	key, err := h.storage.CreateAPIKey(ctx, org.ID, mintedKeyName, apisecret.Hash(secret), req.Scopes, creator, nil)
	if err != nil {
		http.Error(w, "Failed to create api key", http.StatusInternalServerError)
		return
	}

	// Mint a short-lived access token directly (generous TTL covers a full
	// contract-test run) instead of round-tripping the grant endpoint.
	exp := time.Now().Add(1 * time.Hour)
	token, err := jwt.GenerateAccessToken(key.JTI, org.ID, req.Scopes, &exp)
	if err != nil {
		http.Error(w, "Failed to sign access token", http.StatusInternalServerError)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, MintAPIKeyResponse{
		Token: token,
		JTI:   key.JTI,
		Name:  key.Name,
	})
}
