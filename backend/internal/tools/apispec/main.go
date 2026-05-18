// Package main is the apispec CLI tool. It converts swaggo-generated
// OpenAPI 2.0 specs into partitioned OpenAPI 3.0 specs: one public spec
// (operations tagged "public") and one internal spec (operations tagged
// "internal"), with post-processing to match TRA-392's documented contract.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var (
		inPath      = flag.String("in", "", "Path to swagger 2.0 JSON (required)")
		publicOut   = flag.String("public-out", "", "Output path prefix for public spec (required, writes .json and .yaml)")
		internalOut = flag.String("internal-out", "", "Output path prefix for internal spec (required, writes .json and .yaml)")
	)
	flag.Parse()

	if *inPath == "" || *publicOut == "" || *internalOut == "" {
		fmt.Fprintln(os.Stderr, "usage: apispec --in <path> --public-out <prefix> --internal-out <prefix>")
		os.Exit(2)
	}

	if err := run(*inPath, *publicOut, *internalOut); err != nil {
		fmt.Fprintf(os.Stderr, "apispec: %v\n", err)
		os.Exit(1)
	}
}

func run(inPath, publicOut, internalOut string) error {
	data, err := os.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", inPath, err)
	}

	doc3, err := convertV2ToV3(data)
	if err != nil {
		return err
	}

	public, internal, err := partition(doc3)
	if err != nil {
		return err
	}

	// TRA-780 F2: postprocessInternal must run AND emit before postprocessPublic.
	// partition() shallow-copies the doc, so public and internal share
	// *openapi3.SchemaRef pointers through Components. postprocessPublic's
	// renamePublicSpec rewrites $refs in place (e.g. errors.ErrorEnvelope →
	// ErrorEnvelope), which leaks into internal because the SchemaRef objects
	// are shared. Emitting the internal spec before postprocessPublic mutates
	// shared state keeps the internal spec self-consistent.
	//
	// Pre-TRA-780 the issue was latent: no nested $ref between two errors.*
	// schemas existed, so the public rename never produced an in-place rewrite
	// the internal spec would notice. F2 introduced errors.ErrorResponse →
	// $ref errors.ErrorEnvelope, exposing the bug.
	if err := postprocessInternal(internal); err != nil {
		return fmt.Errorf("postprocess internal: %w", err)
	}
	if err := emit(internal, internalOut); err != nil {
		return fmt.Errorf("emit internal: %w", err)
	}

	if err := postprocessPublic(public); err != nil {
		return fmt.Errorf("postprocess public: %w", err)
	}
	if err := emit(public, publicOut); err != nil {
		return fmt.Errorf("emit public: %w", err)
	}
	return nil
}
