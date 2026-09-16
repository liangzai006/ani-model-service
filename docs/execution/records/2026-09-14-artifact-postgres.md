# 制品元数据 PostgreSQL adapter

## 本轮切片

为 `public.model_artifacts` 增加 sqlc 创建/查询语句和 `ArtifactStore` adapter。adapter 只保存 provider、外部引用、格式、大小、SHA256 与加密元数据；对象内容和 bucket/PVC/文件系统生命周期仍由外部 Storage 服务负责。

所有读写都带 `tenant_id`，版本查询使用 `(tenant_id, model_version_id)` 复合条件，避免跨租户读取。写入前执行格式、引用、大小和 SHA256 领域校验，数据库约束继续作为最终防线。

## 验证

```text
GOPATH=/tmp/ani-model-gopath GOMODCACHE=/tmp/ani-model-gomodcache GOCACHE=/tmp/ani-model-gocache go test ./internal/biz/model ./internal/data/postgres
ok

MODEL_DATABASE_URL=postgres://.../recycling?sslmode=disable go test ./internal/data/postgres -run TestArtifactStoreTenantIsolationPostgres -count=1
ok (本机 Docker PostgreSQL，跨租户读取返回 pgx.ErrNoRows)
```

真实 Storage 对象存在性和 checksum 仍依赖尚未提供的正式外部契约，标记为 `not_verified`。
