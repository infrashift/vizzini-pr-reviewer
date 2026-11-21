// Package buildinfo reports the version of a Vizzini binary.
//
// A release build sets Version, Commit and Date through -ldflags -X (see the
// Makefile's LDFLAGS). A `go install ...@vX.Y.Z` build has no ldflags, so the
// module version and VCS data that the Go toolchain embeds fill the gaps.
package buildinfo

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// Set by -ldflags -X at release time.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// String returns "NAME VERSION (commit COMMIT, built DATE, GOOS/GOARCH)".
func String(name string) string {
	v, c, d := Version, Commit, Date
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			v = bi.Main.Version
		}
		for _, s := range bi.Settings {
			switch {
			case s.Key == "vcs.revision" && c == "unknown":
				c = s.Value
			case s.Key == "vcs.time" && d == "unknown":
				d = s.Value
			}
		}
	}
	return fmt.Sprintf("%s %s (commit %s, built %s, %s/%s)", name, v, c, d, runtime.GOOS, runtime.GOARCH)
}

// Requested reports whether args is exactly a version request: --version or
// -version. Callers check it before parsing flags, so a version request does
// not fail on missing required flags.
func Requested(args []string) bool {
	return len(args) == 1 && (args[0] == "--version" || args[0] == "-version")
}
