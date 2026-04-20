package main

import (
	"encoding/json"
	"fmt"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
)

// convertV2ToV3 reads swaggo-emitted OpenAPI 2.0 JSON and returns the
// equivalent 3.0 document.
func convertV2ToV3(data []byte) (*openapi3.T, error) {
	var doc2 openapi2.T
	if err := json.Unmarshal(data, &doc2); err != nil {
		return nil, fmt.Errorf("parse v2 JSON: %w", err)
	}
	doc3, err := openapi2conv.ToV3(&doc2)
	if err != nil {
		return nil, fmt.Errorf("convert v2 → v3: %w", err)
	}
	return doc3, nil
}
