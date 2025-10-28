package i18n

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	translations map[string]interface{}
	once         sync.Once
	loadError    error
)

func init() {
	Load()
}

func Load() error {
	once.Do(func() {
		localePath := findLocaleFile()
		if localePath == "" {
			loadError = fmt.Errorf("failed to locate locale file")
			return
		}

		data, err := os.ReadFile(localePath)
		if err != nil {
			loadError = fmt.Errorf("failed to read locale file: %w", err)
			return
		}

		var root map[string]interface{}
		if err := yaml.Unmarshal(data, &root); err != nil {
			loadError = fmt.Errorf("failed to parse locale file: %w", err)
			return
		}

		enData, ok := root["en"].(map[string]interface{})
		if !ok {
			loadError = fmt.Errorf("invalid locale file structure: missing 'en' key")
			return
		}

		translations = enData
	})

	return loadError
}

func findLocaleFile() string {
	possiblePaths := []string{
		filepath.Join("locales", "en.yml"),
		filepath.Join("..", "..", "locales", "en.yml"),
		filepath.Join("backend", "locales", "en.yml"),
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

func T(key string, params ...map[string]interface{}) string {
	if loadError != nil {
		return key
	}

	value := lookup(key)
	if value == "" {
		return key
	}

	if len(params) > 0 {
		return interpolate(value, params[0])
	}

	return value
}

func lookup(key string) string {
	parts := strings.Split(key, ".")
	current := translations

	for i, part := range parts {
		if current == nil {
			return ""
		}

		value, exists := current[part]
		if !exists {
			return ""
		}

		if i == len(parts)-1 {
			if str, ok := value.(string); ok {
				return str
			}
			return ""
		}

		if next, ok := value.(map[string]interface{}); ok {
			current = next
		} else {
			return ""
		}
	}

	return ""
}

func interpolate(template string, params map[string]interface{}) string {
	result := template
	for key, value := range params {
		placeholder := fmt.Sprintf("%%{%s}", key)
		replacement := fmt.Sprintf("%v", value)
		result = strings.ReplaceAll(result, placeholder, replacement)
	}
	return result
}
