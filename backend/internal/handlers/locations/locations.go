package locations

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

var validate = func() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)
	return v
}()

type Handler struct {
	storage *storage.Storage
}

func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{
		storage: storage,
	}
}

// resolveParent reconciles the parent_id (canonical) and parent_external_key
// (natural-key alternate) inputs on Create/Update. Both nil → nil (no parent).
// parent_external_key alone → resolved via lookup. Both set → must agree;
// mismatch returns a validation FieldError.
func (handler *Handler) resolveParent(
	r *http.Request, orgID int, parentID *int, parentExternalKey *string,
) (*int, *modelerrors.FieldError) {
	hasID := parentID != nil
	hasExt := parentExternalKey != nil && *parentExternalKey != ""

	if !hasID && !hasExt {
		return nil, nil
	}
	if !hasExt {
		return parentID, nil
	}

	parent, err := handler.storage.GetLocationByExternalKey(r.Context(), orgID, *parentExternalKey)
	if err != nil {
		return nil, &modelerrors.FieldError{
			Field:   "parent_external_key",
			Code:    "internal_error",
			Message: err.Error(),
		}
	}
	if parent == nil {
		return nil, &modelerrors.FieldError{
			Field:   "parent_external_key",
			Code:    "invalid_value",
			Message: fmt.Sprintf("parent_external_key %q not found", *parentExternalKey),
		}
	}
	if hasID && *parentID != parent.ID {
		return nil, &modelerrors.FieldError{
			Field:   "parent_external_key",
			Code:    "invalid_value",
			Message: "parent_id and parent_external_key disagree",
		}
	}
	return &parent.ID, nil
}

// @Summary      Create a location
// @Description  Create a new location in the hierarchy, optionally with one or more tags.
// @Description  Set parent_id (canonical) or parent_external_key (alternate) to nest under an existing parent.
// @Tags         locations,public
// @ID           locations.create
// @Accept       json
// @Produce      json
// @Param        request  body  location.CreateLocationWithTagsRequest  true  "Location to create with optional tags"
// @Success      201  {object}  locations.CreateLocationResponse
// @Header       201  {string}  Location  "Canonical URL of the created resource"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      409  {object}  modelerrors.ErrorResponse     "conflict"
// @Failure      415  {object}  modelerrors.ErrorResponse     "unsupported_media_type"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:write]
// @Router       /api/v1/locations [post]
func (handler *Handler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, requestID)
		return
	}

	var request location.CreateLocationWithTagsRequest
	if err := httputil.DecodeJSONStrict(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, requestID)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, requestID)
		return
	}

	resolved, fErr := handler.resolveParent(r, orgID, request.ParentID, request.ParentExternalKey)
	if fErr != nil {
		if fErr.Code == "internal_error" {
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
				fErr.Message, requestID)

			return
		}
		httputil.WriteJSONErrorWithFields(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			fErr.Message, requestID,
			[]modelerrors.FieldError{*fErr})

		return
	}
	request.ParentID = resolved

	if request.IsActive == nil {
		t := true
		request.IsActive = &t
	}
	if request.ValidFrom == nil || request.ValidFrom.IsZero() {
		fd := shared.FlexibleDate{Time: time.Now().UTC()}
		request.ValidFrom = &fd
	}

	result, err := handler.storage.CreateLocationWithTags(r.Context(), orgID, request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				err.Error(), requestID)

			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	w.Header().Set("Location", "/api/v1/locations/"+strconv.Itoa(result.ID))
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": location.ToPublicLocationView(*result)})
}

// @Summary      Update a location
// @Description  Update mutable fields on an existing location. Only fields included in the request body are changed.
// @Tags         locations,public
// @ID           locations.update
// @Accept       json
// @Produce      json
// @Param        location_id path  int                              true  "Location ID"
// @Param        request  body  location.UpdateLocationRequest   true  "Fields to update"
// @Success      200  {object}  locations.UpdateLocationResponse
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      409  {object}  modelerrors.ErrorResponse     "conflict"
// @Failure      415  {object}  modelerrors.ErrorResponse     "unsupported_media_type"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:write]
// @Router       /api/v1/locations/{location_id} [put]
func (handler *Handler) Update(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	id, ok := handler.parseAndVerifyLocationID(w, req, orgID, reqID)
	if !ok {
		return
	}

	handler.doUpdate(w, req, orgID, id)
}

func (handler *Handler) doUpdate(w http.ResponseWriter, req *http.Request, orgID, id int) {
	reqID := middleware.GetRequestID(req.Context())

	var request location.UpdateLocationRequest
	explicitNulls, err := httputil.DecodeJSONStrictWithNulls(req, &request)
	if err != nil {
		httputil.RespondDecodeError(w, req, err, reqID)
		return
	}

	if _, ok := explicitNulls["valid_from"]; ok {
		httputil.WriteJSONErrorWithFields(w, req, http.StatusBadRequest, modelerrors.ErrValidation,
			"validation failed", reqID,
			[]modelerrors.FieldError{{
				Field:   "valid_from",
				Code:    "invalid_value",
				Message: "valid_from cannot be null; omit the field to leave unchanged, or provide a date",
			}})

		return
	}
	if _, ok := explicitNulls["valid_to"]; ok {
		request.ClearValidTo = true
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, req, err, reqID)
		return
	}

	resolved, fErr := handler.resolveParent(req, orgID, request.ParentID, request.ParentExternalKey)
	if fErr != nil {
		if fErr.Code == "internal_error" {
			httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
				fErr.Message, reqID)

			return
		}
		httputil.WriteJSONErrorWithFields(w, req, http.StatusBadRequest, modelerrors.ErrValidation,
			fErr.Message, reqID,
			[]modelerrors.FieldError{*fErr})

		return
	}
	request.ParentID = resolved

	result, err := handler.storage.UpdateLocation(req.Context(), orgID, id, request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, req, http.StatusConflict, modelerrors.ErrConflict,
				err.Error(), reqID)

			return
		}
		httputil.RespondStorageError(w, req, err, reqID)
		return
	}

	if result == nil {
		httputil.Respond404(w, req, apierrors.LocationNotFound, reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": location.ToPublicLocationView(*result)})
}

// @Summary Delete location
// @Description Delete a location by its ID. The location is removed from all subsequent queries. Returns 204 on success, 404 if the location does not exist or has already been deleted.
// @Tags locations,public
// @ID locations.delete
// @Accept json
// @Produce json
// @Param location_id path int true "Location ID"
// @Success 204 "deleted"
// @Failure 400 {object} modelerrors.ErrorResponse "bad_request"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "forbidden"
// @Failure 404 {object} modelerrors.ErrorResponse "not_found"
// @Failure 429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure 500 {object} modelerrors.ErrorResponse "internal_error"
// @Security APIKey[locations:write]
// @Router /api/v1/locations/{location_id} [delete]
func (handler *Handler) Delete(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	id, ok := handler.parseAndVerifyLocationID(w, req, orgID, reqID)
	if !ok {
		return
	}

	handler.doDelete(w, req, orgID, id)
}

func (handler *Handler) doDelete(w http.ResponseWriter, req *http.Request, orgID, id int) {
	reqID := middleware.GetRequestID(req.Context())

	deleted, err := handler.storage.DeleteLocation(req.Context(), orgID, id)
	if err != nil {
		httputil.RespondStorageError(w, req, err, reqID)
		return
	}

	if !deleted {
		httputil.Respond404(w, req, apierrors.LocationNotFound, reqID)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type ListLocationsResponse struct {
	Data       []location.PublicLocationView `json:"data"`
	Limit      int                           `json:"limit"       example:"50"`
	Offset     int                           `json:"offset"      example:"0"`
	TotalCount int                           `json:"total_count" example:"100"`
}

type GetLocationResponse struct {
	Data location.PublicLocationView `json:"data"`
}

type CreateLocationResponse struct {
	Data location.PublicLocationView `json:"data"`
}

type UpdateLocationResponse struct {
	Data location.PublicLocationView `json:"data"`
}

type ListAncestorsResponse struct {
	Data       []location.PublicLocationView `json:"data"`
	Limit      int                           `json:"limit"       example:"50"`
	Offset     int                           `json:"offset"      example:"0"`
	TotalCount int                           `json:"total_count" example:"100"`
}

type ListChildrenResponse struct {
	Data       []location.PublicLocationView `json:"data"`
	Limit      int                           `json:"limit"       example:"50"`
	Offset     int                           `json:"offset"      example:"0"`
	TotalCount int                           `json:"total_count" example:"100"`
}

type ListDescendantsResponse struct {
	Data       []location.PublicLocationView `json:"data"`
	Limit      int                           `json:"limit"       example:"50"`
	Offset     int                           `json:"offset"      example:"0"`
	TotalCount int                           `json:"total_count" example:"100"`
}

// @Summary List locations
// @Tags locations,public
// @ID locations.list
// @Param limit               query int    false "max 200"  default(50)
// @Param offset              query int    false "min 0"   default(0)
// @Param parent_id            query int    false "filter by parent id (canonical, may repeat); mutually exclusive with parent_external_key"
// @Param parent_external_key query string false "filter by parent's external_key (may repeat); mutually exclusive with parent_id"
// @Param is_active           query bool   false "filter by active flag"
// @Param q                   query string false "substring search (case-insensitive) on name, external_key, description, and active tag values"
// @Param sort                query []string false "comma-separated, prefix '-' for DESC" collectionFormat(csv) Enums(tree_path, -tree_path, external_key, -external_key, name, -name, created_at, -created_at)
// @Success 200 {object} locations.ListLocationsResponse
// @Failure 400 {object} modelerrors.ErrorResponse "bad_request"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "forbidden"
// @Failure 429 {object} modelerrors.ErrorResponse "rate_limited"
// @Security APIKey[locations:read]
// @Router /api/v1/locations [get]
func (handler *Handler) ListLocations(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	params, err := httputil.ParseListParams(req, httputil.ListAllowlist{
		Filters:     []string{"parent_id", "parent_external_key", "is_active", "q"},
		BoolFilters: []string{"is_active"},
		Sorts:       []string{"tree_path", "external_key", "name", "created_at"},
	})
	if err != nil {
		httputil.RespondListParamError(w, req, err, reqID)
		return
	}

	// D-10: parent_id and parent_external_key are mutually exclusive — pick
	// one form per request.
	_, hasParentID := params.Filters["parent_id"]
	_, hasParentExtKey := params.Filters["parent_external_key"]
	if hasParentID && hasParentExtKey {
		httputil.WriteJSONErrorWithFields(w, req, http.StatusBadRequest, modelerrors.ErrValidation,
			"parent_id and parent_external_key are mutually exclusive", reqID,
			[]modelerrors.FieldError{{
				Field:   "parent_external_key",
				Code:    "invalid_value",
				Message: "parent_id and parent_external_key are mutually exclusive",
			}})
		return
	}

	f := location.ListFilter{
		ParentExternalKeys: params.Filters["parent_external_key"],
		Limit:              params.Limit,
		Offset:             params.Offset,
	}
	if vs, ok := params.Filters["parent_id"]; ok && len(vs) > 0 {
		f.ParentIDs = make([]int, 0, len(vs))
		for _, s := range vs {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 {
				httputil.WriteJSONErrorWithFields(w, req, http.StatusBadRequest, modelerrors.ErrValidation,
					"invalid parent_id", reqID,
					[]modelerrors.FieldError{{
						Field:   "parent_id",
						Code:    "invalid_value",
						Message: fmt.Sprintf("parent_id %q must be a positive integer", s),
					}})
				return
			}
			f.ParentIDs = append(f.ParentIDs, n)
		}
	}
	if vs, ok := params.Filters["is_active"]; ok && len(vs) > 0 {
		b := vs[0] == "true"
		f.IsActive = &b
	}
	if vs, ok := params.Filters["q"]; ok && len(vs) > 0 {
		f.Q = &vs[0]
	}
	for _, s := range params.Sorts {
		f.Sorts = append(f.Sorts, location.ListSort{Field: s.Field, Desc: s.Desc})
	}

	items, err := handler.storage.ListLocationsFiltered(req.Context(), orgID, f)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	total, err := handler.storage.CountLocationsFiltered(req.Context(), orgID, f)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	out := make([]location.PublicLocationView, 0, len(items))
	for _, l := range items {
		out = append(out, location.ToPublicLocationView(l))
	}

	httputil.WriteJSON(w, http.StatusOK, ListLocationsResponse{
		Data:       out,
		Limit:      params.Limit,
		Offset:     params.Offset,
		TotalCount: total,
	})
}

// @Summary Get location by ID
// @Description Retrieve a location by its canonical ID. Returns 404 if not found.
// @Tags locations,public
// @ID locations.get
// @Param location_id path int true "Location ID"
// @Success 200 {object} locations.GetLocationResponse
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 429 {object} modelerrors.ErrorResponse
// @Security APIKey[locations:read]
// @Router /api/v1/locations/{location_id} [get]
func (handler *Handler) GetLocation(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	idParam := chi.URLParam(req, "location_id")
	id, err := httputil.ParseSurrogateID(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			err.Error(), reqID)

		return
	}

	view, err := handler.storage.GetLocationViewByID(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}
	if view == nil {
		httputil.Respond404(w, req, apierrors.LocationNotFound, reqID)
		return
	}

	withParent := location.LocationWithParent{LocationView: *view}
	if view.ParentID != nil {
		parent, err := handler.storage.GetLocationByID(req.Context(), orgID, *view.ParentID)
		if err == nil && parent != nil {
			ek := parent.ExternalKey
			withParent.ParentExternalKey = &ek
		}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": location.ToPublicLocationView(withParent)})
}

// @Summary Lookup location by external_key
// @Description Find a single live location by its external_key. Returns 404 if no match.
// @Description Exactly one natural-key parameter is required; multiple or none returns 400.
// @Tags locations,public
// @ID locations.lookup
// @Param external_key query string true "Location external_key (natural key)"
// @Success 200 {object} locations.GetLocationResponse
// @Failure 400 {object} modelerrors.ErrorResponse "bad_request"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "forbidden"
// @Failure 404 {object} modelerrors.ErrorResponse "not_found"
// @Failure 429 {object} modelerrors.ErrorResponse "rate_limited"
// @Security APIKey[locations:read]
// @Router /api/v1/locations/lookup [get]
func (handler *Handler) Lookup(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	q := req.URL.Query()
	// D-4: duplicate external_key params silently first-wins is an LLM-hostile
	// bug; reject them explicitly. Repeated values (same or different) → 400.
	if len(q["external_key"]) > 1 {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"exactly one of: external_key", reqID)
		return
	}

	naturalKeyParams := []string{"external_key"}
	provided := 0
	for _, k := range naturalKeyParams {
		if q.Has(k) && q.Get(k) != "" {
			provided++
		}
	}

	if provided == 0 {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"exactly one of: external_key", reqID)

		return
	}
	if provided > 1 {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"exactly one of: external_key", reqID)

		return
	}

	externalKey := q.Get("external_key")
	loc, err := handler.storage.GetLocationByExternalKey(req.Context(), orgID, externalKey)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}
	if loc == nil {
		httputil.Respond404(w, req, apierrors.LocationNotFound, reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data": location.ToPublicLocationView(*loc),
	})
}

// @Summary List location ancestors
// @Tags locations,public
// @ID locations.ancestors
// @Param location_id path  int    true  "Location ID"
// @Param limit  query int    false "max 200"  default(50)
// @Param offset query int    false "min 0"   default(0)
// @Success 200 {object} locations.ListAncestorsResponse
// @Failure 400 {object} modelerrors.ErrorResponse "bad_request"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "forbidden"
// @Failure 404 {object} modelerrors.ErrorResponse "not_found"
// @Failure 429 {object} modelerrors.ErrorResponse "rate_limited"
// @Failure 500 {object} modelerrors.ErrorResponse "internal_error"
// @Security APIKey[locations:read]
// @Router /api/v1/locations/{location_id}/ancestors [get]
func (handler *Handler) GetAncestors(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	id, ok := handler.parseAndVerifyLocationID(w, req, orgID, reqID)
	if !ok {
		return
	}

	params, err := httputil.ParseListParams(req, httputil.ListAllowlist{})
	if err != nil {
		httputil.RespondListParamError(w, req, err, reqID)
		return
	}

	results, err := handler.storage.ListAncestorsPaginated(req.Context(), orgID, id, params.Limit, params.Offset)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	total, err := handler.storage.CountAncestors(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	httputil.WriteJSON(w, http.StatusOK, ListAncestorsResponse{
		Data:       toPublicLocationViews(results),
		Limit:      params.Limit,
		Offset:     params.Offset,
		TotalCount: total,
	})
}

// @Summary List location descendants
// @Tags locations,public
// @ID locations.descendants
// @Param location_id path  int    true  "Location ID"
// @Param limit  query int    false "max 200"  default(50)
// @Param offset query int    false "min 0"   default(0)
// @Success 200 {object} locations.ListDescendantsResponse
// @Failure 400 {object} modelerrors.ErrorResponse "bad_request"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "forbidden"
// @Failure 404 {object} modelerrors.ErrorResponse "not_found"
// @Failure 429 {object} modelerrors.ErrorResponse "rate_limited"
// @Failure 500 {object} modelerrors.ErrorResponse "internal_error"
// @Security APIKey[locations:read]
// @Router /api/v1/locations/{location_id}/descendants [get]
func (handler *Handler) GetDescendants(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	id, ok := handler.parseAndVerifyLocationID(w, req, orgID, reqID)
	if !ok {
		return
	}

	params, err := httputil.ParseListParams(req, httputil.ListAllowlist{})
	if err != nil {
		httputil.RespondListParamError(w, req, err, reqID)
		return
	}

	results, err := handler.storage.ListDescendantsPaginated(req.Context(), orgID, id, params.Limit, params.Offset)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	total, err := handler.storage.CountDescendants(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	httputil.WriteJSON(w, http.StatusOK, ListDescendantsResponse{
		Data:       toPublicLocationViews(results),
		Limit:      params.Limit,
		Offset:     params.Offset,
		TotalCount: total,
	})
}

// @Summary List location children
// @Tags locations,public
// @ID locations.children
// @Param location_id path  int    true  "Location ID"
// @Param limit  query int    false "max 200"  default(50)
// @Param offset query int    false "min 0"   default(0)
// @Success 200 {object} locations.ListChildrenResponse
// @Failure 400 {object} modelerrors.ErrorResponse "bad_request"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "forbidden"
// @Failure 404 {object} modelerrors.ErrorResponse "not_found"
// @Failure 429 {object} modelerrors.ErrorResponse "rate_limited"
// @Failure 500 {object} modelerrors.ErrorResponse "internal_error"
// @Security APIKey[locations:read]
// @Router /api/v1/locations/{location_id}/children [get]
func (handler *Handler) GetChildren(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	id, ok := handler.parseAndVerifyLocationID(w, req, orgID, reqID)
	if !ok {
		return
	}

	params, err := httputil.ParseListParams(req, httputil.ListAllowlist{})
	if err != nil {
		httputil.RespondListParamError(w, req, err, reqID)
		return
	}

	results, err := handler.storage.ListChildrenPaginated(req.Context(), orgID, id, params.Limit, params.Offset)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	total, err := handler.storage.CountChildren(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	httputil.WriteJSON(w, http.StatusOK, ListChildrenResponse{
		Data:       toPublicLocationViews(results),
		Limit:      params.Limit,
		Offset:     params.Offset,
		TotalCount: total,
	})
}

func toPublicLocationViews(locs []location.LocationWithParent) []location.PublicLocationView {
	views := make([]location.PublicLocationView, len(locs))
	for i, l := range locs {
		views[i] = location.ToPublicLocationView(l)
	}
	return views
}

type AddTagResponse struct {
	Data shared.Tag `json:"data"`
}

// @Summary Add a tag to a location
// @Tags locations,public
// @ID locations.tags.add
// @Accept json
// @Param location_id path int               true "Location ID"
// @Param request body shared.TagRequest true "Tag to attach"
// @Success 201 {object} locations.AddTagResponse "tag attached"
// @Failure 400 {object} modelerrors.ErrorResponse "bad_request"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "forbidden"
// @Failure 404 {object} modelerrors.ErrorResponse "not_found"
// @Failure 415 {object} modelerrors.ErrorResponse "unsupported_media_type"
// @Failure 429 {object} modelerrors.ErrorResponse "rate_limited"
// @Failure 500 {object} modelerrors.ErrorResponse "internal_error"
// @Security APIKey[locations:write]
// @Router /api/v1/locations/{location_id}/tags [post]
func (handler *Handler) AddTag(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, requestID)
		return
	}

	id, ok := handler.parseAndVerifyLocationID(w, r, orgID, requestID)
	if !ok {
		return
	}

	handler.doAddLocationTag(w, r, orgID, id)
}

func (handler *Handler) doAddLocationTag(w http.ResponseWriter, r *http.Request, orgID, locationID int) {
	requestID := middleware.GetRequestID(r.Context())

	var request shared.TagRequest
	if err := httputil.DecodeJSON(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, requestID)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, requestID)
		return
	}

	tag, err := handler.storage.AddTagToLocation(r.Context(), orgID, locationID, request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				err.Error(), requestID)

			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, AddTagResponse{Data: *tag})
}

// @Summary Remove a tag from a location
// @Description Detach a tag from a location by its tag record id.
// @Description Idempotent: returns 204 whether or not the tag was associated. Repeated calls are safe.
// @Tags locations,public
// @ID locations.tags.remove
// @Param location_id path int true "Location ID"
// @Param tag_id      path int true "Tag ID"
// @Success 204 "deleted"
// @Failure 400 {object} modelerrors.ErrorResponse "bad_request"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "forbidden"
// @Failure 429 {object} modelerrors.ErrorResponse "rate_limited"
// @Failure 500 {object} modelerrors.ErrorResponse "internal_error"
// @Security APIKey[locations:write]
// @Router /api/v1/locations/{location_id}/tags/{tag_id} [delete]
func (handler *Handler) RemoveTag(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, requestID)
		return
	}

	idParam := chi.URLParam(r, "location_id")
	id, err := httputil.ParseSurrogateID(idParam)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			err.Error(), requestID)

		return
	}

	loc, err := handler.storage.GetLocationByID(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), requestID)

		return
	}
	if loc == nil || loc.OrgID != orgID {
		httputil.Respond404(w, r, apierrors.LocationNotFound, requestID)
		return
	}

	handler.doRemoveLocationTag(w, r, orgID, loc.ID)
}

func (handler *Handler) doRemoveLocationTag(w http.ResponseWriter, r *http.Request, orgID, locationID int) {
	requestID := middleware.GetRequestID(r.Context())

	tagIDParam := chi.URLParam(r, "tag_id")
	tagID, err := strconv.Atoi(tagIDParam)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			err.Error(), requestID)

		return
	}

	_, err = handler.storage.RemoveLocationTag(r.Context(), orgID, locationID, tagID)
	if err != nil {
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// parseAndVerifyLocationID extracts {location_id}, parses it as a surrogate
// int, and verifies the location exists and belongs to the caller's org.
func (handler *Handler) parseAndVerifyLocationID(w http.ResponseWriter, req *http.Request, orgID int, reqID string) (int, bool) {
	idParam := chi.URLParam(req, "location_id")
	id, err := httputil.ParseSurrogateID(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			err.Error(), reqID)

		return 0, false
	}

	loc, err := handler.storage.GetLocationByID(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return 0, false
	}
	if loc == nil || loc.OrgID != orgID {
		httputil.Respond404(w, req, apierrors.LocationNotFound, reqID)
		return 0, false
	}

	return loc.ID, true
}
