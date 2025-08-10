package app

// Build information populated via -ldflags at build time by Dockerfile/CI.
// Defaults are meaningful for local development and tests.
var (
    // BuildVersion is the semantic version of the built binary.
    BuildVersion = "0.0.0-dev"
    // BuildCommit is the VCS commit SHA associated with the build.
    BuildCommit  = "unknown"
    // BuildDate is the ISO-8601 timestamp of the build.
    BuildDate    = "unknown"
)
