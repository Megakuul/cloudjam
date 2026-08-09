package challenge

import (
	"context"

	"golang.org/x/sync/errgroup"
)

type Runner struct {
	runCtx context.Context
	group  *errgroup.Group
}

func NewRunner(rootCtx context.Context) *Runner {
	group, ctx := errgroup.WithContext(rootCtx)
	return &Runner{
		runCtx: ctx,
		group:  group,
	}
}

func (r *Runner) Launch(ctx context.Context, challenge *Challenge) {
	r.group.Go(func() error {
		return challenge.Start(r.runCtx)
	})
}

func (r *Runner) Wait() error {
	return r.group.Wait()
}
