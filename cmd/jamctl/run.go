package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	awsprovider "codeberg.org/megakuul/cloudjam/internal/provider/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/spf13/cobra"
)

func runCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Compile a plugin and run it against a provider",
		Long: "Compile a plugin and run it against a provider.\n\n" +
			"The plugin registers itself, provisions its scenario and then loops over its\n" +
			"checks. Points are logged as they are awarded. It runs until the plugin is\n" +
			"done, until --timeout, or until you interrupt it.",
	}
	cmd.AddCommand(runAWSCommand(), runLocalCommand())
	return cmd
}

func runAWSCommand() *cobra.Command {
	var (
		account string
		profile string
		region  string
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "aws [package]",
		Short: "Run a plugin against a live aws account",
		Long: "Run a plugin against a live aws account.\n\n" +
			"With --account-id the deployment goes through the cloudjam provider, so your\n" +
			"credentials must be the organization management account. Without it the\n" +
			"plugin deploys into whatever account your credentials resolve to — your own,\n" +
			"with no sandbox around it.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			config, err := awsConfig(ctx, profile, region, "")
			if err != nil {
				return err
			}

			if account == "" {
				id, err := callerAccount(ctx, config)
				if err != nil {
					return err
				}
				slog.Warn("deploying into your own account", "account", id, "region", config.Region)
				return runPlugin(ctx, pluginSource(args), &resources{cloudcontrol.NewFromConfig(config)}, timeout)
			}

			provider, err := awsprovider.New(ctx, config)
			if err != nil {
				return fmt.Errorf("initialize provider (are these organization management credentials?): %w", err)
			}
			controller, err := provider.Resources(ctx, account, time.Hour)
			if err != nil {
				return err
			}
			slog.Info("deploying into a cloudjam member account", "account", account)
			return runPlugin(ctx, pluginSource(args), controller, timeout)
		},
	}
	cmd.Flags().StringVar(&account, "account-id", "", "member account to deploy into (needs organization management credentials)")
	cmd.Flags().StringVar(&profile, "profile", "", "shared config profile")
	cmd.Flags().StringVar(&region, "region", "", "region to deploy to (default: from the environment)")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "stop the plugin after this long (default: run until interrupted)")
	return cmd
}

func runLocalCommand() *cobra.Command {
	var (
		image   string
		port    int
		region  string
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "local [package]",
		Short: "Run a plugin against a throwaway localstack container",
		Long: "Run a plugin against a throwaway localstack container.\n\n" +
			"The container is removed when the run ends, including when you interrupt it.\n\n" +
			"Note that plugins deploy through the aws cloud control api, which localstack\n" +
			"only serves with a pro licence: without LOCALSTACK_AUTH_TOKEN every resource\n" +
			"call comes back as a 501.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			container, err := startLocalStack(ctx, image, port)
			if err != nil {
				return err
			}
			defer removeLocalStack(container)

			config, err := awsConfig(ctx, "", region, fmt.Sprintf("http://127.0.0.1:%d", port))
			if err != nil {
				return err
			}
			return runPlugin(ctx, pluginSource(args), &resources{cloudcontrol.NewFromConfig(config)}, timeout)
		},
	}
	cmd.Flags().StringVar(&image, "image", "localstack/localstack:latest", "localstack image")
	cmd.Flags().IntVar(&port, "port", 4566, "host port for the localstack edge port")
	cmd.Flags().StringVar(&region, "region", "us-east-1", "region to deploy to")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "stop the plugin after this long (default: run until interrupted)")
	return cmd
}

// startLocalStack runs the container and waits for it to answer its health
// endpoint. It returns the container id.
func startLocalStack(ctx context.Context, image string, port int) (string, error) {
	token := os.Getenv("LOCALSTACK_AUTH_TOKEN")
	if token == "" {
		slog.Warn("no LOCALSTACK_AUTH_TOKEN set: the community edition does not serve the cloud control api, so resource calls will fail")
	}

	args := []string{"run", "--detach", "--rm", "--publish", fmt.Sprintf("127.0.0.1:%d:4566", port)}
	if token != "" {
		args = append(args, "--env", "LOCALSTACK_AUTH_TOKEN="+token)
	}
	args = append(args, image)

	slog.Info("starting localstack", "image", image, "port", port)
	out, err := exec.CommandContext(ctx, "docker", args...).Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("start localstack: %w\n%s", err, strings.TrimSpace(string(exit.Stderr)))
		}
		return "", fmt.Errorf("start localstack: %w", err)
	}
	container := strings.TrimSpace(string(out))

	health := fmt.Sprintf("http://127.0.0.1:%d/_localstack/health", port)
	deadline := time.Now().Add(90 * time.Second)
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, health, nil)
		if err != nil {
			return "", err
		}
		if response, err := http.DefaultClient.Do(request); err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				slog.Info("localstack ready", "container", container[:12])
				return container, nil
			}
		}
		if time.Now().After(deadline) {
			removeLocalStack(container)
			return "", fmt.Errorf("localstack did not become ready (docker logs %s)", container[:12])
		}
		select {
		case <-ctx.Done():
			removeLocalStack(container)
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// removeLocalStack tears the container down on a fresh context: it usually runs
// because the run context was just cancelled by ctrl-c.
func removeLocalStack(container string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	slog.Info("removing localstack", "container", container[:12])
	if out, err := exec.CommandContext(ctx, "docker", "rm", "--force", container).CombinedOutput(); err != nil {
		slog.Error("failed to remove localstack", "error", strings.TrimSpace(string(out)))
	}
}
