package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"codeberg.org/megakuul/cloudjam/internal/provider/cache"
	"github.com/megakuul/dynamitedb"
	"github.com/megakuul/lake"
)

type Scheduler struct {
	rootCtx context.Context
	wg      sync.WaitGroup

	logger *slog.Logger
	oltp   *dynamitedb.Bucket
	olap   *lake.Bucket

	providerCache *cache.Cache
}

func New(rootCtx context.Context) *Scheduler {
	return &Scheduler{
		rootCtx: rootCtx,
	}
}

func (s *Scheduler) Schedule(fn func(context.Context) error, report func(context.Context, error) error) {
	s.wg.Go(func() {
		if err := fn(s.rootCtx); err != nil {
			if rErr := report(s.rootCtx, err); rErr != nil {
				s.logger.Error(fmt.Sprintf("failed to report an error (%v): %v", rErr, err))
			}
		}
	})
}

func (s *Scheduler) Wait() {
	s.wg.Wait()
}
