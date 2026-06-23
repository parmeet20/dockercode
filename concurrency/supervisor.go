package concurrency

import (
	"context"
	"sync"
)

type Supervisor struct {
	mu     sync.Mutex
	active map[string]int
}

func NewSupervisor() *Supervisor {
	return &Supervisor{
		active: make(map[string]int),
	}
}
func (s *Supervisor) Go(ctx context.Context, name string, fn func()) {
	s.mu.Lock()
	s.active[name]++
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.active[name]--
			if s.active[name] == 0 {
				delete(s.active, name)
			}
			s.mu.Unlock()
		}()
		select {
		case <-ctx.Done():
			return
		default:
			fn()
		}
	}()
}
func (s *Supervisor) ActiveCount() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make(map[string]int, len(s.active))
	for k, v := range s.active {
		out[k] = v
	}
	return out
}
