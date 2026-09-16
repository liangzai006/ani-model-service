# Worker observability metrics slice

新增 `WorkerMetrics` 并接入现有 Prometheus registry，暴露：

- `model_worker_backlog`
- `model_worker_oldest_age_seconds`
- `model_worker_claims_total`
- `model_worker_retries_total`
- `model_worker_checksum_failures_total`
- `model_worker_provider_errors_total`

提供受限更新方法供 worker 调用，未引入 tenant 高基数 label。

验证：`go test ./internal/server`、`go vet ./...`、`go build ./...` 均通过。
