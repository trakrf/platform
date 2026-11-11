// Package phone normalizes the many ways people type a phone number into
// E.164, so opt-out lookups always match on one canonical form.
package phone

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nyaruka/phonenumbers"
)

var (
	// ErrInvalidNumber means the input is not a possible phone number.
	ErrInvalidNumber = errors.New("phone: invalid number")
	// ErrUnknownRegion means the region is not a known ISO 3166 country code.
	ErrUnknownRegion = errors.New("phone: unknown region")
)

// ValidateRegion reports whether region is a known country code. It is
// case-insensitive.
func ValidateRegion(region string) error {
	if phonenumbers.GetCountryCodeForRegion(strings.ToUpper(region)) == 0 {
		return ErrUnknownRegion
	}
	return nil
}

// NormalizeE164 converts raw to E.164 (e.g. +15551234567). Numbers without a
// country code use defaultRegion; numbers with one keep it. Errors never
// include raw, which is personal data.
func NormalizeE164(raw, defaultRegion string) (string, error) {
	if err := ValidateRegion(defaultRegion); err != nil {
		return "", err
	}
	num, err := phonenumbers.Parse(raw, strings.ToUpper(defaultRegion))
	if err != nil {
		return "", fmt.Errorf("%w: unparseable", ErrInvalidNumber)
	}
	// Possible, not valid: 555 numbers fail validity but are what people store.
	if !phonenumbers.IsPossibleNumber(num) {
		return "", fmt.Errorf("%w: not possible", ErrInvalidNumber)
	}
	return phonenumbers.Format(num, phonenumbers.E164), nil
}
