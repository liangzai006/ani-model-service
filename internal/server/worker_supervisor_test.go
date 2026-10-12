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

type healthAwareWorker struct {
	started chan struct{}
	allow   chan struct{}
	health  func(bool)
}

func (w *healthAwareWorker) SetHealthCallback(callback func(bool)) { w.health = callback }

func (w *healthAwareWorker) Run(ctx context.Context) error {
	close(w.started)
	select {
	case <-w.allow:
	case <-ctx.Done():
		return ctx.Err()
	}
	w.health(true)
	<-ctx.Done()
	return ctx.Err()
}

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

func TestWorkerSupervisorWaitsForHealthAwareWorker(t *testing.T) {
	readiness := NewReadiness()
	readiness.RequireDependencies()
	worker := &healthAwareWorker{started: make(chan struct{}), allow: make(chan struct{})}
	supervisor := NewWorkerSupervisor(worker, readiness)
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-worker.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	if readiness.worker.Load() {
		t.Fatal("health-aware worker must not be ready before its first healthy scan")
	}
	close(worker.allow)
	deadline := time.After(time.Second)
	for !readiness.worker.Load() {
		select {
		case <-deadline:
			t.Fatal("worker health was not reported")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := supervisor.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}
