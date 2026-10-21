package routing

import (
	"fmt"
	"os"
	"strings"

	"github.com/trakrf/platform/backend/internal/notification/phone"
)

// DefaultPhoneRegion is the country assumed for phone numbers entered without
// a country code, when NOTIFICATION_DEFAULT_PHONE_REGION is not set.
const DefaultPhoneRegion = "US"

// Config holds the delivery planner's environment settings.
type Config struct {
	PhoneRegion string
}

// ConfigFromEnv reads the planner settings and rejects an unknown phone region.
func ConfigFromEnv() (Config, error) {
	region := strings.ToUpper(strings.TrimSpace(os.Getenv("NOTIFICATION_DEFAULT_PHONE_REGION")))
	if region == "" {
		region = DefaultPhoneRegion
	}
	if err := phone.ValidateRegion(region); err != nil {
		return Config{}, fmt.Errorf("NOTIFICATION_DEFAULT_PHONE_REGION %q: %w", region, err)
	}
	return Config{PhoneRegion: region}, nil
}
