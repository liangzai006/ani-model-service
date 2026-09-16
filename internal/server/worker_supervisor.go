package server

import (
	"context"
	"errors"
	"sync"
)

var ErrWorkerUnavailable = errors.New("import worker is not configured")

type WorkerRunner interface {
	Run(context.Context) error
}

// WorkerSupervisor owns the process lifecycle of the durable import worker.
// Readiness is true only while the worker loop is live; a failed or stopped
// loop immediately makes dependency-gated readiness false.
type WorkerSupervisor struct {
	runner    WorkerRunner
	readiness *Readiness
	mu        sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
}

func NewWorkerSupervisor(runner WorkerRunner, readiness *Readiness) *WorkerSupervisor {
	return &WorkerSupervisor{runner: runner, readiness: readiness}
}

func (s *WorkerSupervisor) Start(parent context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runner == nil || s.readiness == nil {
		return ErrWorkerUnavailable
	}
	if s.done != nil {
		return errors.New("import worker already started")
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.done = make(chan struct{})
	done := s.done
	s.readiness.SetWorkerReady(true)
	go func() {
		defer close(done)
		_ = s.runner.Run(ctx)
		// A worker that exits, even cleanly, is no longer a live dependency.
		s.readiness.SetWorkerReady(false)
	}()
	return nil
}

func (s *WorkerSupervisor) Done() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done == nil {
		return closedChannel()
	}
	return s.done
}

func (s *WorkerSupervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel == nil || done == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		s.readiness.SetWorkerReady(false)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func closedChannel() <-chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}
