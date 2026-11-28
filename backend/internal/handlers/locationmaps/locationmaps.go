// Package locationmaps serves location maps: a floor plan or site image on a
// parent location, its descendant locations pinned on that image, and a live
// picture of who is in each pinned zone.
//
// Internal surface (session auth, not in the public OpenAPI). It is base
// platform, not a sold capability: a map is another view of the locations and
// current-location data every org already has. Reads are open to any member;
// writes are paid mutations and need an org admin.
package locationmaps

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/locationmap"
	"github.com/trakrf/platform/backend/internal/models/report"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

const (
	// maxImageBodyBytes bounds the JSON upload body: base64 of a
	// MaxImageBytes image is 4/3 its size, plus a little envelope.
	maxImageBodyBytes = locationmap.MaxImageBytes/3*4 + 64*1024

	// maxPresenceAssets bounds the current-locations read behind presence.
	// A floor plan with more live assets than this is not readable anyway.
	maxPresenceAssets = 5000

	defaultSeenWithin = time.Hour
)

type Handler struct {
	store *storage.Storage
	now   func() time.Time
}

func NewHandler(store *storage.Storage) *Handler {
	return &Handler{store: store, now: time.Now}
}

// RegisterRoutes mounts the location-map routes. Caller applies session auth.
// paidGate and adminGate go on the writes only.
func (h *Handler) RegisterRoutes(r chi.Router, paidGate, adminGate func(http.Handler) http.Handler) {
	r.Get("/api/v1/location-maps", h.List)
	r.Get("/api/v1/location-maps/{location_id}", h.Get)
	r.Get("/api/v1/location-maps/{location_id}/image", h.GetImage)
	r.Get("/api/v1/location-maps/{location_id}/presence", h.GetPresence)
	r.With(paidGate, adminGate).Put("/api/v1/location-maps/{location_id}/image", h.PutImage)
	r.With(paidGate, adminGate).Put("/api/v1/location-maps/{location_id}/pins", h.PutPins)
	r.With(paidGate, adminGate).Delete("/api/v1/location-maps/{location_id}", h.Delete)
}

// request resolves the org and the {location_id} path param, writing the
// error response itself when either is missing or malformed.
func (h *Handler) request(w http.ResponseWriter, r *http.Request) (orgID, locationID int, reqID string, ok bool) {
	reqID = middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return 0, 0, reqID, false
	}
	if raw := chi.URLParam(r, "location_id"); raw != "" {
		locationID, err = httputil.ParseSurrogateID("location_id", raw)
		if err != nil {
			httputil.RespondPathParamError(w, r, err, reqID)
			return 0, 0, reqID, false
		}
	}
	return orgID, locationID, reqID, true
}

func internalError(w http.ResponseWriter, r *http.Request, err error, reqID string) {
	httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
}

// List returns the org's maps.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	orgID, _, reqID, ok := h.request(w, r)
	if !ok {
		return
	}
	maps, err := h.store.ListLocationMaps(r.Context(), orgID)
	if err != nil {
		internalError(w, r, err, reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": maps})
}

// Get returns a location's map metadata and pins; 404 when it has none.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	orgID, locationID, reqID, ok := h.request(w, r)
	if !ok {
		return
	}
	m, err := h.store.GetLocationMap(r.Context(), orgID, locationID)
	if err != nil {
		internalError(w, r, err, reqID)
		return
	}
	if m == nil {
		httputil.Respond404(w, r, "this location has no map", reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": m})
}

// GetImage serves the image bytes. The ETag is the image's SHA-256, so a
// client polling the map revalidates for free.
func (h *Handler) GetImage(w http.ResponseWriter, r *http.Request) {
	orgID, locationID, reqID, ok := h.request(w, r)
	if !ok {
		return
	}
	ct, img, sha, err := h.store.GetLocationMapImage(r.Context(), orgID, locationID)
	if err != nil {
		internalError(w, r, err, reqID)
		return
	}
	if img == nil {
		httputil.Respond404(w, r, "this location has no map", reqID)
		return
	}
	etag := `"` + sha + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Length", strconv.Itoa(len(img)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(img)
}

type putImageRequest struct {
	DataBase64 string `json:"data_base64"`
}

// PutImage creates or replaces a location's map image. The body is JSON
// carrying the image as base64; the type is sniffed from the bytes.
func (h *Handler) PutImage(w http.ResponseWriter, r *http.Request) {
	orgID, locationID, reqID, ok := h.request(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImageBodyBytes)
	var req putImageRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			httputil.WriteJSONError(w, r, http.StatusRequestEntityTooLarge, modelerrors.ErrValidation, errImageTooLarge.Error(), reqID)
			return
		}
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	img, err := base64.StdEncoding.DecodeString(req.DataBase64)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, "data_base64 is not valid base64", reqID)
		return
	}
	ct, err := sniffMapImage(img)
	if errors.Is(err, errImageTooLarge) {
		httputil.WriteJSONError(w, r, http.StatusRequestEntityTooLarge, modelerrors.ErrValidation, err.Error(), reqID)
		return
	}
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, err.Error(), reqID)
		return
	}

	loc, err := h.store.GetLocationByID(r.Context(), orgID, locationID)
	if err != nil {
		internalError(w, r, err, reqID)
		return
	}
	if loc == nil {
		httputil.Respond404(w, r, "location not found", reqID)
		return
	}
	if err := h.store.UpsertLocationMapImage(r.Context(), orgID, locationID, ct, img); err != nil {
		internalError(w, r, err, reqID)
		return
	}
	h.Get(w, r)
}

type putPinsRequest struct {
	Pins []locationmap.Pin `json:"pins"`
}

// PutPins full-replaces a map's pins. Every pinned location must be a live
// descendant of the map location.
func (h *Handler) PutPins(w http.ResponseWriter, r *http.Request) {
	orgID, locationID, reqID, ok := h.request(w, r)
	if !ok {
		return
	}
	var req putPinsRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validatePins(locationID, req.Pins); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, err.Error(), reqID)
		return
	}
	m, err := h.store.GetLocationMap(r.Context(), orgID, locationID)
	if err != nil {
		internalError(w, r, err, reqID)
		return
	}
	if m == nil {
		httputil.Respond404(w, r, "this location has no map; upload an image first", reqID)
		return
	}
	if len(req.Pins) > 0 {
		desc, err := h.store.GetDescendants(r.Context(), orgID, locationID)
		if err != nil {
			internalError(w, r, err, reqID)
			return
		}
		live := make(map[int]bool, len(desc))
		for _, d := range desc {
			if d.DeletedAt == nil {
				live[d.ID] = true
			}
		}
		for _, p := range req.Pins {
			if !live[p.LocationID] {
				httputil.WriteJSONError(w, r, http.StatusUnprocessableEntity, modelerrors.ErrValidation,
					"a pinned location must be a live location below the map's location", reqID)
				return
			}
		}
	}
	if err := h.store.ReplaceLocationMapPins(r.Context(), orgID, locationID, req.Pins); err != nil {
		internalError(w, r, err, reqID)
		return
	}
	h.Get(w, r)
}

// Delete removes a location's map and its pins. The location itself stays.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, locationID, reqID, ok := h.request(w, r)
	if !ok {
		return
	}
	existed, err := h.store.DeleteLocationMap(r.Context(), orgID, locationID)
	if err != nil {
		internalError(w, r, err, reqID)
		return
	}
	if !existed {
		httputil.Respond404(w, r, "this location has no map", reqID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetPresence returns who is in each pinned zone of a map now.
//
// The source is the same current-location read the Current table uses
// (ListCurrentLocations: each asset's latest location, which BLE zone
// arbitration decides upstream), narrowed to the map's subtree. Each asset is
// drawn at its location's pin, or at the nearest pinned ancestor when its
// location has none. Assets in the subtree with no pinned ancestor come back
// in unplaced. Position inside a zone is never claimed: the zone is all the
// data knows.
//
// seen_within (seconds, default 3600) drops assets not seen that recently;
// 0 means no limit.
func (h *Handler) GetPresence(w http.ResponseWriter, r *http.Request) {
	orgID, locationID, reqID, ok := h.request(w, r)
	if !ok {
		return
	}
	seenWithin := defaultSeenWithin
	if raw := r.URL.Query().Get("seen_within"); raw != "" {
		secs, err := strconv.Atoi(raw)
		if err != nil || secs < 0 {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
				"seen_within must be a non-negative number of seconds", reqID)
			return
		}
		seenWithin = time.Duration(secs) * time.Second
	}

	ctx := r.Context()
	m, err := h.store.GetLocationMap(ctx, orgID, locationID)
	if err != nil {
		internalError(w, r, err, reqID)
		return
	}
	if m == nil {
		httputil.Respond404(w, r, "this location has no map", reqID)
		return
	}
	desc, err := h.store.GetDescendants(ctx, orgID, locationID)
	if err != nil {
		internalError(w, r, err, reqID)
		return
	}
	parents := map[int]*int{locationID: nil}
	names := map[int]string{locationID: m.LocationName}
	ids := []int{locationID}
	for _, d := range desc {
		parents[d.ID] = d.ParentID
		names[d.ID] = d.Name
		ids = append(ids, d.ID)
	}

	current, err := h.store.ListCurrentLocations(ctx, orgID, report.CurrentLocationFilter{
		LocationIDs: ids,
		Limit:       maxPresenceAssets,
	})
	if err != nil {
		internalError(w, r, err, reqID)
		return
	}
	personIDs, err := h.store.ListPersonAssetIDs(ctx, orgID)
	if err != nil {
		internalError(w, r, err, reqID)
		return
	}

	now := h.now()
	out := buildPresence(m, parents, names, current, personIDs, seenWithin, now)
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// buildPresence assembles the presence picture from already-read rows. Pure,
// so the roll-up and filtering are unit-testable without a database.
func buildPresence(
	m *locationmap.Map,
	parents map[int]*int,
	names map[int]string,
	current []report.CurrentLocationItem,
	personIDs []int,
	seenWithin time.Duration,
	now time.Time,
) locationmap.Presence {
	people := make(map[int]bool, len(personIDs))
	for _, id := range personIDs {
		people[id] = true
	}
	pinned := make(map[int]bool, len(m.Pins))
	zones := make([]locationmap.ZoneOccupancy, 0, len(m.Pins))
	zoneIdx := make(map[int]int, len(m.Pins))
	for _, p := range m.Pins {
		pinned[p.LocationID] = true
		zoneIdx[p.LocationID] = len(zones)
		zones = append(zones, locationmap.ZoneOccupancy{
			LocationID: p.LocationID, Name: names[p.LocationID],
			XPct: p.XPct, YPct: p.YPct, Occupants: []locationmap.Occupant{},
		})
	}

	out := locationmap.Presence{
		MapLocationID: m.LocationID,
		GeneratedAt:   now,
		Zones:         zones,
		Unplaced:      []locationmap.Occupant{},
	}
	for _, c := range current {
		if c.LocationID == nil {
			continue
		}
		if seenWithin > 0 && now.Sub(c.LastSeen) > seenWithin {
			continue
		}
		occ := locationmap.Occupant{
			AssetID: c.AssetID, Name: c.AssetName, ExternalKey: c.AssetExternalKey,
			IsPerson: people[c.AssetID], LocationID: *c.LocationID,
			LocationName: names[*c.LocationID], LastSeen: c.LastSeen,
		}
		if zoneID, ok := rollUp(m.LocationID, parents, pinned, *c.LocationID); ok {
			z := &out.Zones[zoneIdx[zoneID]]
			z.Occupants = append(z.Occupants, occ)
			continue
		}
		if _, inSubtree := parents[*c.LocationID]; inSubtree {
			out.Unplaced = append(out.Unplaced, occ)
		}
	}
	return out
}
