package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zhangzhe-ctrl/ani-model-service/internal/biz/idempotency"
	workbiz "github.com/zhangzhe-ctrl/ani-model-service/internal/biz/work"
)

var ErrLeaseLost = errors.New("import task lease lost")

type WorkStore struct{ q *Queries }

func NewWorkStore(db DBTX) *WorkStore { return &WorkStore{q: New(db)} }

func (s *WorkStore) Create(ctx context.Context, t workbiz.Task) (workbiz.Task, error) {
	tenant, id, err := parseIDs(t.TenantID, t.ID)
	if err != nil {
		return workbiz.Task{}, err
	}
	modelID := pgtype.UUID{}
	if t.ModelID != "" {
		parsed, parseErr := uuid.Parse(t.ModelID)
		if parseErr != nil {
			return workbiz.Task{}, fmt.Errorf("model_id: %w", parseErr)
		}
		modelID = uuidType(parsed)
	}
	versionID := uuid.Nil
	if t.VersionID != "" {
		versionID, err = uuid.Parse(t.VersionID)
		if err != nil {
			return workbiz.Task{}, fmt.Errorf("version_id: %w", err)
		}
	}
	fingerprint := idempotency.Fingerprint([]byte(t.TaskType + "\x00" + t.Source + "\x00" + t.RepoID + "\x00" + t.Revision + "\x00" + t.IdempotencyKey))
	existing, lookupErr := s.q.GetImportTaskByIdempotency(ctx, GetImportTaskByIdempotencyParams{TenantID: uuidType(tenant), IdempotencyKey: t.IdempotencyKey})
	if lookupErr == nil {
		if !idempotency.ReplayMatches(existing.RequestFingerprint, fingerprint) {
			return workbiz.Task{}, &pgconn.PgError{Code: "23505", Message: "idempotency key payload conflict"}
		}
		return taskFromIdempotencyRow(existing), nil
	}
	if !errors.Is(lookupErr, pgx.ErrNoRows) {
		return workbiz.Task{}, lookupErr
	}
	r, err := s.q.CreateImportTask(ctx, CreateImportTaskParams{TenantID: uuidType(tenant), ID: uuidType(id), ModelID: modelID, Column4: versionID.String(), TaskType: t.TaskType, Source: t.Source, RepoID: t.RepoID, Revision: t.Revision, IdempotencyKey: t.IdempotencyKey, RequestFingerprint: fingerprint})
	if err != nil {
		if pe, ok := err.(*pgconn.PgError); ok && pe.Code == "23505" {
			if replay, replayErr := s.q.GetImportTaskByIdempotency(ctx, GetImportTaskByIdempotencyParams{TenantID: uuidType(tenant), IdempotencyKey: t.IdempotencyKey}); replayErr == nil && idempotency.ReplayMatches(replay.RequestFingerprint, fingerprint) {
				return taskFromIdempotencyRow(replay), nil
			}
		}
		return workbiz.Task{}, err
	}
	return workbiz.Task{TenantID: t.TenantID, ID: t.ID, ModelID: t.ModelID, VersionID: t.VersionID, TaskType: r.TaskType, Source: r.Source, RepoID: r.RepoID, Revision: r.Revision, IdempotencyKey: r.IdempotencyKey, Status: r.Status, AttemptCount: int(r.AttemptCount), LeaseEpoch: r.LeaseEpoch}, nil
}

func taskFromIdempotencyRow(r GetImportTaskByIdempotencyRow) workbiz.Task {
	modelID, versionID := "", ""
	if r.ModelID.Valid {
		modelID = uuid.UUID(r.ModelID.Bytes).String()
	}
	if r.ModelVersionID.Valid {
		versionID = uuid.UUID(r.ModelVersionID.Bytes).String()
	}
	return workbiz.Task{TenantID: uuid.UUID(r.TenantID.Bytes).String(), ID: uuid.UUID(r.ID.Bytes).String(), ModelID: modelID, VersionID: versionID, TaskType: r.TaskType, Source: r.Source, RepoID: r.RepoID, Revision: r.Revision, IdempotencyKey: r.IdempotencyKey, Status: r.Status, AttemptCount: int(r.AttemptCount), LeaseEpoch: r.LeaseEpoch}
}

func (s *WorkStore) Claim(ctx context.Context, tenantID, taskID, owner string, lease time.Duration) (workbiz.Task, error) {
	tenant, id, err := parseIDs(tenantID, taskID)
	if err != nil {
		return workbiz.Task{}, err
	}
	row, err := s.q.ClaimImportTask(ctx, ClaimImportTaskParams{TenantID: uuidType(tenant), ID: uuidType(id), LeaseOwner: owner, Column4: lease.Microseconds()})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return workbiz.Task{}, ErrLeaseLost
		}
		return workbiz.Task{}, err
	}
	return workbiz.Task{
		TenantID: tenantID, ID: taskID, ModelID: nullableUUIDString(row.ModelID), VersionID: nullableUUIDString(row.ModelVersionID),
		TaskType: row.TaskType, Source: row.Source, RepoID: row.RepoID, Revision: row.Revision,
		IdempotencyKey: row.IdempotencyKey, LeaseOwner: row.LeaseOwner, LeaseEpoch: row.LeaseEpoch,
		Status: row.Status, AttemptCount: int(row.AttemptCount), LeaseUntil: row.LeaseUntil.Time,
	}, nil
}

func nullableUUIDString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func (s *WorkStore) ListDue(ctx context.Context, tenantID string, limit int32) ([]workbiz.Task, error) {
	t, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("tenant_id: %w", err)
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rs, err := s.q.ListDueImportTasks(ctx, ListDueImportTasksParams{TenantID: uuidType(t), Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]workbiz.Task, len(rs))
	for i, r := range rs {
		modelID := ""
		if r.ModelID.Valid {
			modelID = uuid.UUID(r.ModelID.Bytes).String()
		}
		versionID := ""
		if r.ModelVersionID.Valid {
			versionID = uuid.UUID(r.ModelVersionID.Bytes).String()
		}
		out[i] = workbiz.Task{TenantID: tenantID, ID: uuid.UUID(r.ID.Bytes).String(), ModelID: modelID, VersionID: versionID, TaskType: r.TaskType, Source: r.Source, RepoID: r.RepoID, Revision: r.Revision, IdempotencyKey: r.IdempotencyKey, Status: r.Status, LeaseOwner: r.LeaseOwner, LeaseEpoch: r.LeaseEpoch, AttemptCount: int(r.AttemptCount), LeaseUntil: r.LeaseUntil.Time}
	}
	return out, nil
}

func (s *WorkStore) Renew(ctx context.Context, tenantID, taskID, owner string, epoch int64, lease time.Duration) error {
	t, id, err := parseIDs(tenantID, taskID)
	if err != nil {
		return err
	}
	n, err := s.q.RenewImportTaskLease(ctx, RenewImportTaskLeaseParams{TenantID: uuidType(t), ID: uuidType(id), LeaseOwner: owner, LeaseEpoch: epoch, Column5: lease.Microseconds()})
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (s *WorkStore) Bind(ctx context.Context, tenantID, taskID, owner string, epoch int64, modelID, versionID string) error {
	tenant, id, err := parseIDs(tenantID, taskID)
	if err != nil {
		return err
	}
	mid, err := uuid.Parse(modelID)
	if err != nil {
		return fmt.Errorf("model_id: %w", err)
	}
	vid, err := uuid.Parse(versionID)
	if err != nil {
		return fmt.Errorf("model_version_id: %w", err)
	}
	n, err := s.q.BindImportTask(ctx, BindImportTaskParams{TenantID: uuidType(tenant), ID: uuidType(id), LeaseOwner: owner, LeaseEpoch: epoch, ModelID: uuidType(mid), ModelVersionID: uuidType(vid)})
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (s *WorkStore) Complete(ctx context.Context, tenantID, taskID, owner string, epoch int64) error {
	tenant, id, err := parseIDs(tenantID, taskID)
	if err != nil {
		return err
	}
	n, err := s.q.CompleteImportTask(ctx, CompleteImportTaskParams{TenantID: uuidType(tenant), ID: uuidType(id), LeaseOwner: owner, LeaseEpoch: epoch})
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (s *WorkStore) Retry(ctx context.Context, tenantID, taskID, owner string, epoch int64, after time.Duration, message string) error {
	tenant, id, err := parseIDs(tenantID, taskID)
	if err != nil {
		return err
	}
	n, err := s.q.RetryImportTask(ctx, RetryImportTaskParams{TenantID: uuidType(tenant), ID: uuidType(id), LeaseOwner: owner, LeaseEpoch: epoch, Column5: after.Microseconds(), ErrorMessage: message})
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (s *WorkStore) Fail(ctx context.Context, tenantID, taskID, owner string, epoch int64, message string) error {
	tenant, id, err := parseIDs(tenantID, taskID)
	if err != nil {
		return err
	}
	n, err := s.q.FailImportTask(ctx, FailImportTaskParams{TenantID: uuidType(tenant), ID: uuidType(id), LeaseOwner: owner, LeaseEpoch: epoch, ErrorMessage: message})
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrLeaseLost
	}
	return nil
}

func parseIDs(tenantID, taskID string) (uuid.UUID, uuid.UUID, error) {
	t, err := uuid.Parse(tenantID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("tenant_id: %w", err)
	}
	id, err := uuid.Parse(taskID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("task_id: %w", err)
	}
	return t, id, nil
}
func uuidType(v uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: v, Valid: true} }
