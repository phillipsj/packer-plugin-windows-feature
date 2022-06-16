//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package feature

import (
	"bytes"
	"context"
	_ "embed" // this is needed for using the go:embed directive
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/hashicorp/packer-plugin-sdk/retry"
	"github.com/hashicorp/packer-plugin-sdk/uuid"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

const (
	elevatedPath                 = "C:/Windows/Temp/packer-windows-feature-elevated.ps1"
	elevatedCommand              = "PowerShell -ExecutionPolicy Bypass -OutputFormat Text -File C:/Windows/Temp/packer-windows-feature-elevated.ps1"
	windowsFeaturePath           = "C:/Windows/Temp/packer-windows-feature.ps1"
	pendingRebootElevatedPath    = "C:/Windows/Temp/packer-windows-feature-pending-reboot-elevated.ps1"
	pendingRebootElevatedCommand = "PowerShell -ExecutionPolicy Bypass -OutputFormat Text -File C:/Windows/Temp/packer-windows-feature-pending-reboot-elevated.ps1"
	restartCommand               = "shutdown.exe -f -r -t 0 -c \"packer restart\""
	testRestartCommand           = "shutdown.exe -f -r -t 60 -c \"packer restart test\""
	abortTestRestartCommand      = "shutdown.exe -a"
	retryableDelay               = 5 * time.Second
	uploadTimeout                = 5 * time.Minute
)

//go:embed windows-feature.ps1
var windowsFeaturePs1 []byte

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
	Capabilities []string `mapstructure:"capabilities"`

	ctx interpolate.Context
}

type Provisioner struct {
	config Config
}

func (p *Provisioner) ConfigSpec() hcldec.ObjectSpec {
	return p.config.FlatMapstructure().HCL2Spec()
}

func (p *Provisioner) Prepare(raws ...interface{}) error {
	if err := config.Decode(&p.config, &config.DecodeOpts{
		Interpolate:        true,
		InterpolateContext: &p.config.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{
				"execute_command",
			},
		},
	}, raws...); err != nil {
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
	ui.Say("Uploading the Windows feature elevated script...")
	var buffer bytes.Buffer
	if err := elevatedTemplate().Execute(&buffer, elevatedOptions{
		Username:        p.config.Username,
		Password:        p.config.Password,
		TaskDescription: "Packer Windows update elevated task",
		TaskName:        fmt.Sprintf("packer-windows-feature-%s", uuid.TimeOrderedUUID()),
		Command:         p.windowsFeatureCommand(),
	}); err != nil {
		fmt.Printf("Error creating elevated template: %s", err)
		return err
	}

	err := retry.Config{StartTimeout: uploadTimeout}.Run(ctx, func(context.Context) error {
		if err := comm.Upload(
			elevatedPath,
			bytes.NewReader(buffer.Bytes()),
			nil); err != nil {
			return fmt.Errorf("error uploading the Windows feature elevated script: %s", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	ui.Say("Uploading the Windows feature check for reboot required elevated script...")
	buffer.Reset()
	if err := elevatedTemplate().Execute(&buffer, elevatedOptions{
		Username:        p.config.Username,
		Password:        p.config.Password,
		TaskDescription: "Packer Windows feature pending reboot elevated task",
		TaskName:        fmt.Sprintf("packer-windows-feature-pending-reboot-%s", uuid.TimeOrderedUUID()),
		Command:         p.windowsFeatureCheckForRebootRequiredCommand(),
	}); err != nil {
		fmt.Printf("Error creating elevated template: %s", err)
		return err
	}

	err = retry.Config{StartTimeout: uploadTimeout}.Run(ctx, func(context.Context) error {
		if err := comm.Upload(
			pendingRebootElevatedPath,
			bytes.NewReader(buffer.Bytes()),
			nil); err != nil {
			return fmt.Errorf("error uploading the Windows feature check for reboot required elevated script: %s", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	ui.Say("Uploading the Windows update script...")
	if err := comm.Upload(
		windowsFeaturePath,
		bytes.NewReader(windowsFeaturePs1),
		nil); err != nil {
		return err
	}

	for {
		restartPending, err := p.install(ctx, ui, comm)
		if err != nil {
			return err
		}

		if !restartPending {
			return nil
		}

		if err := p.restart(ctx, ui, comm); err != nil {
			return err
		}
	}
}

func (p *Provisioner) install(ctx context.Context, ui packer.Ui, comm packer.Communicator) (bool, error) {
	ui.Say("Running Windows Feature and Capability install...")
	var restartPending bool
	err := retry.Config{
		RetryDelay: func() time.Duration { return retryableDelay },
		Tries:      5,
	}.Run(ctx, func(ctx context.Context) error {
		cmd := &packer.RemoteCmd{Command: elevatedCommand}
		if err := cmd.RunWithUi(ctx, comm, ui); err != nil {
			return err
		}
		var exitStatus = cmd.ExitStatus()
		switch exitStatus {
		case 0:
			return nil
		case 101:
			restartPending = true
			return nil
		default:
			return fmt.Errorf("windows feature script exited with non-zero exit status: %d", exitStatus)
		}
	})
	return restartPending, err
}

func (p *Provisioner) restart(ctx context.Context, ui packer.Ui, comm packer.Communicator) error {
	restartPending := true
	for restartPending {
		ui.Say("Restarting the machine...")
		err := p.retryable(ctx, func(ctx context.Context) error {
			cmd := &packer.RemoteCmd{Command: restartCommand}
			if err := cmd.RunWithUi(ctx, comm, ui); err != nil {
				return err
			}
			exitStatus := cmd.ExitStatus()
			if exitStatus != 0 {
				return fmt.Errorf("failed to restart the machine with exit status: %d", exitStatus)
			}
			return nil
		})
		if err != nil {
			return err
		}

		ui.Say("Waiting for machine to become available...")
		err = p.retryable(ctx, func(ctx context.Context) error {
			// wait for the machine to reboot.
			cmd := &packer.RemoteCmd{Command: testRestartCommand}
			if err := cmd.RunWithUi(ctx, comm, ui); err != nil {
				return err
			}
			exitStatus := cmd.ExitStatus()
			if exitStatus != 0 {
				return fmt.Errorf("machine not yet available (exit status %d)", exitStatus)
			}
			cmd = &packer.RemoteCmd{Command: abortTestRestartCommand}
			return cmd.RunWithUi(ctx, comm, ui)
		})
		if err != nil {
			return err
		}

		ui.Say("Checking for pending restart...")
		err = p.retryable(ctx, func(ctx context.Context) error {
			cmd := &packer.RemoteCmd{Command: pendingRebootElevatedCommand}
			if err := cmd.RunWithUi(ctx, comm, ui); err != nil {
				return err
			}

			exitStatus := cmd.ExitStatus()
			switch {
			case exitStatus == 0:
				restartPending = false
			case exitStatus == 101:
				restartPending = true
			default:
				return fmt.Errorf("machine not yet available (exit status %d)", exitStatus)
			}

			return nil
		})
		if err != nil {
			return err
		}

		if restartPending {
			ui.Say("Restart is still pending...")
		} else {
			ui.Say("Restart complete")
		}
	}

	return nil
}

// retryable will retry the given function over and over until a
// non-error is returned, RestartTimeout expires, or ctx is
// cancelled.
func (p *Provisioner) retryable(ctx context.Context, f func(ctx context.Context) error) error {
	return retry.Config{
		RetryDelay:   func() time.Duration { return retryableDelay },
		StartTimeout: p.config.RestartTimeout,
	}.Run(ctx, f)
}

func (p *Provisioner) windowsFeatureCommand() string {
	return fmt.Sprintf(
		"PowerShell -ExecutionPolicy Bypass -OutputFormat Text -EncodedCommand %s",
		base64.StdEncoding.EncodeToString(
			encodeUtf16Le(fmt.Sprintf(
				"%s%s%s",
				windowsFeaturePath,
				featuresArgument(p.config.Features),
				capabilitiesArgument(p.config.Capabilities)))))
}

func (p *Provisioner) windowsFeatureCheckForRebootRequiredCommand() string {
	return fmt.Sprintf(
		"PowerShell -ExecutionPolicy Bypass -OutputFormat Text -EncodedCommand %s",
		base64.StdEncoding.EncodeToString(
			encodeUtf16Le(fmt.Sprintf(
				"%s -OnlyCheckForRebootRequired",
				windowsFeaturePath))))
}

func encodeUtf16Le(s string) []byte {
	d := utf16.Encode([]rune(s))
	b := make([]byte, len(d)*2)
	for i, r := range d {
		b[i*2] = byte(r)
		b[i*2+1] = byte(r >> 8)
	}
	return b
}

func featuresArgument(features []string) string {
	if features == nil {
		return ""
	}

	var buffer bytes.Buffer
	buffer.WriteString(" -Features ")
	for i, feature := range features {
		if i > 0 {
			buffer.WriteString(",")
		}
		buffer.WriteString(escapePowerShellString(feature))
	}

	return buffer.String()
}

func capabilitiesArgument(capabilities []string) string {
	if capabilities == nil {
		return ""
	}

	var buffer bytes.Buffer
	buffer.WriteString(" -Capabilities ")
	for i, capability := range capabilities {
		if i > 0 {
			buffer.WriteString(",")
		}
		buffer.WriteString(escapePowerShellString(capability))
	}

	return buffer.String()
}

func escapePowerShellString(value string) string {
	return fmt.Sprintf(
		"'%s'",
		strings.Replace(value, "'", "''", -1))
}
