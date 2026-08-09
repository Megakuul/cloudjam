package cache

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"codeberg.org/megakuul/cloudjam/internal/provider"
	"codeberg.org/megakuul/cloudjam/internal/provider/aws"
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

func (c *Cache) Load(ctx context.Context, providerID string) (provider.Provider, error) {
	c.providersLock.Lock()
	defer c.providersLock.Unlock()

	provider, ok := c.providers[providerID]
	if !ok {
		providerData, err := dynamitedb.Get(ctx, c.oltp, &oltp.Provider{
			ProviderID: dynamitedb.Key(providerID),
		})
		if err != nil {
			return nil, err
		}
		provider, err = aws.New(ctx, providerData.Credentials.Value(),
			aws.WithEmailSuffix(fmt.Sprintf("+%s", providerData.Email)),
			aws.WithRegions(providerData.Regions.Value()...),
			aws.WithLogger(slog.With("system", fmt.Sprintf("provider-%s", providerData.Name.Value()))),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize provider-%s: %w", providerData.Name.Value(), err)
		}
		c.providers[providerID] = provider
	}
	return provider, nil
}
