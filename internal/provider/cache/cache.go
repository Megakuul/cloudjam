package cache

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"codeberg.org/megakuul/cloudjam/internal/provider"
	"codeberg.org/megakuul/cloudjam/internal/provider/aws"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud"
	"github.com/megakuul/dynamitedb"
)

type Cache struct {
	oltp          *dynamitedb.Bucket
	providersLock sync.Mutex
	providers     map[string]provider.Provider
}

func New(oltp *dynamitedb.Bucket) *Cache {
	return &Cache{
		oltp:      oltp,
		providers: map[string]provider.Provider{},
	}
}

// Bust removes the specified provider from the cache.
func (c *Cache) Bust(providerMeta *oltp.Provider) {
	c.providersLock.Lock()
	defer c.providersLock.Unlock()

	delete(c.providers, providerMeta.ProviderID.Value())
}

// Load loads teh specified provider from cache. If not there, it initializes and caches the provider.
func (c *Cache) Load(ctx context.Context, providerMeta *oltp.Provider) (provider.Provider, error) {
	var err error
	c.providersLock.Lock()
	defer c.providersLock.Unlock()

	provider, ok := c.providers[providerMeta.ProviderID.Value()]
	if !ok {
		switch providerMeta.Type.Value() {
		case cloud.ProviderType_AWS:
			provider, err = aws.New(ctx, providerMeta.Credentials.Value(),
				aws.WithEmailSuffix(fmt.Sprintf("+%s", providerMeta.Email)),
				aws.WithRegions(providerMeta.Regions.Value()...),
				aws.WithLogger(slog.With("system", fmt.Sprintf("provider-%s", providerMeta.Name.Value()))),
			)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize provider-%s: %w", providerMeta.Name.Value(), err)
			}
		}
		c.providers[providerMeta.ProviderID.Value()] = provider
	}
	return provider, nil
}
