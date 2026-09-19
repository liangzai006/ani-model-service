package postgres

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	modelbiz "github.com/liangzai006/ani-model-service/internal/biz/model"
)

type versionReferencesFunc func(context.Context, string, []string) (bool, error)

func (f versionReferencesFunc) HasActiveVersionReferences(ctx context.Context, tenant string, ids []string) (bool, error) {
	return f(ctx, tenant, ids)
}

func TestDeletionSerializesReadyReadAndVersionCreationPostgres(t *testing.T) {
	t.Run("whole model", func(t *testing.T) { testDeletionConcurrency(t, false) })
	t.Run("single version", func(t *testing.T) { testDeletionConcurrency(t, true) })
}

func testDeletionConcurrency(t *testing.T, versionOnly bool) {
	dsn := os.Getenv("MODEL_DATABASE_URL")
	if dsn == "" {
		t.Skip("MODEL_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tenant, mid, vid := uuid.NewString(), uuid.NewString(), uuid.NewString()
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, q := range []string{`DELETE FROM model_artifacts WHERE tenant_id=$1`, `DELETE FROM model_versions WHERE tenant_id=$1`, `DELETE FROM models WHERE tenant_id=$1`} {
			if _, err := pool.Exec(cleanup, q, tenant); err != nil {
				t.Error(err)
			}
		}
	})
	if _, err = pool.Exec(ctx, `INSERT INTO models(tenant_id,id,model_id,name,source,status) VALUES($1,$2,'concurrent-delete','concurrent-delete','upload','ready')`, tenant, mid); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO model_versions(tenant_id,id,model_id,version,format,status,checksum_sha256) VALUES($1,$2,$3,'v1','gguf','ready',repeat('a',64))`, tenant, vid, mid); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO model_artifacts(tenant_id,id,model_version_id,provider,reference,format,size_bytes,sha256) VALUES($1,$2,$3,'upload','fixture/model.tar','gguf',1,repeat('a',64))`, tenant, uuid.NewString(), vid); err != nil {
		t.Fatal(err)
	}
	locked, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	deleted := make(chan error, 1)
	go func() {
		delete := NewModelStore(pool).DeleteModel
		selector := mid
		if versionOnly {
			delete = NewModelStore(pool).DeleteVersion
			selector = vid
		}
		deleted <- delete(ctx, tenant, selector, versionReferencesFunc(func(ctx context.Context, _ string, _ []string) (bool, error) {
			close(locked)
			select {
			case <-release:
				return false, nil
			case <-ctx.Done():
				return false, ctx.Err()
			}
		}))
	}()
	select {
	case <-locked:
	case err := <-deleted:
		t.Fatalf("delete never reached guard: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	reader, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close(context.Background())
	creator, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer creator.Close(context.Background())
	var readerPID, creatorPID int32
	if err = reader.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&readerPID); err != nil {
		t.Fatal(err)
	}
	if err = creator.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&creatorPID); err != nil {
		t.Fatal(err)
	}
	readResult, createResult := make(chan error, 1), make(chan error, 1)
	go func() { _, err := NewVersionStore(reader).GetVersion(ctx, tenant, vid); readResult <- err }()
	go func() {
		_, err := NewVersionStore(creator).CreateVersion(ctx, modelbiz.Version{TenantID: tenant, ID: uuid.NewString(), ModelID: mid, Version: "v2", Format: "gguf"})
		createResult <- err
	}()
	for _, pid := range []int32{readerPID, creatorPID} {
		deadline := time.Now().Add(3 * time.Second)
		for {
			var blocked bool
			if err = pool.QueryRow(ctx, "SELECT cardinality(pg_blocking_pids($1))>0", pid).Scan(&blocked); err != nil {
				t.Fatal(err)
			}
			if blocked {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("PID %d did not wait for deletion lock", pid)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	unblock()
	if err = <-deleted; err != nil {
		t.Fatal(err)
	}
	if err = <-readResult; !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("read escaped deletion: %v", err)
	}
	if err = <-createResult; (!versionOnly && !errors.Is(err, pgx.ErrNoRows)) || (versionOnly && err != nil) {
		t.Fatalf("create escaped deletion: %v", err)
	}
}

func TestProtectedDeletionPostgres(t *testing.T) {
	dsn := os.Getenv("MODEL_DATABASE_URL")
	if dsn == "" {
		t.Skip("MODEL_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	tenant, mid, vid := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err = tx.Exec(ctx, `INSERT INTO models(tenant_id,id,model_id,name,source,status) VALUES($1,$2,'delete-fixture','delete-fixture','upload','ready')`, tenant, mid); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO model_versions(tenant_id,id,model_id,version,format,status) VALUES($1,$2,$3,'v1','gguf','pending')`, tenant, vid, mid); err != nil {
		t.Fatal(err)
	}
	store := NewModelStore(tx)
	for _, deleteVersion := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			refs := versionReferencesFunc(func(_ context.Context, got string, ids []string) (bool, error) {
				if got != tenant || len(ids) != 1 || ids[0] != vid {
					t.Fatalf("wrong references tenant=%s ids=%v", got, ids)
				}
				if fail {
					return false, errors.New("provider unavailable")
				}
				return true, nil
			})
			var err error
			if deleteVersion {
				err = store.DeleteVersion(ctx, tenant, vid, refs)
			} else {
				err = store.DeleteModel(ctx, tenant, "delete-fixture", refs)
			}
			want := modelbiz.ErrModelInUse
			if fail {
				want = modelbiz.ErrReferenceCheckUnavailable
			}
			if !errors.Is(err, want) {
				t.Fatalf("deleteVersion=%v failure=%v got=%v want=%v", deleteVersion, fail, err, want)
			}
		}
	}
	refs := versionReferencesFunc(func(context.Context, string, []string) (bool, error) { return false, nil })
	if err = store.DeleteVersion(ctx, uuid.NewString(), vid, refs); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("other tenant deletion=%v", err)
	}
	if err = store.DeleteVersion(ctx, tenant, vid, refs); err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteVersion(ctx, tenant, vid, refs); err != nil {
		t.Fatalf("version retry=%v", err)
	}
	var state string
	if err = tx.QueryRow(ctx, `SELECT status FROM model_versions WHERE tenant_id=$1 AND id=$2`, tenant, vid).Scan(&state); err != nil || state != "deleted" {
		t.Fatalf("version state=%s err=%v", state, err)
	}
	if err = store.DeleteModel(ctx, tenant, mid, refs); err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteModel(ctx, tenant, "delete-fixture", refs); err != nil {
		t.Fatalf("model retry=%v", err)
	}
	if err = tx.QueryRow(ctx, `SELECT status FROM models WHERE tenant_id=$1 AND id=$2`, tenant, mid).Scan(&state); err != nil || state != "deleted" {
		t.Fatalf("model state=%s err=%v", state, err)
	}
	_, err = NewVersionStore(tx).CreateVersion(ctx, modelbiz.Version{TenantID: tenant, ID: uuid.NewString(), ModelID: mid, Version: "v2", Format: "gguf"})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("created version under deleted model: %v", err)
	}
}
