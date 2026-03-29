package main

import (
	"context"
	"crypto/rand"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Options struct {
	Addr string `mapstructure:"addr"`
	Json bool   `mapstructure:"json"`

	DatabaseSource     string        `mapstructure:"database-source"`
	DatabaseName       string        `mapstructure:"database-name"`
	DatabaseCollection string        `mapstructure:"database-collection"`
	TokenIssuer        string        `mapstructure:"token-issuer"`
	TokenSecret        string        `mapstructure:"token-secret"`
	PolicyCacheTimeout time.Duration `mapstructure:"policy-cache-timeout"`

	Dev        bool   `mapstructure:"dev"`
	DevWebAddr string `mapstructure:"dev-web-addr"`
}

func main() {
	opts := &Options{}
	cmd := cobra.Command{
		Short:         "hornet 💥",
		Long:          "Hornet is a single binary launcher for CloudJam 💥",
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
			if err := viper.Unmarshal(opts); err != nil {
				return err
			}
			return Start(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolP("json", "", false, "enable json logs")
	cmd.Flags().StringP("addr", "", "0.0.0.0:9000", "location of the hornet server entrypoint")
	cmd.Flags().StringP("database-source", "", "mongodb://./documentdb.s.PGSQL.10260", "mongodb source connection string")
	cmd.Flags().StringP("database-name", "", "cloudjam", "name of the mongodb database")
	cmd.Flags().StringP("database-collection", "", "table", "name of the mongodb collection (only needs one)")
	cmd.Flags().StringP("token-issuer", "", "cloudjam", "issuer used inside issued jwt tokens (iss)")
	cmd.Flags().StringP("token-secret", "", rand.Text(), "secret used to HMAC sign jwt tokens")
	cmd.Flags().DurationP("policy-cache-timeout", "", time.Minute*15, "duration for policy cache (also dictates the max request duration)")
	cmd.Flags().BoolP("dev", "D", false, "enable dev mode (proxies web requests to live watcher)")
	cmd.Flags().StringP("dev-web-addr", "", "127.0.0.1:5173", "location of the live web server in dev mode")

	viper.BindPFlags(cmd.Flags())
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := cmd.ExecuteContext(ctx); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
