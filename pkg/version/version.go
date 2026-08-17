// Package version holds the plugin version, populated at build time.
package version

import (
	"fmt"
	"strings"
)

var (
	// GitCommit is the git commit that was compiled. Filled in by the compiler.
	GitCommit string

	// Version is the main version number that is being run at the moment.
	Version = "0.0.1"

	// VersionPrerelease is a pre-release marker for the version. If this is ""
	// (empty string) then it means that it is a final release. Otherwise, this
	// is a pre-release such as "dev" (in development), "beta", "rc1", etc.
	VersionPrerelease = "dev"
)

// GetHumanVersion composes the parts of the version in a way that's suitable
// for displaying to humans.
func GetHumanVersion() string {
	version := Version
	release := VersionPrerelease

	if release != "" {
		if !strings.HasSuffix(version, "-"+release) {
			version += fmt.Sprintf("-%s", release)
		}
	}

	return strings.ReplaceAll(version, "'", "")
}
