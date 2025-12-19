package location

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// FlexibleDate is a custom type that can unmarshal from multiple date formats
type FlexibleDate struct {
	time.Time
}

// Supported date formats
var dateFormats = []string{
	time.RFC3339,          // "2006-01-02T15:04:05Z07:00"
	time.RFC3339Nano,      // "2006-01-02T15:04:05.999999999Z07:00"
	"2006-01-02",          // ISO 8601: "2025-12-14"
	"2006-01-02 15:04:05", // ISO with time: "2025-12-14 10:30:00"
	"01/02/2006",          // US format: "12/14/2025"
	"02/01/2006",          // UK format: "14/12/2025"
	"02.01.2006",          // European format: "14.12.2025"
	"01.02.2006",          // Alternative: "12.14.2025"
	"2006/01/02",          // ISO slashes: "2025/12/14"
}

// UnmarshalJSON implements custom JSON unmarshaling for flexible date parsing
func (fd *FlexibleDate) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), "\"")

	// Handle empty string
	if s == "null" || s == "" {
		return nil
	}

	// Try each format
	for _, format := range dateFormats {
		t, err := time.Parse(format, s)
		if err == nil {
			fd.Time = t
			return nil
		}
	}

	return fmt.Errorf("invalid date format '%s'. Supported formats: YYYY-MM-DD, YYYY-MM-DDTHH:MM:SSZ, MM/DD/YYYY, DD/MM/YYYY", s)
}

// MarshalJSON implements custom JSON marshaling (uses RFC3339 for output)
func (fd FlexibleDate) MarshalJSON() ([]byte, error) {
	if fd.Time.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(fd.Time.Format(time.RFC3339))
}

// ToTime converts FlexibleDate to time.Time
func (fd FlexibleDate) ToTime() time.Time {
	return fd.Time
}
