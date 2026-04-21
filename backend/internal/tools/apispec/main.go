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

	postprocessPublic(public)
	postprocessInternal(internal)

	if err := emit(public, publicOut); err != nil {
		return fmt.Errorf("emit public: %w", err)
	}
	if err := emit(internal, internalOut); err != nil {
		return fmt.Errorf("emit internal: %w", err)
	}
	return nil
}
