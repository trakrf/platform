// Package api holds the vendored OpenAPI spec and the oapi-codegen-generated
// typed client + models for the TrakRF public REST API.
//
// The client is generated, never hand-edited. To refresh it after a spec change:
//
//	just regen        # re-vendor the spec from docs/api + regenerate
//
// or directly:
//
//	go generate ./api
//
//go:generate go tool oapi-codegen -config cfg.yaml openapi.public.yaml
package api
