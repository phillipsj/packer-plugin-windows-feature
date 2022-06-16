package version

import (
	"github.com/hashicorp/packer-plugin-sdk/version"
)

var (
	Commit = "unknown"
	Date   = "unknown"
	// Version is the main version number that is being run at the moment.
	Version = "0.0.1"

	// Prerelease is A prerelease marker for the Version. If this is ""
	// (empty string) then it means that it is a final release. Otherwise, this
	// is a prerelease such as "dev" (in development), "beta", "rc1", etc.
	Prerelease = "dev"

	// PluginVersion is used by the plugin set to allow Packer to recognize
	// what version this plugin is.
	PluginVersion = version.InitializePluginVersion(Version, Prerelease)
)
