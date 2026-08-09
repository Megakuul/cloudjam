package app

import (
	"log/slog"
	"os"
	"time"

	"codeberg.org/megakuul/cloudjam/cmd/jamctl/app/build"
	"codeberg.org/megakuul/cloudjam/cmd/jamctl/app/run"
	"codeberg.org/megakuul/cloudjam/cmd/jamctl/flags"
	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
)

func NewCmd() *cobra.Command {
	options := NewRootOptions(flags.NewGlobalFlags())
	cmd := &cobra.Command{
		Use:           "jamctl",
		Short:         "CloudJam CLI for designing challenges",
		SilenceUsage:  true,
		SilenceErrors: false,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			level := slog.LevelInfo
			if options.globalFlags.Verbose {
				level = slog.LevelDebug
			}
			slog.SetDefault(slog.New(tint.NewHandler(os.Stdout, &tint.Options{
				Level:      level,
				TimeFormat: time.Kitchen,
			})))
		},
	}
	options.globalFlags.AttachFlags(cmd.PersistentFlags())

	cmd.AddCommand(
		run.NewCmd(run.NewOptions(options.globalFlags)),
		build.NewCmd(build.NewOptions(options.globalFlags)),
	)

	return cmd
}

type RootOptions struct {
	globalFlags *flags.GlobalFlags
}

func NewRootOptions(gFlags *flags.GlobalFlags) *RootOptions {
	return &RootOptions{
		globalFlags: gFlags,
	}
}
