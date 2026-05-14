package shared

import (
	"database/sql/driver"
	"encoding/json"
	"reflect"
	"strings"
	"time"
)

// FlexibleDate is a JSON wrapper around time.Time that accepts only strict
// RFC 3339 date-time strings. The historical name is preserved (the type
// once accepted multiple loose formats — slashes, date-only, European
// punctuation), but TRA-649 / BB23 F2 reduced the surface to RFC 3339 to
// match both the OpenAPI declaration (`format: date-time`) and the strict
// query-param validator on /api/v1/assets/{asset_id}/history. Empty string,
// the Go zero time (`0001-01-01T00:00:00Z`), and the Unix epoch
// (`1970-01-01T00:00:00Z`) are rejected so that a missing value cannot
// silently become a server-substituted default at the handler seam — both
// are programming-language default-value markers that almost always mean an
// upstream ETL forgot to map "unset" to null (TRA-704 / BB32 C4).
type FlexibleDate struct {
	time.Time
}

// unixEpochUTC is the second rejected sentinel (1970-01-01T00:00:00Z).
// The Go zero time is detected via time.IsZero() — there is no equivalent
// helper for Unix epoch, so the comparison is explicit.
var unixEpochUTC = time.Unix(0, 0).UTC()

// IsSentinelTimestamp reports whether t is one of the two rejected
// default-value markers: the Go zero time or the Unix epoch. Exported so
// httputil.RespondDecodeError can branch on the original parsed value and
// surface the sentinel-specific guidance instead of the generic
// "must be RFC 3339" message (TRA-704 / BB32 C4).
func IsSentinelTimestamp(s string) bool {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return false
	}
	return t.IsZero() || t.Equal(unixEpochUTC)
}

// UnmarshalJSON implements strict RFC 3339 JSON unmarshaling.
//
// Accepts: time.RFC3339 / time.RFC3339Nano, JSON null (treated as
// "field omitted" — the surrounding pointer remains nil so handlers can
// distinguish absence from an explicit value).
//
// Rejects: empty string, the two sentinel default-value timestamps (Go
// zero time and Unix epoch — TRA-704 / BB32 C4), and any other format. All
// rejections surface as *json.UnmarshalTypeError with Type == time.Time so
// httputil.RespondDecodeError can render them as a validation_error keyed
// on the offending field path (TRA-641 / BB21 §2.1).
func (fd *FlexibleDate) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), "\"")

	if s == "null" {
		return nil
	}

	if s != "" {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil && !t.IsZero() && !t.Equal(unixEpochUTC) {
			fd.Time = t
			return nil
		}
	}

	return &json.UnmarshalTypeError{
		Value: string(b),
		Type:  reflect.TypeOf(time.Time{}),
	}
}

// MarshalJSON emits the canonical public-surface shape (RFC 3339 with
// three-digit millisecond fractional precision, UTC; see
// FormatPublicTime). Zero renders as JSON null so "field not provided"
// on a request echo stays distinguishable from a real timestamp.
// Aligned with PublicTime's outbound formatter per TRA-717 / BB34 F3
// rework so the wire shape is uniform across both types.
func (fd FlexibleDate) MarshalJSON() ([]byte, error) {
	if fd.Time.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(FormatPublicTime(fd.Time))
}

// ToTime converts FlexibleDate to time.Time
func (fd FlexibleDate) ToTime() time.Time {
	return fd.Time
}

// Value implements driver.Valuer so pgx/pq encode the underlying time.Time.
// A zero FlexibleDate is encoded as SQL NULL.
func (fd FlexibleDate) Value() (driver.Value, error) {
	if fd.Time.IsZero() {
		return nil, nil
	}
	return fd.Time, nil
}
