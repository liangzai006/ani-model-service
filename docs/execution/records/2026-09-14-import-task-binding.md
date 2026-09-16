# ImportModel 任务关联切片

## 变更

兼容的 `ImportModelRequest` 没有 `model_id`，因此远程导入任务不能在受理阶段伪造模型 ID。新增 `000002_import_model_nullable.sql`，允许 `public.model_import_tasks.model_id` 暂为空；worker 在解析 provider 元数据后再绑定模型/版本。`WorkStore` 现在正确保存和读取空的模型/版本 UUID，并继续使用 `(tenant_id, idempotency_key)` 唯一约束。

## 开发机验证

```text
MODEL_DATABASE_URL=postgres://.../recycling?sslmode=disable \
go test ./internal/data/postgres -run 'TestWorkStore(RemoteImportWithoutModelBinding|LeaseRecovery)Postgres' -count=1
ok

make verify
PASS
```

远程 provider 下载、模型/版本实际绑定和 Storage 上传仍依赖外部契约，保持 `not_verified`。
