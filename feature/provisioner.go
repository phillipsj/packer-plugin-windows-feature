//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package feature

import (
	"context"
	_ "embed" // this is needed for using the go:embed directive
	"errors"
	"time"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

type Config struct {
	common.PackerConfig `mapstructure:",squash"`

	// The timeout for waiting for the machine to restart
	RestartTimeout time.Duration `mapstructure:"restart_timeout"`

	// Instructs the communicator to run the remote script as a
	// Windows scheduled task, effectively elevating the remote
	// user by impersonating a logged-in user.
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`

	// Windows features to install. If no features are
	// defined then the installation is skipped.
	Features []string `mapstructure:"features"`

	// Windows capabilities to install. If no features are
	// defined then the installation is skipped.
	Capabilities int `mapstructure:"capabilities"`

	ctx interpolate.Context
}

type Provisioner struct {
	config Config
}

func (b *Provisioner) ConfigSpec() hcldec.ObjectSpec {
	return b.config.FlatMapstructure().HCL2Spec()
}

func (p *Provisioner) Prepare(raws ...interface{}) error {
	err := config.Decode(&p.config, &config.DecodeOpts{
		Interpolate:        true,
		InterpolateContext: &p.config.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{
				"execute_command",
			},
		},
	}, raws...)
	if err != nil {
		return err
	}

	if p.config.RestartTimeout == 0 {
		p.config.RestartTimeout = 4 * time.Hour
	}

	if p.config.Username == "" {
		p.config.Username = "SYSTEM"
	}

	var errs error

	if p.config.Username == "" {
		errs = packer.MultiErrorAppend(errs,
			errors.New("must supply an 'username'"))
	}

	return errs
}

func (p *Provisioner) Provision(ctx context.Context, ui packer.Ui, comm packer.Communicator, _ map[string]interface{}) error {
	return nil
}
