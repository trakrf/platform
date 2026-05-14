package shared

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"
)

// publicTimeLayout is RFC 3339 with fixed three-digit fractional precision.
// Used by PublicTime.MarshalJSON and FlexibleDate.MarshalJSON so every
// outbound timestamp on the public surface emits the same shape
// regardless of which Go-side type backs the field (TRA-717 / BB34 F3
// rework). Postgres timestamptz stores microseconds; the bottom three
// digits are false precision relative to server-receipt-time jitter and
// to what any mainstream client app reasons about, so the wire is
// truncated to milliseconds.
const publicTimeLayout = "2006-01-02T15:04:05.000Z07:00"

// FormatPublicTime renders t in the canonical public-surface shape
// (RFC 3339, UTC, three-digit millisecond fractional, e.g.
// "2026-05-14T12:34:56.123Z"). Exported so non-time-typed call sites
// (string columns in reports, log lines, etc.) can emit the same shape
// without going through a JSON marshal.
func FormatPublicTime(t time.Time) string {
	return t.UTC().Format(publicTimeLayout)
}

// PublicTime is the wire-format wrapper for outbound time.Time fields on
// public response DTOs. Marshal output is the canonical public-surface
// shape (see FormatPublicTime). Zero values are emitted as the formatted
// zero time, not JSON null — nullable response fields use
// `*PublicTime` and rely on the Go json encoder's nil-to-null behavior
// (TRA-717 / BB34 F3 rework).
//
// UnmarshalJSON accepts any RFC 3339 input (0–9 fractional digits) so
// any well-formed timestamp round-trips through the type without parse
// error, even if the consumer kept microsecond precision from an
// upstream source.
type PublicTime struct {
	time.Time
}

// NewPublicTime wraps a time.Time in a PublicTime value. Use in
// converters that project internal models onto public DTOs.
func NewPublicTime(t time.Time) PublicTime {
	return PublicTime{Time: t}
}

// PublicTimePtr wraps an optional time.Time pointer. Returns nil when
// the source is nil so the JSON encoder emits null (matches the
// non-omitempty nullable contract on public DTO fields per
// TRA-610 / BB18 §1.8 + §1.10).
func PublicTimePtr(t *time.Time) *PublicTime {
	if t == nil {
		return nil
	}
	return &PublicTime{Time: *t}
}

// MarshalJSON emits the canonical public-surface shape. A zero
// PublicTime renders as the formatted zero time (not JSON null) so
// non-nullable response fields like PublicLocationView.updated_at
// always carry a string.
func (pt PublicTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(FormatPublicTime(pt.Time))
}

// UnmarshalJSON accepts RFC 3339 with any fractional precision (0–9
// digits). JSON null and empty string leave the receiver at its zero
// value so a pointer field's "field omitted" semantic is preserved.
// Any other input surfaces as *json.UnmarshalTypeError keyed on
// time.Time so httputil.RespondDecodeError can render the standard
// validation_error envelope.
func (pt *PublicTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), "\"")
	if s == "null" || s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return &json.UnmarshalTypeError{
			Value: string(b),
			Type:  reflect.TypeOf(time.Time{}),
		}
	}
	pt.Time = t
	return nil
}
