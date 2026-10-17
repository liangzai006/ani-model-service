package worker

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	workbiz "github.com/liangzai006/ani-model-service/internal/biz/work"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

type workerStoreFake struct {
	task                              workbiz.Task
	completed, retried, failed, bound bool
	failedMessage                     string
	boundModel, boundVersion          string
	renewErr                          error
}

type pollErrorStore struct {
	workerStoreFake
	err error
}

func (s *pollErrorStore) ListDue(context.Context, string, int32) ([]workbiz.Task, error) {
	return nil, s.err
}

type allTenantStoreFake struct {
	workerStoreFake
	claimTenant string
}

func (s *allTenantStoreFake) ListDueAll(context.Context, int32) ([]workbiz.Task, error) {
	return []workbiz.Task{s.task}, nil
}

func (s *allTenantStoreFake) Claim(ctx context.Context, tenantID, taskID, owner string, lease time.Duration) (workbiz.Task, error) {
	s.claimTenant = tenantID
	return s.workerStoreFake.Claim(ctx, tenantID, taskID, owner, lease)
}

func (s *workerStoreFake) ListDue(context.Context, string, int32) ([]workbiz.Task, error) {
	return []workbiz.Task{s.task}, nil
}
func (s *workerStoreFake) Claim(context.Context, string, string, string, time.Duration) (workbiz.Task, error) {
	s.task.Status, s.task.LeaseOwner, s.task.LeaseEpoch, s.task.LeaseUntil = workbiz.Importing, "worker", s.task.LeaseEpoch+1, time.Now().Add(time.Minute)
	s.task.AttemptCount++
	return s.task, nil
}
func (s *workerStoreFake) Complete(context.Context, string, string, string, int64) error {
	s.completed = true
	return nil
}
func (s *workerStoreFake) Retry(context.Context, string, string, string, int64, time.Duration, string) error {
	s.retried = true
	return nil
}
func (s *workerStoreFake) Renew(context.Context, string, string, string, int64, time.Duration) error {
	return s.renewErr
}
func (s *workerStoreFake) Bind(_ context.Context, _, _, _ string, _ int64, modelID, versionID string) error {
	s.bound, s.boundModel, s.boundVersion = true, modelID, versionID
	s.task.ModelID, s.task.VersionID = modelID, versionID
	return nil
}
func (s *workerStoreFake) Fail(_ context.Context, _, _, _ string, _ int64, message string) error {
	s.failed = true
	s.failedMessage = message
	return nil
}

type workerExecutorFake struct{ err error }

func (e workerExecutorFake) Execute(context.Context, workbiz.Task) error { return e.err }

type recordingExecutor struct{ called bool }

func (e *recordingExecutor) Execute(context.Context, workbiz.Task) error { e.called = true; return nil }

type wakeScanStore struct {
	calls  chan int
	listed int
	task   workbiz.Task
}

func (s *wakeScanStore) ListDue(context.Context, string, int32) ([]workbiz.Task, error) {
	s.listed++
	s.calls <- s.listed
	if s.listed == 2 {
		return []workbiz.Task{s.task}, nil
	}
	return nil, nil
}
func (s *wakeScanStore) Claim(_ context.Context, _ string, _ string, owner string, lease time.Duration) (workbiz.Task, error) {
	s.task.Status, s.task.LeaseOwner, s.task.LeaseEpoch, s.task.LeaseUntil = workbiz.Importing, owner, s.task.LeaseEpoch+1, time.Now().Add(lease)
	s.task.AttemptCount++
	return s.task, nil
}
func (s *wakeScanStore) Complete(context.Context, string, string, string, int64) error { return nil }
func (s *wakeScanStore) Fail(context.Context, string, string, string, int64, string) error {
	return nil
}
func (s *wakeScanStore) Retry(context.Context, string, string, string, int64, time.Duration, string) error {
	return nil
}
func (s *wakeScanStore) Renew(context.Context, string, string, string, int64, time.Duration) error {
	return nil
}
func (s *wakeScanStore) Bind(context.Context, string, string, string, int64, string, string) error {
	return nil
}

type notifierFake struct{ calls int }

func (n *notifierFake) Notify() { n.calls++ }

type bindingFake struct{ err error }

func (b bindingFake) ResolveBinding(context.Context, workbiz.Task) (string, string, error) {
	return "model-uuid", "version-uuid", b.err
}

type finalizerFake struct {
	err          error
	errorErr     error
	tenant       string
	version      string
	errorTenant  string
	errorVersion string
	errorMessage string
}

func (f *finalizerFake) MarkReady(_ context.Context, tenant, version string) error {
	f.tenant, f.version = tenant, version
	return f.err
}

func (f *finalizerFake) MarkErrorAndMaybeModel(_ context.Context, tenant, version, message string) error {
	f.errorTenant, f.errorVersion, f.errorMessage = tenant, version, message
	return f.errorErr
}

func TestWorkerFinalizesVersionBeforeCompletingTask(t *testing.T) {
	store := &workerStoreFake{task: workbiz.Task{TenantID: "tenant", ID: "task", VersionID: "version", Status: workbiz.Pending}}
	finalizer := &finalizerFake{}
	w := &Worker{Store: store, Executor: workerExecutorFake{}, Finalizer: finalizer, TenantID: "tenant", Owner: "worker", Lease: time.Minute}
	if err := w.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if finalizer.version != "version" || !store.completed || store.retried {
		t.Fatalf("finalize/complete order failed: version=%q completed=%v retried=%v", finalizer.version, store.completed, store.retried)
	}
}

func TestWorkerUsesTaskTenantWhenScopeIsEmpty(t *testing.T) {
	store := &allTenantStoreFake{workerStoreFake: workerStoreFake{task: workbiz.Task{TenantID: "tenant-from-task", ID: "task", VersionID: "version", Status: workbiz.Pending}}}
	finalizer := &finalizerFake{}
	w := &Worker{Store: store, Executor: workerExecutorFake{}, Finalizer: finalizer, Owner: "worker", Lease: time.Minute}
	if err := w.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.claimTenant != "tenant-from-task" {
		t.Fatalf("claim tenant = %q, want tenant-from-task", store.claimTenant)
	}
	if finalizer.tenant != "tenant-from-task" {
		t.Fatalf("finalizer tenant = %q, want tenant-from-task", finalizer.tenant)
	}
}

func TestWorkerRetriesWhenVersionFinalizationFails(t *testing.T) {
	store := &workerStoreFake{task: workbiz.Task{TenantID: "tenant", ID: "task", VersionID: "version", Status: workbiz.Pending}}
	finalizer := &finalizerFake{err: errors.New("checksum mismatch")}
	w := &Worker{Store: store, Executor: workerExecutorFake{}, Finalizer: finalizer, TenantID: "tenant", Owner: "worker", Lease: time.Minute}
	if err := w.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !store.retried || store.completed {
		t.Fatalf("failed finalization was not retried: retried=%v completed=%v", store.retried, store.completed)
	}
}

func TestWorkerDefersUnboundTaskWithoutExecuting(t *testing.T) {
	store := &workerStoreFake{task: workbiz.Task{TenantID: "tenant", ID: "task", Status: workbiz.Pending}}
	executor := &recordingExecutor{}
	w := &Worker{Store: store, Executor: executor, TenantID: "tenant", Owner: "worker", Lease: time.Minute}
	if err := w.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !store.retried || store.completed || executor.called {
		t.Fatalf("unbound task was executed/completed: retried=%v completed=%v called=%v", store.retried, store.completed, executor.called)
	}
}

func TestWorkerBindsUnboundTaskBeforeExecuting(t *testing.T) {
	store := &workerStoreFake{task: workbiz.Task{TenantID: "tenant", ID: "task", Status: workbiz.Pending}}
	executor := &recordingExecutor{}
	w := &Worker{Store: store, Binder: bindingFake{}, Executor: executor, TenantID: "tenant", Owner: "worker", Lease: time.Minute}
	if err := w.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !store.bound || store.boundModel != "model-uuid" || store.boundVersion != "version-uuid" || !executor.called || !store.completed {
		t.Fatalf("binding flow failed: bound=%v model=%s version=%s executed=%v completed=%v", store.bound, store.boundModel, store.boundVersion, executor.called, store.completed)
	}
}

func TestWorkerFailsTaskAfterMaxAttempts(t *testing.T) {
	store := &workerStoreFake{task: workbiz.Task{TenantID: "tenant", ID: "task", VersionID: "version", Status: workbiz.Pending, AttemptCount: 0}}
	finalizer := &finalizerFake{}
	w := &Worker{Store: store, Executor: workerExecutorFake{err: errors.New("checksum mismatch")}, Finalizer: finalizer, TenantID: "tenant", Owner: "worker", Lease: time.Minute, MaxAttempts: 1}
	if err := w.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !store.failed || store.retried || store.completed {
		t.Fatalf("terminal failure not recorded: failed=%v retried=%v completed=%v", store.failed, store.retried, store.completed)
	}
	if finalizer.errorTenant != "tenant" || finalizer.errorVersion != "version" || finalizer.errorMessage != "checksum mismatch" {
		t.Fatalf("terminal version error not recorded: tenant=%q version=%q message=%q", finalizer.errorTenant, finalizer.errorVersion, finalizer.errorMessage)
	}
	if store.failedMessage != "checksum mismatch" {
		t.Fatalf("task error changed after version error transition: %q", store.failedMessage)
	}
}

func TestWorkerPreservesTaskErrorWhenVersionErrorTransitionFails(t *testing.T) {
	store := &workerStoreFake{task: workbiz.Task{TenantID: "tenant", ID: "task", VersionID: "version", Status: workbiz.Pending}}
	finalizer := &finalizerFake{errorErr: errors.New("version CAS lost")}
	w := &Worker{Store: store, Executor: workerExecutorFake{err: errors.New("provider unavailable")}, Finalizer: finalizer, TenantID: "tenant", Owner: "worker", Lease: time.Minute, MaxAttempts: 1}
	if err := w.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !store.failed || store.failedMessage != "provider unavailable" {
		t.Fatalf("task failure was not preserved: failed=%v message=%q", store.failed, store.failedMessage)
	}
}

func TestWorkerCancelsExecutionWhenLeaseRenewalFails(t *testing.T) {
	store := &workerStoreFake{task: workbiz.Task{TenantID: "tenant", ID: "task", VersionID: "version", Status: workbiz.Pending}, renewErr: errors.New("lease lost")}
	executor := &blockingExecutor{}
	w := &Worker{Store: store, Executor: executor, TenantID: "tenant", Owner: "worker", Lease: 10 * time.Millisecond, PollInterval: time.Hour}
	if err := w.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !executor.cancelled || !store.retried || store.completed {
		t.Fatalf("lease loss was not fenced: cancelled=%v retried=%v completed=%v", executor.cancelled, store.retried, store.completed)
	}
}

func TestWorkerNotifyWakesDueScanImmediately(t *testing.T) {
	store := &wakeScanStore{calls: make(chan int, 4), task: workbiz.Task{TenantID: "tenant", ID: "task", VersionID: "version", Status: workbiz.Pending}}
	w := &Worker{Store: store, Executor: workerExecutorFake{}, TenantID: "tenant", Owner: "worker", Lease: time.Minute, PollInterval: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { _ = w.Run(ctx); close(done) }()
	select {
	case <-store.calls:
	case <-time.After(time.Second):
		t.Fatal("initial due scan did not run")
	}
	w.Notify()
	select {
	case call := <-store.calls:
		if call != 2 {
			t.Fatalf("scan count=%d, want immediate second scan", call)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("notify did not wake due scan")
	}
	cancel()
	<-done
}

func TestWorkerReportsHealthAfterDueScan(t *testing.T) {
	store := &workerStoreFake{task: workbiz.Task{TenantID: "tenant", ID: "task", VersionID: "version", Status: workbiz.Pending}}
	w := &Worker{Store: store, Executor: workerExecutorFake{}, TenantID: "tenant", Owner: "worker", Lease: time.Minute, PollInterval: time.Hour}
	health := make(chan bool, 4)
	w.SetHealthCallback(func(ready bool) { health <- ready })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { _ = w.Run(ctx); close(done) }()
	select {
	case ready := <-health:
		if ready {
			t.Fatal("worker should report unready before its first due scan")
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not report initial health")
	}
	select {
	case ready := <-health:
		if !ready {
			t.Fatal("successful due scan should report worker ready")
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not report successful scan")
	}
	cancel()
	<-done
}

func TestWorkerLogsDueScanFailure(t *testing.T) {
	var logs lockedBuffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	w := &Worker{
		Logger: logger, Store: &pollErrorStore{err: errors.New("database unavailable")},
		TenantID: "tenant", Owner: "worker", PollInterval: time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Run(ctx); close(done) }()
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(logs.String(), "import worker due scan failed") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if !strings.Contains(logs.String(), "import worker due scan failed") {
		t.Fatalf("due scan failure was not logged: %s", logs.String())
	}
}

func TestWorkerLogsTaskLifecycle(t *testing.T) {
	var logs lockedBuffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	store := &workerStoreFake{task: workbiz.Task{TenantID: "tenant", ID: "task-id", VersionID: "version", Source: "huggingface", Status: workbiz.Pending}}
	w := &Worker{Logger: logger, Store: store, Executor: workerExecutorFake{}, Finalizer: &finalizerFake{}, TenantID: "tenant", Owner: "worker", Lease: time.Minute}
	if err := w.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := logs.String()
	for _, want := range []string{"import task claimed", "import execution started", "import execution completed", "import task completed", "task-id"} {
		if !strings.Contains(got, want) {
			t.Fatalf("log output %q does not contain %q", got, want)
		}
	}
}

type blockingExecutor struct{ cancelled bool }

func (e *blockingExecutor) Execute(ctx context.Context, _ workbiz.Task) error {
	<-ctx.Done()
	e.cancelled = true
	return ctx.Err()
}
