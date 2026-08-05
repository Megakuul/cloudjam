// Command jamctl runs challenge plugins without a cloudjam server.
//
// It compiles a plugin to wasm, implements the plugin api itself and deploys
// through a real provider. Everything the server would store — the challenge,
// the player, the score — is logged instead.
//
//	jamctl build ./examples/challenges/s3-encryption
//	jamctl run local ./examples/challenges/s3-encryption
//	jamctl run aws ./examples/challenges/s3-encryption
//	jamctl nuke aws
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
)

func main() {
	verbose := false

	cmd := &cobra.Command{
		Use:           "jamctl",
		Short:         "Run cloudjam challenge plugins locally 🎮",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			level := slog.LevelInfo
			if verbose {
				level = slog.LevelDebug
			}
			slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{
				Level:      level,
				TimeFormat: time.TimeOnly,
			})))
		},
	}
	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logs")
	cmd.AddCommand(buildCommand(), runCommand(), nukeCommand())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := cmd.ExecuteContext(ctx); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
