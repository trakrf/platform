// Package migrations holds the versioned SQL migration files for the TrakRF
// platform backend. The files are embedded into the binary at build time so
// they travel with whichever binary consumes them (both server startup and
// the standalone `server migrate` subcommand).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
