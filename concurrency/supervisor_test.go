package concurrency

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSupervisor_Go(t *testing.T) {
	s := NewSupervisor()
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(1)

	s.Go(ctx, "test_job", func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
	})
	time.Sleep(10 * time.Millisecond)
	active := s.ActiveCount()
	if active["test_job"] != 1 {
		t.Errorf("Expected 1 active 'test_job', got %v", active)
	}

	wg.Wait()
	time.Sleep(10 * time.Millisecond)

	active = s.ActiveCount()
	if len(active) != 0 {
		t.Errorf("Expected 0 active jobs after completion, got %v", active)
	}
}

func TestSupervisor_Go_CancelledContext(t *testing.T) {
	s := NewSupervisor()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	executed := false
	s.Go(ctx, "cancelled_job", func() {
		executed = true
	})

	time.Sleep(20 * time.Millisecond)
	if executed {
		t.Error("Job should not have executed with cancelled context")
	}

	active := s.ActiveCount()
	if len(active) != 0 {
		t.Errorf("Expected no active jobs for cancelled context, got %v", active)
	}
}
