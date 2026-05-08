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
	preserveCollectionFormat(&doc2, doc3)
	return doc3, nil
}

// preserveCollectionFormat copies Swagger 2.0 query-parameter collectionFormat
// onto the v3 parameter as Style/Explode. kin-openapi's openapi2conv.ToV3
// drops collectionFormat (see its `// CollectionFormat: val.CollectionFormat`
// commented-out line), which loses the multi-vs-csv distinction needed for
// codegen. OpenAPI 3.0 mappings used here:
//
//	multi → style: form, explode: true   (?x=a&x=b — repeatable filters)
//	csv   → style: form, explode: false  (?x=a,b — sort)
//	ssv   → style: spaceDelimited
//	pipes → style: pipeDelimited
//
// tsv has no direct v3 equivalent and is left unset.
func preserveCollectionFormat(doc2 *openapi2.T, doc3 *openapi3.T) {
	if doc3.Paths == nil {
		return
	}
	for path, item2 := range doc2.Paths {
		if item2 == nil {
			continue
		}
		item3 := doc3.Paths.Find(path)
		if item3 == nil {
			continue
		}
		ops3 := item3.Operations()
		for method, op2 := range item2.Operations() {
			op3 := ops3[method]
			if op3 == nil {
				continue
			}
			for _, p2 := range op2.Parameters {
				if p2 == nil || p2.In != "query" || p2.CollectionFormat == "" {
					continue
				}
				for _, p3Ref := range op3.Parameters {
					if p3Ref == nil || p3Ref.Value == nil {
						continue
					}
					if p3Ref.Value.Name != p2.Name || p3Ref.Value.In != "query" {
						continue
					}
					applyCollectionFormat(p3Ref.Value, p2.CollectionFormat)
				}
			}
		}
	}
}

func applyCollectionFormat(p3 *openapi3.Parameter, fmt string) {
	t, f := true, false
	switch fmt {
	case "multi":
		p3.Style = "form"
		p3.Explode = &t
	case "csv":
		p3.Style = "form"
		p3.Explode = &f
	case "ssv":
		p3.Style = "spaceDelimited"
		p3.Explode = &f
	case "pipes":
		p3.Style = "pipeDelimited"
		p3.Explode = &f
	}
}
