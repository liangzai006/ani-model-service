# Trusted identity context

新增 `internal/identity`，定义 `Principal{TenantID, Actor, Workload, RequestID, Scopes}`，提供 context 注入、读取、租户交叉校验和 scope 检查。缺少可信 principal 或请求 tenant 与可信 tenant 不一致时拒绝。

该包不解析普通 Header，也不生成身份；正式 middleware/IAM adapter 负责验证凭据后调用 `WithPrincipal`。单元测试覆盖缺失身份和跨租户拒绝。

验证：`gofmt`、`go test ./...`、`go vet ./...`、`go build ./...` 均通过。
