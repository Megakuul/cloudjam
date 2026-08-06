package build

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"codeberg.org/megakuul/cloudjam/cmd/jamctl/flags"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewCmd(options *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "build [plugin package]",
		Short:         "Compile a plugin to a wasm module that can be uploaded",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := options.Run(cmd.Context(), args); err != nil {
				slog.Error(err.Error())
				return err
			}
			return nil
		},
	}
	options.AttachFlags(cmd.Flags())

	return cmd
}

type Options struct {
	globalFlags *flags.GlobalFlags
	output      string
}

func NewOptions(gFlags *flags.GlobalFlags) *Options {
	return &Options{
		globalFlags: gFlags,
	}
}

func (r *Options) AttachFlags(flagSet *pflag.FlagSet) {
	flagSet.StringVarP(&r.output, "output", "o", "plugin.wasm", "the output path for your plugin")
}

func (r *Options) Run(ctx context.Context, args []string) error {
	build := exec.CommandContext(ctx, "go", "build", "-o", r.output, args[0])
	build.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("compile %s: %w\n%s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}
