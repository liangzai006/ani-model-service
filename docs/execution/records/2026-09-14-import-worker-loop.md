# Import worker loop slice

新增 `internal/worker/import_worker.go`：

- 按 tenant 定时扫描 due tasks
- claim 前执行 lease/CAS
- 执行期间后台定期 renew lease
- 成功 complete，失败 retry
- 所有结果通过 Store 接口落 PostgreSQL，进程重启后可再次扫描 pending/importing 任务

尚未接入真实 provider executor、进程 crash 集成测试和组合根生命周期。

验证：`go test ./internal/worker ./internal/data/postgres`、`go vet ./...`、`go build ./...` 均通过。
