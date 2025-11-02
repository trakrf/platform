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

func DetectEnvironment() Environment {
	env := strings.ToLower(os.Getenv("ENVIRONMENT"))
	switch env {
	case "staging":
		return EnvStaging
	case "prod", "production":
		return EnvProd
	default:
		return EnvDev
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
		cfg.Level = getEnvOrDefault("LOG_LEVEL", "warn")
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
