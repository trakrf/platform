package notificationrecipients

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/notificationalert"
	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// parsed flattens parseAlertQuery's results for assertions.
type parsed struct {
	notificationalert.ListFilter
	Page, PerPage int
}

func parse(t *testing.T, raw string) (parsed, error) {
	t.Helper()
	q, err := url.ParseQuery(raw)
	require.NoError(t, err)
	f, page, perPage, err := parseAlertQuery(q)
	return parsed{f, page, perPage}, err
}

func fieldsOf(t *testing.T, err error) []string {
	t.Helper()
	var lpe *httputil.ListParamError
	require.True(t, errors.As(err, &lpe), "want *ListParamError, got %v", err)
	out := make([]string, 0, len(lpe.Fields))
	for _, f := range lpe.Fields {
		out = append(out, f.Field)
	}
	return out
}

func TestParseAlertQuery_Defaults(t *testing.T) {
	f, err := parse(t, "")
	require.NoError(t, err)
	require.Equal(t, 1, f.Page)
	require.Equal(t, 20, f.PerPage)
	require.Equal(t, 20, f.Limit)
	require.Equal(t, 0, f.Offset)
	require.Nil(t, f.AssetID)
	require.Nil(t, f.Type)
	require.Nil(t, f.From)
}

func TestParseAlertQuery_AllFilters(t *testing.T) {
	f, err := parse(t, "asset_id=4503599627370495&recipient_id=7&type=location_change&channel=sms&page=3&per_page=10")
	require.NoError(t, err)
	require.Equal(t, 4503599627370495, *f.AssetID, "52-bit obfuscated ids are accepted")
	require.Equal(t, 7, *f.RecipientID)
	require.Equal(t, notificationevent.SourceLocationChange, *f.Type)
	require.Equal(t, notificationdelivery.ChannelSMS, *f.Channel)
	require.Equal(t, 10, f.Limit)
	require.Equal(t, 20, f.Offset)
}

func TestParseAlertQuery_DateBoundsCoverWholeDays(t *testing.T) {
	f, err := parse(t, "from=2026-10-01&to=2026-10-01")
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), *f.From)
	require.Equal(t, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), *f.To, "a bare to date includes that whole day")
}

func TestParseAlertQuery_InstantToIsInclusive(t *testing.T) {
	f, err := parse(t, "from=2026-10-01T08:00:00Z&to=2026-10-01T09:00:00-04:00")
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC), f.From.UTC())
	require.Equal(t, time.Date(2026, 10, 1, 13, 0, 0, 1000, time.UTC), f.To.UTC())
}

func TestParseAlertQuery_RejectsBadValues(t *testing.T) {
	_, err := parse(t, "asset_id=abc&recipient_id=0&type=sms&channel=fax&page=0&per_page=101&from=yesterday")
	require.ElementsMatch(t,
		[]string{"asset_id", "recipient_id", "type", "channel", "page", "per_page", "from"},
		fieldsOf(t, err))
}

func TestParseAlertQuery_RejectsToBeforeFrom(t *testing.T) {
	_, err := parse(t, "from=2026-10-05&to=2026-10-01")
	require.Equal(t, []string{"to"}, fieldsOf(t, err))
}
