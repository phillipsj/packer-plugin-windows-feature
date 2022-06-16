package main

import (
	"log"

	"github.com/hashicorp/packer-plugin-sdk/plugin"
	"github.com/phillipsj/packer-plugin-windows-feature/feature"
	pluginVersion "github.com/phillipsj/packer-plugin-windows-feature/version"
)

func main() {
	log.Printf("Starting packer-plugin-windows-feature (version %s; prelease %s; commit %s; date %s)",
		pluginVersion.Version, pluginVersion.Prerelease, pluginVersion.Commit, pluginVersion.Date)
	pps := plugin.NewSet()
	pps.RegisterProvisioner(plugin.DEFAULT_NAME, new(feature.Provisioner))
	pps.SetVersion(pluginVersion.PluginVersion)
	if err := pps.Run(); err != nil {
		log.Fatalf("Failed to start: %v", err)
	}
}
