package server

import (
	"context"
	"errors"
	"testing"
	"time"
)

type blockingWorker struct{ started chan struct{} }

func (w *blockingWorker) Run(ctx context.Context) error {
	close(w.started)
	<-ctx.Done()
	return ctx.Err()
}

type failingWorker struct{ err error }

func (w failingWorker) Run(context.Context) error { return w.err }

func TestWorkerSupervisorMarksLiveWorkerReadyAndStops(t *testing.T) {
	readiness := NewReadiness()
	readiness.RequireDependencies()
	worker := &blockingWorker{started: make(chan struct{})}
	supervisor := NewWorkerSupervisor(worker, readiness)
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-worker.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	if !readiness.worker.Load() {
		t.Fatal("live worker should mark readiness")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := supervisor.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if readiness.worker.Load() {
		t.Fatal("stopped worker should clear readiness")
	}
}

func TestWorkerSupervisorClearsReadinessWhenWorkerFails(t *testing.T) {
	readiness := NewReadiness()
	supervisor := NewWorkerSupervisor(failingWorker{err: errors.New("worker failed")}, readiness)
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-supervisor.Done():
	case <-time.After(time.Second):
		t.Fatal("worker failure was not observed")
	}
	if readiness.worker.Load() {
		t.Fatal("failed worker should clear readiness")
	}
}

func TestWorkerSupervisorRequiresRunner(t *testing.T) {
	if err := NewWorkerSupervisor(nil, NewReadiness()).Start(context.Background()); err == nil {
		t.Fatal("nil runner should be rejected")
	}
}
