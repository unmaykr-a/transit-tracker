// Package buildinfo holds version metadata injected at link time.
package buildinfo

// Set via -ldflags "-X github.com/unmaykr-a/transit-tracker/internal/buildinfo.Version=...".
var (
	Version = "dev"
	Commit  = "unknown"
)
