package challenge

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	extism "github.com/extism/go-sdk"
	"github.com/tetratelabs/wazero"
)

// Cache provides a compilation cache for challenge plugins.
// Challenge binaries can be up to 50 MB if just 20 people start a challenge we are on 1 GB memory usage for the whole challenge duration.
// This cache uses teh challenge hash as key and ensures everyone on this server instance uses the same compiled instruction pages.
type Cache struct {
	logger *slog.Logger

	challengesLock sync.Mutex
	challenges     map[string]*extism.CompiledPlugin
	lastUsage      map[string]time.Time

	uncacheTimeout time.Duration
}

func NewCache(uncacheTimeout time.Duration) *Cache {
	return &Cache{
		challenges:     map[string]*extism.CompiledPlugin{},
		lastUsage:      map[string]time.Time{},
		uncacheTimeout: uncacheTimeout,
	}
}

// clean removes old plugins from the cache to avoid leaking memory.
func (c *Cache) clean(ctx context.Context) {
	c.challengesLock.Lock()
	defer c.challengesLock.Unlock()
	for hash, lastUsage := range c.lastUsage {
		if time.Now().After(lastUsage.Add(c.uncacheTimeout)) {
			plugin, ok := c.challenges[hash]
			if ok {
				if err := plugin.Close(ctx); err != nil {
					c.logger.Error(fmt.Sprintf("failed to properly close plugin %q: %v", hash, err))
				}
				delete(c.challenges, hash)
			}
			delete(c.lastUsage, hash)
		}
	}
}

func (c *Cache) Load(ctx context.Context, hash string, binaryFactory func(ctx context.Context) (extism.Wasm, error), hostFunctions ...extism.HostFunction) (*extism.Plugin, error) {
	c.clean(ctx)

	c.challengesLock.Lock()
	defer c.challengesLock.Unlock()
	plugin, ok := c.challenges[hash]
	if !ok {
		pluginData, err := binaryFactory(ctx)
		if err != nil {
			return nil, err
		}
		plugin, err = extism.NewCompiledPlugin(ctx,
			extism.Manifest{Wasm: []extism.Wasm{pluginData}},
			extism.PluginConfig{
				EnableWasi: true,
				// Without this a cancelled context cannot interrupt the guest, and a
				// challenge loop would keep running after ctrl-c.
				RuntimeConfig: wazero.NewRuntimeConfig().WithCloseOnContextDone(true),
			},
			hostFunctions,
		)
		if err != nil {
			return nil, err
		}
		c.challenges[hash] = plugin
	}
	instance, err := plugin.Instance(ctx, extism.PluginInstanceConfig{
		ModuleConfig: wazero.NewModuleConfig().
			WithSysWalltime().
			WithSysNanotime().
			WithSysNanosleep().
			WithRandSource(rand.Reader).
			WithStdout(os.Stdout).
			WithStderr(os.Stderr),
	})
	if err != nil {
		return nil, fmt.Errorf("instantiate plugin: %w", err)
	}
	return instance, nil
}

func (c *Cache) Close(ctx context.Context) (err error) {
	c.challengesLock.Lock()
	defer c.challengesLock.Unlock()

	for hash, challenge := range c.challenges {
		err = errors.Join(err, challenge.Close(ctx))
		delete(c.challenges, hash)
	}
	c.lastUsage = map[string]time.Time{}
	return err
}
