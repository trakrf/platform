// Package buildinfo carries build-time metadata (version tag, git commit,
// build timestamp) from main into the rest of the backend. main holds the
// ldflags target vars; everything else reads from the Info struct.
package buildinfo

// Info captures the values injected at build time via -ldflags and the Go
// runtime version the binary was compiled with. All string fields are
// free-form; unset values default to "unknown" (or "dev" for Version/Tag).
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Tag       string `json:"tag"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}
