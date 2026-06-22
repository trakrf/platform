// Command trakrf is a scriptable CLI over the TrakRF public REST API. The
// typed API client is generated from the vendored OpenAPI spec (see ./api); the
// command layer only handles flags, auth/config, and output formatting.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/trakrf/platform/cli/internal/cmd"
)

// version is overridden at release time via -ldflags (see .goreleaser.yaml).
var version = "dev"

func main() {
	if err := cmd.NewApp(version).Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "Error: "+err.Error())
		os.Exit(1)
	}
}
