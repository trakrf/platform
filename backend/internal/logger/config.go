package logger

import (
	"fmt"
	"os"
	"strings"
)

type Environment string

const (
	EnvDev     Environment = "dev"
	EnvStaging Environment = "staging"
	EnvProd    Environment = "prod"
)

type Config struct {
	Environment    Environment
	ServiceName    string
	Level          string
	Format         string
	IncludeStack   bool
	IncludeCaller  bool
	ColorOutput    bool
	SanitizeEmails bool
	SanitizeIPs    bool
	MaxBodySize    int
	Version        string
}

// DetectEnvironment picks the log profile from APP_ENV, the same variable every
// other env gate in the backend reads (the deploy chart sets "prod"/"preview").
//
// Fail-CLOSED, like serve.testAffordancesAllowed: only the known local/CI values
// get the dev profile (debug, console, caller+stack); an unrecognized APP_ENV
// gets prod. Reading a variable the chart never set is how prod once ran the
// dev profile, with env=dev in every line.
func DetectEnvironment() Environment {
	switch strings.ToLower(os.Getenv("APP_ENV")) {
	case "", "test", "dev", "development", "local":
		return EnvDev
	case "staging", "preview":
		return EnvStaging
	default:
		return EnvProd
	}
}

func DetectServiceName() string {
	if name := os.Getenv("SERVICE_NAME"); name != "" {
		return name
	}
	return "platform-backend"
}

func NewConfig(version string) *Config {
	env := DetectEnvironment()

	cfg := &Config{
		Environment: env,
		ServiceName: DetectServiceName(),
		Version:     version,
	}

	switch env {
	case EnvDev:
		cfg.Level = getEnvOrDefault("LOG_LEVEL", "debug")
		cfg.Format = getEnvOrDefault("LOG_FORMAT", "console")
		cfg.IncludeStack = getBoolEnv("LOG_INCLUDE_STACK", true)
		cfg.IncludeCaller = getBoolEnv("LOG_INCLUDE_CALLER", true)
		cfg.ColorOutput = getBoolEnv("LOG_COLOR", true)
		cfg.SanitizeEmails = getBoolEnv("LOG_SANITIZE_EMAILS", false)
		cfg.SanitizeIPs = getBoolEnv("LOG_SANITIZE_IPS", false)
		cfg.MaxBodySize = getIntEnv("LOG_MAX_BODY_SIZE", 1000)

	case EnvStaging:
		cfg.Level = getEnvOrDefault("LOG_LEVEL", "info")
		cfg.Format = getEnvOrDefault("LOG_FORMAT", "json")
		cfg.IncludeStack = getBoolEnv("LOG_INCLUDE_STACK", false)
		cfg.IncludeCaller = getBoolEnv("LOG_INCLUDE_CALLER", false)
		cfg.ColorOutput = getBoolEnv("LOG_COLOR", false)
		cfg.SanitizeEmails = getBoolEnv("LOG_SANITIZE_EMAILS", true)
		cfg.SanitizeIPs = getBoolEnv("LOG_SANITIZE_IPS", true)
		cfg.MaxBodySize = getIntEnv("LOG_MAX_BODY_SIZE", 0)

	case EnvProd:
		// info, not warn: startup, MQTT subscriber, request and api.write audit
		// lines are INF and are what operators read in prod.
		cfg.Level = getEnvOrDefault("LOG_LEVEL", "info")
		cfg.Format = getEnvOrDefault("LOG_FORMAT", "json")
		cfg.IncludeStack = getBoolEnv("LOG_INCLUDE_STACK", false)
		cfg.IncludeCaller = getBoolEnv("LOG_INCLUDE_CALLER", false)
		cfg.ColorOutput = getBoolEnv("LOG_COLOR", false)
		cfg.SanitizeEmails = getBoolEnv("LOG_SANITIZE_EMAILS", true)
		cfg.SanitizeIPs = getBoolEnv("LOG_SANITIZE_IPS", true)
		cfg.MaxBodySize = getIntEnv("LOG_MAX_BODY_SIZE", 0)
	}

	return cfg
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getBoolEnv(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val == "true" || val == "1"
}

func getIntEnv(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	var result int
	if _, err := fmt.Sscanf(val, "%d", &result); err != nil {
		return defaultVal
	}
	return result
}
