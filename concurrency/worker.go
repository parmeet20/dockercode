package concurrency

import (
	"context"
	"sync"
)

type Pool struct {
	sem    chan struct{}
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

func NewPool(ctx context.Context, maxConcurrent int) *Pool {
	ctx2, cancel := context.WithCancel(ctx)
	p := &Pool{
		sem:    make(chan struct{}, maxConcurrent),
		ctx:    ctx2,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	return p
}
func (p *Pool) Submit(fn func(ctx context.Context) error) error {
	select {
	case p.sem <- struct{}{}:
	case <-p.ctx.Done():
		return p.ctx.Err()
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer func() { <-p.sem }()

		_ = fn(p.ctx)
	}()
	return nil
}
func (p *Pool) Wait() {
	p.wg.Wait()
}
func (p *Pool) Stop() {
	p.cancel()
	p.wg.Wait()
	select {
	case <-p.done:
	default:
		close(p.done)
	}
}
