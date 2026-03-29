package main

import (
	"context"
	"crypto/rand"
	"log/slog"
	"os"
	"os/signal"

	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
)

type Options struct {
	Addr string
	Json bool

	Database    string
	TokenIssuer string
	TokenSecret string

	Dev        bool
	DevWebAddr string
}

func main() {
	opts := &Options{}
	cmd := cobra.Command{
		Short:         "hornet 💥",
		Long:          "Hornet is a single binary bootstrapper for CloudJam",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if opts.Json {
				slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
					AddSource: true,
					Level:     slog.LevelDebug,
				})))
			} else {
				slog.SetDefault(slog.New(tint.NewHandler(os.Stdout, &tint.Options{
					AddSource: true,
					Level:     slog.LevelDebug,
				})))
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return Start(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.Json, "json", "", false, "enable json logs")

	cmd.Flags().StringVarP(&opts.Addr, "addr", "", "0.0.0.0:9000", "location of the hornet server entrypoint")
	cmd.Flags().StringVarP(&opts.Database, "database", "", "mongodb://./documentdb.s.PGSQL.10260/cloudjam", "mongodb source string to the cloudjam collection")
	cmd.Flags().StringVarP(&opts.TokenIssuer, "token-issuer", "", "cloudjam", "issuer used inside issued jwt tokens (iss)")
	cmd.Flags().StringVarP(&opts.TokenSecret, "token-secret", "", rand.Text(), "secret used to HMAC sign jwt tokens")

	cmd.Flags().BoolVarP(&opts.Dev, "dev", "D", false, "enable dev mode (proxies web requests to live watcher)")
	cmd.Flags().StringVarP(&opts.DevWebAddr, "dev-web-addr", "", "127.0.0.1:5173", "location of the live web server in dev mode")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := cmd.ExecuteContext(ctx); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
