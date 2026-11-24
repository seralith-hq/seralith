// Package version exposes build metadata injected at link time.
package version

import (
	"fmt"
	"runtime"
)

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// String returns a human-readable build description.
func String() string {
	return fmt.Sprintf("%s (commit %s, built %s, %s/%s)", Version, Commit, Date, runtime.GOOS, runtime.GOARCH)
}
