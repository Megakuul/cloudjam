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
	Addr    string `mapstructure:"addr"`
	Json    bool   `mapstructure:"json"`
	Verbose bool   `mapstructure:"verbose"`

	AdminEmail string `mapstructure:"admin-email"`

	OLTPBucketURL       string `mapstructure:"oltp-bucket-url"`
	OLTPBucketName      string `mapstructure:"oltp-bucket-name"`
	OLTPBucketRegion    string `mapstructure:"oltp-bucket-region"`
	OLTPBucketAccessKey string `mapstructure:"oltp-bucket-access-key"`
	OLTPBucketSecretKey string `mapstructure:"oltp-bucket-secret-key"`

	OLAPCompactionInterval time.Duration `mapstructure:"olap-compaction-interval"`
	OLAPBucketURL          string        `mapstructure:"olap-bucket-url"`
	OLAPBucketName         string        `mapstructure:"olap-bucket-name"`
	OLAPBucketRegion       string        `mapstructure:"olap-bucket-region"`
	OLAPBucketAccessKey    string        `mapstructure:"olap-bucket-access-key"`
	OLAPBucketSecretKey    string        `mapstructure:"olap-bucket-secret-key"`

	TokenIssuer        string        `mapstructure:"token-issuer"`
	TokenLifetime      time.Duration `mapstructure:"token-lifetime"`
	TokenSecret        string        `mapstructure:"token-secret"`
	PolicyCacheTimeout time.Duration `mapstructure:"policy-cache-timeout"`

	ShutdownTimeout time.Duration `mapstructure:"shutdown-timeout"`

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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := viper.Unmarshal(opts); err != nil {
				return err
			}
			level := slog.LevelInfo
			if opts.Verbose {
				level = slog.LevelDebug
			}
			if opts.Json {
				slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
					AddSource: true,
					Level:     level,
				})))
			} else {
				slog.SetDefault(slog.New(tint.NewHandler(os.Stdout, &tint.Options{
					AddSource:  true,
					Level:      level,
					TimeFormat: time.Kitchen,
				})))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return Start(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolP("json", "", false, "enable json logs")
	cmd.Flags().BoolP("verbose", "", false, "enable verbose logs")
	cmd.Flags().StringP("addr", "", "0.0.0.0:8000", "location of the hornet server entrypoint")
	cmd.Flags().StringP("admin-email", "", "admin@local", "initial administrator account email")

	cmd.Flags().StringP("oltp-bucket-url", "", "http://127.0.0.1:9000", "s3 oltp-bucket url used for the database backend")
	cmd.Flags().StringP("oltp-bucket-name", "", "cloudjam-oltp", "s3 bucket name used for the database backend")
	cmd.Flags().StringP("oltp-bucket-region", "", "us-east-1", "s3 bucket region used for the database backend")
	cmd.Flags().StringP("oltp-bucket-access-key", "", "cloudjam-access-key", "s3 bucket access key used for the database backend")
	cmd.Flags().StringP("oltp-bucket-secret-key", "", "cloudjam-secret-key", "s3 bucket secret key used for the database backend")

	cmd.Flags().DurationP("olap-compaction-interval", "", time.Hour, "interval for the olap parquet compaction process")
	cmd.Flags().StringP("olap-bucket-url", "", "http://127.0.0.1:9000", "s3 olap-bucket url used for the analytics database backend")
	cmd.Flags().StringP("olap-bucket-name", "", "cloudjam-olap", "s3 bucket name used for the analytics database backend")
	cmd.Flags().StringP("olap-bucket-region", "", "us-east-1", "s3 bucket region used for the database backend")
	cmd.Flags().StringP("olap-bucket-access-key", "", "cloudjam-access-key", "s3 bucket access key used for the analytics database backend")
	cmd.Flags().StringP("olap-bucket-secret-key", "", "cloudjam-secret-key", "s3 bucket secret key used for the analytics database backend")

	cmd.Flags().StringP("token-issuer", "", "cloudjam", "issuer used inside issued jwt tokens (iss)")
	cmd.Flags().DurationP("token-lifetime", "", 24*time.Hour, "lifetime of issued jwt tokens")
	cmd.Flags().StringP("token-secret", "", rand.Text(), "secret used to HMAC sign jwt tokens")
	cmd.Flags().DurationP("policy-cache-timeout", "", time.Minute*15, "duration for policy cache (also dictates the max request duration)")
	cmd.Flags().DurationP("shutdown-timeout", "", time.Second*5, "maximum time to wait for one shutdown step")
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
