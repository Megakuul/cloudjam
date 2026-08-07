package aws

import (
	"context"
	"fmt"
	"log/slog"

	"codeberg.org/megakuul/cloudjam/cmd/jamctl/flags"
	"codeberg.org/megakuul/cloudjam/cmd/jamctl/plugin"
	"codeberg.org/megakuul/cloudjam/internal/provider/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewCmd(options *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "aws [plugin package]",
		Short:         "Compile an AWS plugin and run it against the provider",
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
	fake        bool
	fakeImage   string
	fakePort    int
	region      string
}

func NewOptions(gFlags *flags.GlobalFlags) *Options {
	return &Options{
		globalFlags: gFlags,
	}
}

func (r *Options) AttachFlags(flagSet *pflag.FlagSet) {
	flagSet.BoolVarP(&r.fake, "fake", "F", true, "run against fakecloud on docker (might not support all APIs / retrieval opts)")
	flagSet.StringVar(&r.fakeImage, "image", "ghcr.io/faiscadev/fakecloud:latest", "fakecloud docker image (only used if --fake is true)")
	flagSet.IntVar(&r.fakePort, "port", 4566, "host port for the fakecloud endpoint (only used if --fake is true)")
	flagSet.StringVar(&r.region, "region", "us-east-1", "region to deploy to")
}

func (r *Options) Run(ctx context.Context, args []string) error {
	if r.fake {
		container, err := startFakeCloud(ctx, r.fakeImage, r.fakePort)
		if err != nil {
			return err
		}
		defer removeFakeCloud(container)

		config, err := config.LoadDefaultConfig(ctx,
			config.WithRegion(r.region),
			config.WithBaseEndpoint(fmt.Sprintf("http://127.0.0.1:%d", r.fakePort)),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
		)
		if err != nil {
			return fmt.Errorf("load aws configuration: %w", err)
		}
		slog.Info(fmt.Sprintf(
			"starting fakecloud at 'http://127.0.0.1:%d' with credentials 'AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test'",
			r.fakePort,
		))
		s3Client, assetBucket := s3.NewFromConfig(config), fmt.Sprintf("asset-bucket-%s", uuid.NewString())
		_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: &assetBucket,
		})
		if err != nil {
			return fmt.Errorf("failed to create asset bucket: %w", err)
		}
		return plugin.Run(ctx, args[0],
			aws.NewAssetController(s3Client, assetBucket),
			aws.NewResourceController(cloudcontrol.NewFromConfig(config)),
		)
	}
	return nil
}
