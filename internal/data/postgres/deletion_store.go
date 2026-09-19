package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	modelbiz "github.com/liangzai006/ani-model-service/internal/biz/model"
)

func (s *ModelStore) DeleteModel(ctx context.Context, tenant, selector string, refs modelbiz.ReferenceChecker) error {
	return s.deleteProtected(ctx, tenant, selector, false, refs)
}
func (s *ModelStore) DeleteVersion(ctx context.Context, tenant, version string, refs modelbiz.ReferenceChecker) error {
	return s.deleteProtected(ctx, tenant, version, true, refs)
}

func (s *ModelStore) deleteProtected(ctx context.Context, tenant, selector string, versionOnly bool, refs modelbiz.ReferenceChecker) error {
	if refs == nil {
		return modelbiz.ErrReferenceCheckUnavailable
	}
	if s == nil || s.pool == nil {
		return errors.New("model store unavailable")
	}
	t, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	var selected pgtype.UUID
	if id, err := uuid.Parse(selector); err == nil {
		selected = uuidType(id)
	} else if versionOnly {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var tx pgx.Tx
	switch db := s.pool.(type) {
	case interface {
		BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
	}:
		tx, err = db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	case pgx.Tx:
		// Nested integration transactions must not hide a concurrently committed
		// version behind a repeatable-read snapshot taken before the parent lock.
		var isolation string
		if err = db.QueryRow(ctx, "SHOW transaction_isolation").Scan(&isolation); err != nil {
			return err
		}
		if isolation != "read committed" {
			return errors.New("protected deletion requires read committed isolation")
		}
		tx, err = db.Begin(ctx)
	default:
		return errors.New("model store cannot start deletion transaction")
	}
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	var modelID uuid.UUID
	var state string
	if versionOnly {
		err = tx.QueryRow(ctx, `SELECT m.id,m.status FROM public.model_versions v JOIN public.models m ON m.tenant_id=v.tenant_id AND m.id=v.model_id WHERE v.tenant_id=$1 AND v.id=$2 FOR UPDATE OF m, v`, t, selected).Scan(&modelID, &state)
	} else {
		err = tx.QueryRow(ctx, `SELECT id,status FROM public.models WHERE tenant_id=$1 AND (id=$2::uuid OR ($2::uuid IS NULL AND model_id=$3)) FOR UPDATE`, t, selected, selector).Scan(&modelID, &state)
	}
	if err != nil {
		return err
	}
	if state == "deleted" {
		return tx.Commit(ctx)
	}
	ids := []string{}
	if versionOnly {
		err = tx.QueryRow(ctx, `SELECT status FROM public.model_versions WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, t, selected).Scan(&state)
		if err != nil {
			return err
		}
		if state == "deleted" {
			return tx.Commit(ctx)
		}
		ids = append(ids, uuid.UUID(selected.Bytes).String())
	} else {
		rows, err := tx.Query(ctx, `SELECT id FROM public.model_versions WHERE tenant_id=$1 AND model_id=$2 ORDER BY id`, t, modelID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id uuid.UUID
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id.String())
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	// Inference commits its reference before the worker revalidates Model for
	// materialization. Admission also performs an earlier metadata preflight;
	// that preflight never authorizes runtime readiness. Shared row locks force
	// later worker reads to wait and recheck deletion before returning ready.
	for start := 0; start < len(ids); start += 256 {
		end := start + 256
		if end > len(ids) {
			end = len(ids)
		}
		active, err := refs.HasActiveVersionReferences(ctx, tenant, ids[start:end])
		if err != nil {
			return modelbiz.ErrReferenceCheckUnavailable
		}
		if active {
			return modelbiz.ErrModelInUse
		}
	}
	if versionOnly {
		_, err = tx.Exec(ctx, `UPDATE public.model_versions SET status='deleted',updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2`, t, selected)
	} else {
		_, err = New(tx).SoftDeleteModel(ctx, SoftDeleteModelParams{TenantID: uuidType(t), ID: uuidType(modelID)})
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
