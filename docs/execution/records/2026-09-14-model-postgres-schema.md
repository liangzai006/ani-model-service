# Model PostgreSQL schema 切片

## 完成内容

- 新增 `migrations/000001_model_control_plane.sql`。
- 使用`public` schema，未修改 `public.inference_*`。
- 创建 `models`、`model_versions`、`model_artifacts`、`model_import_tasks`、`audit_events` 五张表。
- 模型版本与制品通过 tenant-scoped 复合外键关联；artifact SHA256、状态、大小和版本唯一性有数据库约束。
- 导入任务包含 `lease_owner`、`lease_epoch`、`lease_until`、attempt 和 due time。
- `queries/model.sql` 已加入版本查询、ready 查询、建模和租约 fenced 完成/重试查询；sqlc 代码已生成。

## 真实 PostgreSQL 验证

开发机 `recycling-postgres` 容器中的 `ani_inference` 数据库已应用该 migration。只读核验确认 `public Model tables` 五张表存在，所有表 `relrowsecurity=false`。未修改旧 ANI 或 Inference 表，未清库。

## 验证命令

- `sqlc generate`：pass
- `go test ./...`：pass
- `go vet ./...`：pass
- `go build ./...`：pass
