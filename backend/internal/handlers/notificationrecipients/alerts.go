package notificationrecipients

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
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
	dateOnly             = "2006-01-02"
)

var alertQueryParams = []string{"asset_id", "recipient_id", "type", "channel", "from", "to", "page", "per_page"}

type listAlertsResponse struct {
	Data       []notificationalert.Alert `json:"data"`
	Pagination shared.Pagination         `json:"pagination"`
}

// ListAlerts returns sent notifications, newest first, paginated by page and
// per_page. Filters: asset_id, recipient_id, type (alarm, location_change),
// channel (email, sms), and from/to on the sent time. from and to take an
// RFC 3339 instant or a YYYY-MM-DD date, both inclusive; a bare to date covers
// that whole UTC day.
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
// as a *httputil.ListParamError.
func parseAlertQuery(q url.Values) (notificationalert.ListFilter, int, int, error) {
	var (
		f      notificationalert.ListFilter
		fields []apierrors.FieldError
	)
	fail := func(field, code, msg string) {
		fields = append(fields, apierrors.FieldError{Field: field, Code: code, Message: msg})
	}
	intParam := func(field string, min, max int) (int, bool) {
		raw := q.Get(field)
		if raw == "" {
			return 0, false
		}
		n, err := strconv.Atoi(raw)
		switch {
		case err != nil:
			fail(field, "invalid_value", field+" must be an integer")
		case n < min:
			fail(field, "too_small", fmt.Sprintf("%s must be ≥ %d", field, min))
		case n > max:
			fail(field, "too_large", fmt.Sprintf("%s must be ≤ %d", field, max))
		default:
			return n, true
		}
		return 0, false
	}

	page := 1
	if n, ok := intParam("page", 1, math.MaxInt32); ok {
		page = n
	}
	perPage := defaultAlertsPerPage
	if n, ok := intParam("per_page", 1, maxAlertsPerPage); ok {
		perPage = n
	}
	if n, ok := intParam("asset_id", 1, math.MaxInt); ok {
		f.AssetID = &n
	}
	if n, ok := intParam("recipient_id", 1, math.MaxInt); ok {
		f.RecipientID = &n
	}

	switch v := notificationevent.Source(q.Get("type")); v {
	case "":
	case notificationevent.SourceAlarm, notificationevent.SourceLocationChange:
		f.Type = &v
	default:
		fail("type", "invalid_value", "type must be one of: alarm, location_change")
	}

	switch v := notificationdelivery.Channel(q.Get("channel")); v {
	case "":
	case notificationdelivery.ChannelEmail, notificationdelivery.ChannelSMS:
		f.Channel = &v
	default:
		fail("channel", "invalid_value", "channel must be one of: email, sms")
	}

	timeParam := func(field string, upper bool) *time.Time {
		raw := q.Get(field)
		if raw == "" {
			return nil
		}
		if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			if upper {
				// An instant to is inclusive; storage bounds it exclusively.
				t = t.Add(time.Microsecond)
			}
			return &t
		}
		if t, err := time.Parse(dateOnly, raw); err == nil {
			if upper {
				t = t.AddDate(0, 0, 1)
			}
			return &t
		}
		fail(field, "invalid_value", field+" must be an RFC 3339 timestamp or a YYYY-MM-DD date")
		return nil
	}
	f.From = timeParam("from", false)
	f.To = timeParam("to", true)
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		fail("to", "invalid_value", "to must be later than from")
	}

	if len(fields) > 0 {
		return notificationalert.ListFilter{}, 0, 0, &httputil.ListParamError{Fields: fields}
	}
	f.Limit = perPage
	f.Offset = (page - 1) * perPage
	return f, page, perPage, nil
}
