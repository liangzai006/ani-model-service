package service

import (
	"context"
	"testing"
	"time"

	kratoserrors "github.com/go-kratos/kratos/v3/errors"
	modelv1 "github.com/liangzai006/ani-model-service/api/model/v1"
	workbiz "github.com/liangzai006/ani-model-service/internal/biz/work"
	"github.com/liangzai006/ani-model-service/internal/identity"
)

type importTaskReaderFake struct {
	task workbiz.Task
	err  error
}

func (f importTaskReaderFake) Get(context.Context, string, string) (workbiz.Task, error) {
	return f.task, f.err
}

type importTaskRetrierFake struct {
	task       workbiz.Task
	err        error
	tenant, id string
}

func (f *importTaskRetrierFake) RetryFailed(_ context.Context, tenant, id string) (workbiz.Task, error) {
	f.tenant, f.id = tenant, id
	return f.task, f.err
}

func TestGetImportTaskReturnsProgressAndError(t *testing.T) {
	created := time.Date(2026, 9, 18, 1, 2, 3, 0, time.UTC)
	s := NewModelImportService(nil)
	s.SetImportTaskReader(importTaskReaderFake{task: workbiz.Task{
		TenantID: "tenant-a", ID: "11111111-1111-1111-1111-111111111111", Source: "huggingface", Status: workbiz.Failed,
		AttemptCount: 3, ProgressPct: 42, ErrorMessage: "checksum mismatch", CreatedAt: created,
	}})
	ctx := identity.WithPrincipal(context.Background(), identity.Principal{TenantID: "tenant-a", Actor: "actor", Workload: "gateway"})
	got, err := s.GetImportTask(ctx, &modelv1.GetImportTaskRequest{TenantId: "tenant-a", TaskId: "11111111-1111-1111-1111-111111111111"})
	if err != nil {
		t.Fatal(err)
	}
	task := got.GetTask()
	if task.GetStatus() != workbiz.Failed || task.GetProgressPct() != 42 || task.GetErrorMessage() != "checksum mismatch" || task.GetAttemptCount() != 3 || task.GetCreatedAt().AsTime() != created {
		t.Fatalf("task response = %+v", task)
	}
}

func TestRetryImportTaskResetsFailedTask(t *testing.T) {
	retrier := &importTaskRetrierFake{task: workbiz.Task{TenantID: "tenant-a", ID: "11111111-1111-1111-1111-111111111111", Status: workbiz.Pending, AttemptCount: 0}}
	s := NewModelImportService(nil)
	s.SetImportTaskRetrier(retrier)
	ctx := identity.WithPrincipal(context.Background(), identity.Principal{TenantID: "tenant-a", Actor: "actor", Workload: "gateway"})
	got, err := s.RetryImportTask(ctx, &modelv1.RetryImportTaskRequest{TenantId: "tenant-a", TaskId: "11111111-1111-1111-1111-111111111111"})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetTask().GetStatus() != workbiz.Pending || got.GetTask().GetAttemptCount() != 0 || retrier.tenant != "tenant-a" || retrier.id != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("retry response=%+v tenant=%q id=%q", got, retrier.tenant, retrier.id)
	}
}

func TestRetryImportTaskRequiresTrustedTenant(t *testing.T) {
	s := NewModelImportService(nil)
	_, err := s.RetryImportTask(context.Background(), &modelv1.RetryImportTaskRequest{TenantId: "tenant-a", TaskId: "task-a"})
	if kratoserrors.Code(err) != 401 {
		t.Fatalf("code=%v err=%v", kratoserrors.Code(err), err)
	}
}
