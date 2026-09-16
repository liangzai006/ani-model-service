# Import worker lease domain slice

新增 `internal/biz/work` 持久导入任务状态机：

- `pending/importing` 可在无有效 lease 时 claim
- claim 递增 `lease_epoch`、记录 `lease_owner`、`lease_until` 和 attempt
- completed/failed 仅允许 owner、epoch 匹配且 lease 未过期的 worker 写入
- 旧 worker、错误 owner、过期 lease 均返回 `ErrLeaseFenced`

该切片只实现领域规则，PostgreSQL CAS 查询已由 sqlc 生成但尚未接入 worker 循环。

验证：`go test ./internal/biz/work ./internal/biz/storage ./internal/service`、`go vet ./...`、`go build ./...` 均通过。
