// Package version holds build-time version information injected via ldflags.
package version

// Version is the semantic version of the build (e.g. v0.1.0).
var Version = "dev"

// Commit is the short Git commit SHA of the build.
var Commit = "unknown"

// BuildDate is the RFC3339 UTC timestamp at which the binary was built.
var BuildDate = "unknown"
