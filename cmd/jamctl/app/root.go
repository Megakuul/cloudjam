package app

import (
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"codeberg.org/megakuul/cloudjam/cmd/jamctl/app/run"
	"codeberg.org/megakuul/cloudjam/cmd/jamctl/flags"
	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	options := NewRootOptions(flags.NewGlobalFlags())
	cmd := &cobra.Command{
		Use:               "jamctl",
		Short:             "Bob Building System 🏗️ ",
		SilenceUsage:      true,
		SilenceErrors:     true,
		PersistentPreRunE: options.PreRun,
	}
	options.globalFlags.AttachFlags(cmd.PersistentFlags())

	cmd.AddCommand(
		run.NewRunCmd(run.NewRunOptions(options.globalFlags)),
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

func (r *RootOptions) PreRun(cmd *cobra.Command, args []string) error {
	slog.SetDefault(obtainLogger(r.globalFlags.Verbose, r.globalFlags.Traces, r.globalFlags.Json))

	path, err := obtainMod(r.globalFlags.Mod)
	if err != nil {
		slog.Error(err.Error())
		return err
	}
	r.globalFlags.Mod = path
	return nil
}

// obtainLogger obtains a logger based on the provided input flags.
func obtainLogger(verbose, traces, json bool) *slog.Logger {
	level := slog.LevelError
	if verbose {
		level = slog.LevelDebug
	}

	if json {
		return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			AddSource: traces,
			Level:     level,
		}))
	}
	return slog.New(tint.NewHandler(os.Stdout, &tint.Options{
		AddSource:  traces,
		Level:      level,
		TimeFormat: time.Kitchen,
	}))
}

// obtainMod obtains the bob module path for this operation based on the provided input flag.
// If no path is provided, it searches the module in the current directory structure.
func obtainMod(path string) (string, error) {
	if path != "" {
		_, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("bob module '%s' not found: %w", path, err)
		}
		return path, nil
	}

	modPath, err := searchMod()
	if err != nil {
		return "", fmt.Errorf("cannot find bob module: %w", err)
	}
	return modPath, nil
}

// searchMod performs a reverse directory traversal to find the current bob module.
func searchMod() (string, error) {
	searchPath, err := filepath.Abs(".")
	if err != nil {
		return "", fmt.Errorf("failed to read absolute directory path: %w", err)
	}
	for {
		entries, err := os.ReadDir(searchPath)
		if err != nil {
			return "", fmt.Errorf("failed to reverse traverse directory: %w", err)
		}

		for _, entry := range entries {
			if entry.Name() == modcfg.MOD_FILE_NAME {
				return path.Join(searchPath, entry.Name()), nil
			}
		}

		if len(strings.Split(searchPath, string(filepath.Separator))) <= 2 {
			return "", fmt.Errorf("not inside a bob module...")
		}
		searchPath = filepath.Dir(searchPath)
	}
}
