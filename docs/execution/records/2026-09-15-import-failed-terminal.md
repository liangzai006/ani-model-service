# Import task failed terminal state

日期：2026-09-15

导入任务不再对 provider、Storage、checksum、绑定或 ready finalization 错误无限
重试。worker 默认最多尝试五次（可通过 `MaxAttempts` 调整）；未达到上限时保留
pending 并设置下一次 due 时间，达到上限时调用 `FailImportTask`，将任务置为
`failed` 并保存错误信息。

失败写入继续要求同一 tenant、task、lease owner、lease epoch 和有效 lease，旧
worker 或过期结果不能覆盖终态。进程重启后 pending 任务仍由 due scan 恢复。

验证：`TestWorkerFailsTaskAfterMaxAttempts`、worker 全量测试、整仓 `go test ./...`、
`go vet ./...`、`go build -trimpath ./...`、`buf lint` 和 `go mod verify` 通过。
真实 PostgreSQL failed CAS 与进程崩溃恢复仍为 `not_verified`。
