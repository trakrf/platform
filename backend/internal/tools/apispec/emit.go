package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

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
	// UseNumber preserves integer literals; otherwise json.Decoder turns every
	// JSON number into float64 and yaml.Marshal would emit `2147483647` as
	// `2.147483647e+09` — pairing a float bound with `type: integer` trips
	// strict codegens (TRA-657 / BB25 B1).
	dec := json.NewDecoder(bytes.NewReader(jsonBytes))
	dec.UseNumber()
	var asMap map[string]any
	if err := dec.Decode(&asMap); err != nil {
		return fmt.Errorf("round-trip JSON→map: %w", err)
	}
	coerceJSONNumbers(asMap)
	yamlBytes, err := yaml.Marshal(asMap)
	if err != nil {
		return fmt.Errorf("marshal YAML: %w", err)
	}
	if err := os.WriteFile(prefix+".yaml", yamlBytes, 0o644); err != nil {
		return fmt.Errorf("write %s.yaml: %w", prefix, err)
	}
	return nil
}

// coerceJSONNumbers walks v in place and replaces every json.Number with
// int64 when it parses as an integer, falling back to float64. yaml.v3
// emits int64 as a bare integer literal and float64 as decimal/scientific,
// which preserves the JSON literal shape across the round-trip.
func coerceJSONNumbers(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if n, ok := val.(json.Number); ok {
				x[k] = numberToTyped(n)
				continue
			}
			coerceJSONNumbers(val)
		}
	case []any:
		for i, val := range x {
			if n, ok := val.(json.Number); ok {
				x[i] = numberToTyped(n)
				continue
			}
			coerceJSONNumbers(val)
		}
	}
}

func numberToTyped(n json.Number) any {
	if i, err := strconv.ParseInt(n.String(), 10, 64); err == nil {
		return i
	}
	if f, err := n.Float64(); err == nil {
		return f
	}
	return n.String()
}
