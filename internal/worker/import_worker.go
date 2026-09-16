package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	workbiz "github.com/zhangzhe-ctrl/ani-model-service/internal/biz/work"
)

var ErrVersionBindingRequired = errors.New("import task requires model version binding")

type Store interface {
	ListDue(context.Context, string, int32) ([]workbiz.Task, error)
	Claim(context.Context, string, string, string, time.Duration) (workbiz.Task, error)
	Complete(context.Context, string, string, string, int64) error
	Fail(context.Context, string, string, string, int64, string) error
	Retry(context.Context, string, string, string, int64, time.Duration, string) error
	Renew(context.Context, string, string, string, int64, time.Duration) error
	Bind(context.Context, string, string, string, int64, string, string) error
}
type Executor interface {
	Execute(context.Context, workbiz.Task) error
}

type BindingResolver interface {
	ResolveBinding(context.Context, workbiz.Task) (modelID, versionID string, err error)
}

type VersionFinalizer interface {
	MarkReady(context.Context, string, string) error
}

// Notifier wakes a running worker after a task is committed. It is only an
// optimization; the worker's PostgreSQL due scan remains authoritative.
type Notifier interface {
	Notify()
}

type Worker struct {
	Store               Store
	Executor            Executor
	Binder              BindingResolver
	Finalizer           VersionFinalizer
	TenantID, Owner     string
	PollInterval, Lease time.Duration
	Batch               int32
	MaxAttempts         int
	Logger              *slog.Logger
	wakeOnce            sync.Once
	wake                chan struct{}
}

func (w *Worker) initWake() {
	w.wakeOnce.Do(func() { w.wake = make(chan struct{}, 1) })
}

// Notify requests an immediate due scan without blocking the caller or
// creating an in-memory task queue.
func (w *Worker) Notify() {
	w.initWake()
	if w.Logger != nil {
		w.Logger.Info("import worker wake requested", slog.String("tenant_id", w.TenantID))
	}
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *Worker) Run(ctx context.Context) error {
	w.initWake()
	if w.PollInterval <= 0 {
		w.PollInterval = time.Second
	}
	if w.Lease <= 0 {
		w.Lease = time.Minute
	}
	if w.Batch <= 0 {
		w.Batch = 10
	}
	if w.MaxAttempts <= 0 {
		w.MaxAttempts = 5
	}
	tick := time.NewTicker(w.PollInterval)
	defer tick.Stop()
	for {
		if err := w.runOnce(ctx); err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		case <-w.wake:
		}
	}
}
func (w *Worker) runOnce(ctx context.Context) error {
	tasks, err := w.Store.ListDue(ctx, w.TenantID, w.Batch)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		claimed, err := w.Store.Claim(ctx, w.TenantID, t.ID, w.Owner, w.Lease)
		if err != nil {
			w.log(ctx, "import task claim failed", t, slog.String("error_class", errorClass(err)))
			continue
		}
		w.log(ctx, "import task claimed", claimed, slog.Int("attempt", claimed.AttemptCount), slog.Int64("lease_epoch", claimed.LeaseEpoch))
		// A remote task without a target version has nowhere to persist its
		// artifact. Keep it durable and pending until a trusted binding step
		// supplies model_version_id; never download and then complete blindly.
		if claimed.VersionID == "" {
			if w.Binder == nil {
				w.log(ctx, "import task binding unavailable", claimed, slog.String("error_class", errorClass(ErrVersionBindingRequired)))
				_ = w.retryOrFail(ctx, claimed, ErrVersionBindingRequired, 5*time.Minute)
				continue
			}
			modelID, versionID, bindErr := w.Binder.ResolveBinding(ctx, claimed)
			if bindErr != nil || modelID == "" || versionID == "" {
				if bindErr == nil {
					bindErr = ErrVersionBindingRequired
				}
				w.log(ctx, "import task binding failed", claimed, slog.String("error_class", errorClass(bindErr)))
				_ = w.retryOrFail(ctx, claimed, bindErr, time.Second)
				continue
			}
			if bindErr = w.Store.Bind(ctx, w.TenantID, claimed.ID, w.Owner, claimed.LeaseEpoch, modelID, versionID); bindErr != nil {
				w.log(ctx, "import task bind CAS failed", claimed, slog.String("error_class", errorClass(bindErr)))
				_ = w.retryOrFail(ctx, claimed, bindErr, time.Second)
				continue
			}
			claimed.ModelID, claimed.VersionID = modelID, versionID
			w.log(ctx, "import task bound", claimed, slog.String("model_id", modelID), slog.String("version_id", versionID))
		}
		w.log(ctx, "import execution started", claimed, slog.String("provider", claimed.Source), slog.String("repo_id", claimed.RepoID), slog.String("revision", claimed.Revision))
		execCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		renewErr := make(chan error, 1)
		go func() {
			ticker := time.NewTicker(w.Lease / 2)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if err := w.Store.Renew(ctx, w.TenantID, claimed.ID, w.Owner, claimed.LeaseEpoch, w.Lease); err != nil {
						select {
						case renewErr <- err:
						default:
						}
						cancel()
						return
					}
				case <-done:
					return
				case <-ctx.Done():
					return
				}
			}
		}()
		err = w.Executor.Execute(execCtx, claimed)
		close(done)
		cancel()
		if err == nil {
			select {
			case err = <-renewErr:
			default:
			}
		}
		if err != nil {
			w.log(ctx, "import execution failed", claimed, slog.String("error_class", errorClass(err)))
			_ = w.retryOrFail(ctx, claimed, err, time.Second)
			continue
		}
		w.log(ctx, "import execution completed", claimed)
		if w.Finalizer != nil && claimed.VersionID != "" {
			if err := w.Finalizer.MarkReady(ctx, w.TenantID, claimed.VersionID); err != nil {
				w.log(ctx, "import version ready transition failed", claimed, slog.String("error_class", errorClass(err)))
				_ = w.retryOrFail(ctx, claimed, err, time.Second)
				continue
			}
			w.log(ctx, "import version ready", claimed, slog.String("version_id", claimed.VersionID))
		}
		if err := w.Store.Complete(ctx, w.TenantID, claimed.ID, w.Owner, claimed.LeaseEpoch); err != nil {
			w.log(ctx, "import task completion failed", claimed, slog.String("error_class", errorClass(err)))
			continue
		}
		w.log(ctx, "import task completed", claimed)
	}
	return nil
}

func (w *Worker) retryOrFail(ctx context.Context, task workbiz.Task, err error, after time.Duration) error {
	if w.MaxAttempts > 0 && task.AttemptCount >= w.MaxAttempts {
		w.log(ctx, "import task failed", task, slog.String("error_class", errorClass(err)), slog.String("terminal", "true"))
		return w.Store.Fail(ctx, w.TenantID, task.ID, w.Owner, task.LeaseEpoch, err.Error())
	}
	w.log(ctx, "import task retry scheduled", task, slog.String("error_class", errorClass(err)), slog.Duration("retry_after", after))
	return w.Store.Retry(ctx, w.TenantID, task.ID, w.Owner, task.LeaseEpoch, after, err.Error())
}

func (w *Worker) log(ctx context.Context, message string, t workbiz.Task, attrs ...any) {
	if w.Logger == nil {
		return
	}
	base := []any{slog.String("tenant_id", t.TenantID), slog.String("task_id", t.ID)}
	w.Logger.InfoContext(ctx, message, append(base, attrs...)...)
}

func errorClass(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}
