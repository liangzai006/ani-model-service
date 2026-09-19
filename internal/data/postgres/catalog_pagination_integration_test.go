package postgres_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	kratoserrors "github.com/go-kratos/kratos/v3/errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	modelv1 "github.com/liangzai006/ani-model-service/api/model/v1"
	"github.com/liangzai006/ani-model-service/internal/data/postgres"
	"github.com/liangzai006/ani-model-service/internal/identity"
	"github.com/liangzai006/ani-model-service/internal/service"
)

// Test fixtures are isolated by fresh tenants and rolled back, including on
// failure. No deployed model or inference state is changed by this test.
func TestCatalogPaginationPostgres(t *testing.T) {
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
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	tenant, other := uuid.NewString(), uuid.NewString()
	created := time.Date(2026, 9, 17, 0, 0, 0, 123456000, time.UTC)
	id := func(n int) string { return fmt.Sprintf("11111111-1111-4111-8111-%012d", n) }
	for _, entry := range []struct {
		tenant                        string
		n                             int
		source, status, caps, display string
	}{
		{tenant, 1, "huggingface", "ready", `["chat"]`, "Smol a"},
		{tenant, 2, "huggingface", "ready", `["chat","embedding"]`, "Smol b"},
		{tenant, 3, "huggingface", "ready", `["chat"]`, "Smol 100%"},
		{tenant, 4, "upload", "ready", `["chat"]`, "Smol upload"},
		{tenant, 5, "huggingface", "error", `["chat"]`, "Smol failed"},
		{tenant, 6, "huggingface", "ready", `["embedding"]`, "Smol embedding"},
		{tenant, 7, "huggingface", "deleted", `["chat"]`, "Smol deleted"},
		{other, 3, "huggingface", "ready", `["chat"]`, "Smol foreign"},
	} {
		exec(`INSERT INTO public.models (tenant_id,id,model_id,name,display_name,source,source_repo_id,status,capabilities,total_size_bytes,error_message,created_at,updated_at)
		 VALUES ($1,$2,$3,$3,$4,$5,'org/smol',$6,$7,123,'fixture error',$8,$8)`, entry.tenant, id(entry.n), fmt.Sprintf("smol-%d", entry.n), entry.display, entry.source, entry.status, entry.caps, created)
	}
	for n, state := range []string{"ready", "pending", "deleted"} {
		exec(`INSERT INTO public.model_versions (tenant_id,id,model_id,version,format,status,size_bytes,checksum_sha256,is_encrypted,encrypt_algo,encrypt_hint,created_at,updated_at)
		 VALUES ($1,$2,$3,$4,'safetensors',$5,123,$6,true,'AES-256','key-a',$7,$7)`, tenant, id(10+n), id(3), fmt.Sprintf("v%d", n+1), state, strings.Repeat("a", 64), created)
	}
	exec(`INSERT INTO public.model_artifacts (tenant_id,id,model_version_id,provider,reference,format,size_bytes,sha256)
	 VALUES ($1,$2,$3,'s3','tenant/model/v1/model.tar','safetensors',123,$4)`, tenant, uuid.NewString(), id(10), strings.Repeat("a", 64))
	catalog := postgres.NewCatalogStore(postgres.NewModelStore(tx))
	versions := postgres.NewVersionStore(tx)
	server := service.NewModelServiceWithDependencies(versions, versions, catalog, nil, nil)
	principal := func(tenant string) context.Context {
		return identity.WithPrincipal(ctx, identity.Principal{TenantID: tenant, Actor: "integration-test", Workload: "test"})
	}
	request := &modelv1.ListModelsRequest{Status: "ready", Source: "huggingface", Capability: "chat", Keyword: "SMOL", Page: &modelv1.CursorPageRequest{Limit: 2}}
	first, err := server.ListModels(principal(tenant), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Models) != 2 || first.Models[0].Id != id(3) || first.Models[1].Id != id(2) || !first.GetMeta().GetHasMore() {
		t.Fatalf("first page: %v", first)
	}
	m := first.Models[0]
	if m.SourceRepoId != "org/smol" || m.TotalSizeBytes != 123 || m.ErrorMessage != "fixture error" || !m.CreatedAt.AsTime().Equal(created) || !m.UpdatedAt.AsTime().Equal(created) || len(m.Versions) != 1 || m.Versions[0].Version != "v2" {
		t.Fatalf("metadata/latest version: %v", m)
	}
	request.Page.Cursor = first.Meta.NextCursor
	// A newly inserted item ahead of the cursor must not shift or repeat page 2.
	exec(`INSERT INTO public.models (tenant_id,id,model_id,name,source,status,capabilities,created_at) VALUES ($1,$2,'smol-new','smol-new','huggingface','ready','["chat"]',$3)`, tenant, id(99), created.Add(time.Second))
	last, err := server.ListModels(principal(tenant), request)
	if err != nil || len(last.GetModels()) != 1 || last.Models[0].Id != id(1) || last.GetMeta().GetHasMore() || last.GetMeta().GetNextCursor() != "" {
		t.Fatalf("last page: %v err=%v", last, err)
	}
	_, err = server.ListModels(principal(other), request)
	if kratoserrors.Code(err) != 400 {
		t.Fatalf("foreign cursor accepted: %v", err)
	}
	for _, tc := range []struct {
		name string
		req  *modelv1.ListModelsRequest
		ids  []string
	}{
		{"literal percent", &modelv1.ListModelsRequest{Keyword: "%"}, []string{id(3)}},
		{"source", &modelv1.ListModelsRequest{Source: "upload"}, []string{id(4)}},
		{"capability", &modelv1.ListModelsRequest{Capability: "embedding"}, []string{id(6), id(2)}},
		{"status", &modelv1.ListModelsRequest{Status: "error"}, []string{id(5)}},
		{"no matches", &modelv1.ListModelsRequest{Keyword: "missing"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := server.ListModels(principal(tenant), tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Models) != len(tc.ids) {
				t.Fatalf("results=%v", got)
			}
			for i, want := range tc.ids {
				if got.Models[i].Id != want {
					t.Fatalf("results=%v", got)
				}
			}
			if got.Meta == nil || got.Meta.HasMore || got.Meta.NextCursor != "" {
				t.Fatalf("unexpected page: %v", got.Meta)
			}
		})
	}
	foreign, err := server.ListModels(principal(other), &modelv1.ListModelsRequest{})
	if err != nil || len(foreign.GetModels()) != 1 || foreign.Models[0].DisplayName != "Smol foreign" || len(foreign.Models[0].Versions) != 0 {
		t.Fatalf("tenant leaked: %v err=%v", foreign, err)
	}
	versionRequest := &modelv1.ListModelVersionsRequest{ModelId: "smol-3", Page: &modelv1.CursorPageRequest{Limit: 1}}
	page, err := server.ListModelVersions(principal(tenant), versionRequest)
	if err != nil || len(page.GetVersions()) != 1 || page.Versions[0].Version != "v2" || !page.GetMeta().GetHasMore() {
		t.Fatalf("versions page=%v err=%v", page, err)
	}
	versionRequest.Page.Cursor = page.Meta.NextCursor
	versionRequest.ModelId = id(3) // external and internal model selectors share a scope.
	page, err = server.ListModelVersions(principal(tenant), versionRequest)
	if err != nil || len(page.GetVersions()) != 1 || page.Versions[0].Version != "v1" || page.GetMeta().GetHasMore() || page.GetMeta().GetNextCursor() != "" {
		t.Fatalf("versions last=%v err=%v", page, err)
	}
	v := page.Versions[0]
	if v.SizeBytes != 123 || !v.IsEncrypted || v.EncryptAlgo != "AES-256" || v.EncryptHint != "key-a" || v.ChecksumSha256 != strings.Repeat("a", 64) || v.StoragePath != "tenant/model/v1/model.tar" || !v.CreatedAt.AsTime().Equal(created) {
		t.Fatalf("version metadata=%v", v)
	}
	ready, err := server.GetModelVersion(principal(tenant), &modelv1.GetModelVersionRequest{ModelVersionId: id(10)})
	if err != nil || ready.GetVersion().GetSizeBytes() != 123 || !ready.GetVersion().GetIsEncrypted() || !ready.GetVersion().GetCreatedAt().AsTime().Equal(created) {
		t.Fatalf("ready metadata=%v err=%v", ready, err)
	}
	versionRequest.ModelId = id(2)
	_, err = server.ListModelVersions(principal(tenant), versionRequest)
	if kratoserrors.Code(err) != 400 {
		t.Fatalf("cursor reused for different model: %v", err)
	}
	detail, err := server.GetModel(principal(tenant), &modelv1.GetModelRequest{ModelId: "smol-3"})
	if err != nil || len(detail.GetVersions()) != 1 || detail.Versions[0].Version != "v2" {
		t.Fatalf("detail=%v err=%v", detail, err)
	}
}
