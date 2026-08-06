package run

import (
	"codeberg.org/megakuul/cloudjam/cmd/jamctl/app/run/aws"
	"codeberg.org/megakuul/cloudjam/cmd/jamctl/flags"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewCmd(options *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "run [provider]",
		Short:         "Compile a plugin and run it against a provider",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	options.AttachFlags(cmd.Flags())

	cmd.AddCommand(
		aws.NewCmd(aws.NewOptions(options.globalFlags)),
	)

	return cmd
}

type Options struct {
	globalFlags *flags.GlobalFlags
}

func NewOptions(gFlags *flags.GlobalFlags) *Options {
	return &Options{
		globalFlags: gFlags,
	}
}

func (r *Options) AttachFlags(flagSet *pflag.FlagSet) {
}
