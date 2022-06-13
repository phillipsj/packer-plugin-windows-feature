package main

import (
	"log"

	"github.com/hashicorp/packer-plugin-sdk/plugin"
	v "github.com/hashicorp/packer-plugin-sdk/version"
	"github.com/phillipsj/packer-plugin-windows-feature/feature"
)

var (
	version = "0.0.0"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	log.Printf("Starting packer-plug-windows-feature (version %s; commit %s; date %s)", version, commit, date)
	pps := plugin.NewSet()
	pps.RegisterProvisioner(plugin.DEFAULT_NAME, new(feature.Provisioner))
	pps.SetVersion(v.InitializePluginVersion(version, ""))
	if err := pps.Run(); err != nil {
		log.Fatalf("Failed to start: %v", err)
	}
}
