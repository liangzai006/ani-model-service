# ImportModel handler slice

新增持久任务 `work.Creator` port，并实现 `ImportModel` 受理 handler：

- 可信 Principal 决定 tenant
- 仅允许 HuggingFace/ModelScope 来源
- 强制 repo_id 与幂等键非空
- 创建 pending 任务并返回任务摘要
- 未配置持久 worker 时保持未就绪错误，不使用内存队列冒充持久化

PostgreSQL task creator、幂等键唯一约束接入和真正 worker 扫描仍待完成。

验证：`go test ./internal/service ./internal/biz/work`、`go vet ./...`、`go build ./...` 均通过。
