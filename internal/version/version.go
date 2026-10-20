// Package version reports the k4s build version.
package version

import "runtime/debug"

// Version is stamped at release time via
// -ldflags "-X github.com/LywwKkA-aD/k4s/internal/version.Version=v1.2.3".
var Version string

// String returns the build version: the ldflags stamp if present, else the
// module version recorded by `go install …@vX`, else "dev".
func String() string {
	return resolve(Version, debug.ReadBuildInfo)
}

func resolve(stamped string, readInfo func() (*debug.BuildInfo, bool)) string {
	if stamped != "" {
		return stamped
	}
	info, ok := readInfo()
	if !ok || info == nil || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "dev"
	}
	return info.Main.Version
}
