package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func buildCommand() *cobra.Command {
	output := ""

	cmd := &cobra.Command{
		Use:     "build [package]",
		Short:   "Compile a plugin to a wasm module that can be uploaded",
		Args:    cobra.MaximumNArgs(1),
		Example: "  jamctl build ./examples/challenges/s3-encryption -o s3.wasm",
		RunE: func(cmd *cobra.Command, args []string) error {
			source := pluginSource(args)
			if output == "" {
				output = strings.TrimSuffix(filepath.Base(mustAbs(source)), ".go") + ".wasm"
			}
			module, err := compile(cmd.Context(), source, output)
			if err != nil {
				return err
			}
			slog.Info("plugin compiled", "module", module)
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "module path (default: package name with a .wasm suffix)")
	return cmd
}

// compile builds the plugin as a wasip1 command module: the runtime calls main
// at startup, so there is nothing to export. An output of "" writes a temporary
// file. A source that is already a module is passed straight through.
func compile(ctx context.Context, source, output string) (string, error) {
	if strings.HasSuffix(source, ".wasm") {
		return source, nil
	}
	if output == "" {
		file, err := os.CreateTemp("", "jamctl-*.wasm")
		if err != nil {
			return "", err
		}
		file.Close()
		output = file.Name()
	}

	build := exec.CommandContext(ctx, "go", "build", "-o", mustAbs(output), source)
	build.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if out, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("compile %s: %w\n%s", source, err, strings.TrimSpace(string(out)))
	}
	return output, nil
}

// pluginSource is the package to build: a directory, a go file or a module.
func pluginSource(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return "."
}

func mustAbs(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return absolute
}
