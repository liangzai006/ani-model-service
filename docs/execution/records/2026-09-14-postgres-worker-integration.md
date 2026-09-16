# PostgreSQL worker integration evidence

在本机 Docker 容器 `recycling-postgres`（PostgreSQL 17.11）中确认：

- 数据库：`recycling`
- 用户：`recycling_user`
- Model 表在 `public` schema 初始不存在，已应用 `migrations/000001_model_control_plane.sql`
- 未执行清库或删除操作

新增 `internal/data/postgres/work_integration_test.go`（仅设置 `MODEL_DATABASE_URL` 时运行）。测试创建唯一 tenant/model/task，真实执行：

1. CreateImportTask
2. Claim（worker-a）
3. Renew lease
4. 旧 owner 完成被拒绝
5. 正确 owner + epoch 完成成功

命令与结果：

```text
MODEL_DATABASE_URL='postgres://recycling_user:change-me-before-development@127.0.0.1:5432/recycling?sslmode=disable' \
  go test ./internal/data/postgres -run TestWorkStoreLeaseRecoveryPostgres -count=1
ok
```

另行验证：`go test -count=1 ./internal/...`、`go vet ./...`、`go build ./...` 均通过。该证据覆盖 lease/CAS 和旧 worker fencing；尚未覆盖进程 crash 中断后的自动恢复与真实 provider 下载。
