package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	auditbiz "github.com/liangzai006/ani-model-service/internal/biz/audit"
)

type AuditStore struct{ q *Queries }

func NewAuditStore(db DBTX) *AuditStore { return &AuditStore{q: New(db)} }
func (s *AuditStore) Insert(ctx context.Context, e auditbiz.Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	tenant, err := uuid.Parse(e.TenantID)
	if err != nil {
		return fmt.Errorf("tenant_id: %w", err)
	}
	task := uuid.Nil
	if e.TaskID != "" {
		task, err = uuid.Parse(e.TaskID)
		if err != nil {
			return fmt.Errorf("task_id: %w", err)
		}
	}
	return s.q.InsertAuditEvent(ctx, InsertAuditEventParams{TenantID: uuidType(tenant), ID: uuidType(uuid.New()), Actor: e.Actor, Workload: e.Workload, RequestID: e.RequestID, Column6: task.String(), Action: e.Action, BeforeState: e.BeforeState, AfterState: e.AfterState, ErrorClass: e.ErrorClass})
}
