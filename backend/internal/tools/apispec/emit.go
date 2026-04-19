package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

// emit writes doc to <prefix>.json and <prefix>.yaml. The JSON is
// pretty-printed with two-space indentation; the YAML uses the default
// gopkg.in/yaml.v3 encoding.
func emit(doc *openapi3.T, prefix string) error {
	jsonBytes, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	if err := os.WriteFile(prefix+".json", jsonBytes, 0o644); err != nil {
		return fmt.Errorf("write %s.json: %w", prefix, err)
	}

	// Round-trip through JSON so kin-openapi's JSON-tag-driven serialization
	// is used (openapi3.T is JSON-first; direct yaml.Marshal drops fields).
	var asMap map[string]any
	if err := json.Unmarshal(jsonBytes, &asMap); err != nil {
		return fmt.Errorf("round-trip JSON→map: %w", err)
	}
	yamlBytes, err := yaml.Marshal(asMap)
	if err != nil {
		return fmt.Errorf("marshal YAML: %w", err)
	}
	if err := os.WriteFile(prefix+".yaml", yamlBytes, 0o644); err != nil {
		return fmt.Errorf("write %s.yaml: %w", prefix, err)
	}
	return nil
}
