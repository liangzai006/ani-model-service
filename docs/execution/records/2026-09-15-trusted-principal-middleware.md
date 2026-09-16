# Trusted principal middleware

日期：2026-09-15

## 实现

- 在 `internal/identity` 增加 `Resolver` 端口，由认证边界（Gateway/IAM）提供可信 Principal。
- 在 `internal/server` 增加可插拔 `PrincipalMiddleware`：resolver 成功后写入 `identity.WithPrincipal`，缺失、无效或解析失败返回 Kratos 401。
- 标准 `ServerMiddleware` 默认不配置 resolver；组合根可使用 `ServerMiddlewareWithPrincipal` 显式接入认证实现。
- middleware 不读取普通 `tenant_id`、`actor` 或任意请求 header，Model handler 继续把请求 tenant 仅作为交叉校验。

## 验证

- `go test ./internal/identity ./internal/server` 通过。
- 开发机 `make verify` 通过（Buf 固定版本/生成稳定性、`go mod tidy -diff`、全量测试、vet、build、mod verify、diff check）。

## 未验证

- 真实 Gateway/IAM resolver、凭据签发和跨进程身份转发尚未接入，标记 `not_verified`。
