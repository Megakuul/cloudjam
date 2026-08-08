package plugin

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"codeberg.org/megakuul/cloudjam/internal/challenge"
	"codeberg.org/megakuul/cloudjam/internal/provider"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
	extism "github.com/extism/go-sdk"
	"github.com/tetratelabs/wazero"
)

// Run compiles the plugin from the source package, installs the plugin api as host functions and calls _start.
// It uses the provided resource controller to implement the host functions.
func Run(ctx context.Context, source string, access provider.AccessController, assets provider.AssetController, resources provider.ResourceController) error {
	slog.Info("compiling plugin", "source", source)

	file, err := os.CreateTemp("", "jamctl-*.wasm")
	if err != nil {
		return err
	}
	file.Close()
	output := file.Name()

	build := exec.CommandContext(ctx, "go", "build", "-o", output, source)
	build.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("compile %s: %w\n%s", source, err, strings.TrimSpace(string(out)))
	}

	provider := &localProvider{access: access, assets: assets, resources: resources}

	report := func(err error) {
		slog.Error(err.Error())
	}

	plugin, err := extism.NewCompiledPlugin(ctx,
		extism.Manifest{Wasm: []extism.Wasm{extism.WasmFile{Path: output}}},
		extism.PluginConfig{
			EnableWasi: true,
			// Without this a cancelled context cannot interrupt the guest, and a
			// challenge loop would keep running after ctrl-c.
			RuntimeConfig: wazero.NewRuntimeConfig().WithCloseOnContextDone(true),
		},
		[]extism.HostFunction{
			challenge.RegisterInOutHost(api.ReportName, provider.report, report),
			challenge.RegisterInOutHost(api.LogName, provider.log, report),
			challenge.RegisterInOutHost(api.CreateMetaName, provider.createMeta, report),
			challenge.RegisterInOutHost(api.UpdateMetaName, provider.updateMeta, report),
			challenge.RegisterInOutHost(api.ReadScoreName, provider.readScore, report),
			challenge.RegisterInOutHost(api.UpdateScoreName, provider.updateScore, report),
			challenge.RegisterOutHost(api.CreateAssetName, provider.createAsset, report),
			challenge.RegisterInOutHost(api.UpdateAssetName, provider.updateAsset, report),
			challenge.RegisterInOutHost(api.CreatePermissionName, provider.createPermission, report),
			challenge.RegisterInOutHost(api.UpdatePermissionName, provider.updatePermission, report),
			challenge.RegisterInOutHost(api.CreateGuardrailName, provider.createGuardrail, report),
			challenge.RegisterInOutHost(api.UpdateGuardrailName, provider.updateGuardrail, report),
			challenge.RegisterInOutHost(api.CreateResourceName, provider.createResource, report),
			challenge.RegisterInOutHost(api.ReadResourceName, provider.readResource, report),
			challenge.RegisterInOutHost(api.UpdateResourceName, provider.updateResource, report),
			challenge.RegisterInOutHost(api.DeleteResourceName, provider.deleteResource, report),
			challenge.RegisterInOutHost(api.ListResourceName, provider.listResource, report),
		},
	)
	if err != nil {
		return fmt.Errorf("load plugin: %w", err)
	}
	defer plugin.Close(context.WithoutCancel(ctx))

	instance, err := plugin.Instance(ctx, extism.PluginInstanceConfig{
		// The defaults are a frozen clock and a sleep that returns immediately,
		// which would collapse every challenge interval into one hot spin.
		ModuleConfig: wazero.NewModuleConfig().
			WithSysWalltime().
			WithSysNanotime().
			WithSysNanosleep().
			WithRandSource(rand.Reader).
			WithStdout(os.Stdout).
			WithStderr(os.Stderr),
	})
	if err != nil {
		return fmt.Errorf("instantiate plugin: %w", err)
	}
	defer instance.Close(context.WithoutCancel(ctx))

	_, _, err = instance.CallWithContext(ctx, "_start", nil)
	slog.Info("plugin finished", "score", provider.score)
	if err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}
