package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"strings"

	awsprovider "codeberg.org/megakuul/cloudjam/internal/provider/aws"
	"github.com/spf13/cobra"
)

func nukeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "nuke",
		Short: "Erase everything in an account",
		Long: "Erase everything in an account.\n\n" +
			"This is the provider's nuke, the same one hornet runs when it hands a sandbox\n" +
			"back to the pool: every resource cloud-nuke can reach, in every region given.\n" +
			"It is not limited to what a challenge created, and there is no undo.",
	}
	cmd.AddCommand(nukeAWSCommand(), nukeLocalCommand())
	return cmd
}

func nukeAWSCommand() *cobra.Command {
	var (
		account string
		profile string
		region  string
		regions []string
		yes     bool
	)

	cmd := &cobra.Command{
		Use:   "aws",
		Short: "Erase everything in an aws account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			config, err := awsConfig(ctx, profile, region, "")
			if err != nil {
				return err
			}
			if len(regions) == 0 {
				regions = []string{config.Region}
			}

			if account != "" {
				provider, err := awsprovider.New(ctx, config)
				if err != nil {
					return fmt.Errorf("initialize provider (are these organization management credentials?): %w", err)
				}
				if err := confirm(cmd, account, yes,
					"About to erase EVERY resource in cloudjam member account %s.", account); err != nil {
					return err
				}
				slog.Warn("nuking account", "account", account)
				return provider.Nuke(ctx, account)
			}

			id, err := callerAccount(ctx, config)
			if err != nil {
				return err
			}
			if err := confirm(cmd, id, yes,
				"About to erase EVERY resource in your own aws account %s (regions: %s).",
				id, strings.Join(regions, ", ")); err != nil {
				return err
			}
			slog.Warn("nuking account", "account", id)
			return nukeAccount(ctx, config, id, regions)
		},
	}
	cmd.Flags().StringVar(&account, "account-id", "", "member account to erase (needs organization management credentials)")
	cmd.Flags().StringVar(&profile, "profile", "", "shared config profile")
	cmd.Flags().StringVar(&region, "region", "", "region to sweep (default: from the environment)")
	cmd.Flags().StringSliceVar(&regions, "regions", nil, "regions to sweep (default: just --region)")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the confirmation")
	return cmd
}

func nukeLocalCommand() *cobra.Command {
	var (
		port   int
		region string
		yes    bool
	)

	cmd := &cobra.Command{
		Use:   "local",
		Short: "Erase everything in a running localstack",
		Long: "Erase everything in a running localstack.\n\n" +
			"`jamctl run local` removes its container on the way out, so this is only for a\n" +
			"stack you started yourself.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			endpoint := fmt.Sprintf("http://127.0.0.1:%d", port)

			config, err := awsConfig(ctx, "", region, endpoint)
			if err != nil {
				return err
			}
			if err := confirm(cmd, "localstack", yes,
				"About to erase every resource in the localstack at %s.", endpoint); err != nil {
				return err
			}
			slog.Warn("nuking localstack", "endpoint", endpoint)
			return nukeAccount(ctx, config, "000000000000", []string{region})
		},
	}
	cmd.Flags().IntVar(&port, "port", 4566, "port the localstack edge port is published on")
	cmd.Flags().StringVar(&region, "region", "us-east-1", "region to sweep")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the confirmation")
	return cmd
}

// confirm blocks until the account id is typed back. Typing the id rather than
// "y" is the point: the whole risk here is running against the wrong account.
func confirm(cmd *cobra.Command, phrase string, yes bool, warning string, args ...any) error {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "\n%s\nThis cannot be undone.\n\n", fmt.Sprintf(warning, args...))
	if yes {
		return nil
	}

	fmt.Fprintf(out, "Type %q to continue: ", phrase)
	answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if strings.TrimSpace(answer) != phrase {
		return fmt.Errorf("aborted")
	}
	fmt.Fprintln(out)
	return nil
}
