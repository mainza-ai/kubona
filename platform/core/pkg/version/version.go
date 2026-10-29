// Package version carries the build version shown by the daemon, the A2A
// card, and the console About panel.
//
// The default is "dev". Release builds inject the tag at link time:
//
//	go build -ldflags "-X github.com/mainza-ai/kubona/core/pkg/version.Version=v0.3.0"
//
// A literal in source is not a version; it is a stale claim waiting to
// happen (the 2026-10-29 stale-state audit found "0.2.0-ts-go" hardcoded in
// three places). Keep this the only default.
package version

// Version is the build version. Overridden with -ldflags at release time.
var Version = "dev"
