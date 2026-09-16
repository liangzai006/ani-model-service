# Idempotency domain slice

新增 `internal/biz/idempotency`：

- 统一校验 1~128 字符幂等键格式
- 对请求 payload 计算 SHA256 fingerprint
- 只有 stored fingerprint 与当前一致才允许 replay
- 参数变化可由上层映射为 Conflict，避免同一 key 复用不同请求

尚未把 fingerprint 与结果写入 PostgreSQL 本地事务。

验证：`go test ./internal/biz/idempotency ./internal/service`、`go vet ./...`、`go build ./...` 均通过。
