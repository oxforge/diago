package main

import "runtime/debug"

// version is diago's version, from the build information Go embeds in every
// binary: a release tag, or a pseudo-version naming the commit it was built
// from (+dirty for a modified tree); "(devel)" when the binary carries none.
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "(devel)"
	}
	return versionOf(info)
}

// versionOf reads the main module's version from build information.
func versionOf(info *debug.BuildInfo) string {
	if v := info.Main.Version; v != "" {
		return v
	}
	return "(devel)"
}
