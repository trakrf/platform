package notificationrecipients

import (
	"errors"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/trakrf/platform/backend/internal/middleware"
	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/notificationalert"
	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

const (
	defaultAlertsPerPage = 20
	maxAlertsPerPage     = 100
)

var alertQueryParams = []string{"asset_id", "recipient_id", "type", "channel", "from", "to", "page", "per_page"}

type listAlertsResponse struct {
	Data       []notificationalert.Alert `json:"data"`
	Pagination shared.Pagination         `json:"pagination"`
}

// ListAlerts returns sent notifications, newest first, paginated by page and
// per_page. Filters: asset_id, recipient_id, type (alarm, location_change),
// channel (email, sms), and from/to on the sent time as inclusive RFC 3339
// instants.
func (h *Handler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	if err := httputil.RejectUnknownQueryParams(r, alertQueryParams...); err != nil {
		httputil.RespondListParamError(w, r, err, reqID)
		return
	}
	filter, page, perPage, err := parseAlertQuery(r.URL.Query())
	if err != nil {
		httputil.RespondListParamError(w, r, err, reqID)
		return
	}
	alerts, total, err := h.storage.ListNotificationAlerts(r.Context(), orgID, filter)
	if err != nil {
		httputil.RespondStorageError(w, r, err, reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, listAlertsResponse{
		Data:       alerts,
		Pagination: shared.Pagination{Page: page, PerPage: perPage, Total: total},
	})
}

// parseAlertQuery validates every parameter and reports all failures at once
// as a *httputil.ListParamError. A parameter that is present but empty is an
// error, not an absent filter: a client with an unset asset_id must not get
// the whole org's alerts.
func parseAlertQuery(q url.Values) (notificationalert.ListFilter, int, int, error) {
	var (
		f      notificationalert.ListFilter
		fields []apierrors.FieldError
	)
	fail := func(field, msg string) {
		fields = append(fields, apierrors.FieldError{Field: field, Code: "invalid_value", Message: msg})
	}
	intParam := func(field string, min, max int64) (int, bool) {
		if !q.Has(field) {
			return 0, false
		}
		n, err := httputil.ParsePathInt(field, q.Get(field), min, max)
		if err != nil {
			var fpe *httputil.FieldParamError
			if errors.As(err, &fpe) {
				fields = append(fields, fpe.FieldError)
			}
			return 0, false
		}
		return n, true
	}

	page := 1
	if n, ok := intParam("page", 1, math.MaxInt32); ok {
		page = n
	}
	perPage := defaultAlertsPerPage
	if n, ok := intParam("per_page", 1, maxAlertsPerPage); ok {
		perPage = n
	}
	if n, ok := intParam("asset_id", 1, math.MaxInt64); ok {
		f.AssetID = &n
	}
	if n, ok := intParam("recipient_id", 1, math.MaxInt64); ok {
		f.RecipientID = &n
	}

	if q.Has("type") {
		switch v := notificationevent.Source(q.Get("type")); v {
		case notificationevent.SourceAlarm, notificationevent.SourceLocationChange:
			f.Type = &v
		default:
			fail("type", "type must be one of: alarm, location_change")
		}
	}

	if q.Has("channel") {
		switch v := notificationdelivery.Channel(q.Get("channel")); v {
		case notificationdelivery.ChannelEmail, notificationdelivery.ChannelSMS:
			f.Channel = &v
		default:
			fail("channel", "channel must be one of: email, sms")
		}
	}

	// Bare dates are rejected rather than read as UTC days: the caller knows
	// the viewer's zone and the server does not, so a local day must arrive
	// as instants.
	timeParam := func(field string) *time.Time {
		if !q.Has(field) {
			return nil
		}
		t, err := time.Parse(time.RFC3339Nano, q.Get(field))
		if err != nil {
			fail(field, field+" must be an RFC 3339 timestamp with a time zone, e.g. 2026-10-20T00:00:00-04:00")
			return nil
		}
		return &t
	}
	f.From = timeParam("from")
	if to := timeParam("to"); to != nil {
		// to is inclusive; storage bounds it exclusively.
		end := to.Add(time.Microsecond)
		f.To = &end
		if f.From != nil && f.From.After(*to) {
			fail("to", "to must not be earlier than from")
		}
	}

	if len(fields) > 0 {
		return notificationalert.ListFilter{}, 0, 0, &httputil.ListParamError{Fields: fields}
	}
	f.Limit = perPage
	f.Offset = (page - 1) * perPage
	return f, page, perPage, nil
}
