package main

// Version information - can be overridden at build time via ldflags
var (
	Version   = "v0.0.1"  // Semantic version
	CommitSHA = "unknown" // Git commit SHA (set via ldflags)
	BuildDate = "unknown" // Build timestamp (set via ldflags)
)

// To build with version info:
// go build -ldflags="-X main.Version=v0.0.1 -X main.CommitSHA=$(git rev-parse --short HEAD) -X main.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o rayls .
