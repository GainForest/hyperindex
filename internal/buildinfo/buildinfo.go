// Package buildinfo exposes build-time metadata embedded into the backend binary.
package buildinfo

// Version is the backend version reported by public status endpoints.
// Build commands may replace it with Go ldflags, for example:
// -ldflags "-X github.com/GainForest/hyperindex/internal/buildinfo.Version=v1.2.3".
var Version = "0.1.0-dev"
