package concurrency

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"
)

func RunGroup(ctx context.Context, timeout time.Duration, fns ...func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)
	for _, fn := range fns {
		fn := fn
		g.Go(func() error {
			return fn(ctx)
		})
	}
	return g.Wait()
}
