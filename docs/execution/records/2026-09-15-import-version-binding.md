# Remote import version binding

日期：2026-09-15

## 实现

- `ImportModelRequest` 增加可选 `model_id`、`model_version_id`，兼容旧字段编号。
- Model handler 从可信 Principal 取得 tenant，仅把绑定 ID 作为交叉校验后的业务参数，并验证 UUID 格式。
- worker 对已绑定版本的 provider 结果执行 Storage 存在性和 SHA256 校验，然后写入不可变 artifact；随后由 lease/CAS finalizer 置版本 ready。
- artifact 使用 task ID 作为幂等键；worker 崩溃重试时，若唯一键已存在且引用、摘要、格式、大小一致，则视为成功，冲突内容拒绝。
- 未绑定任务仍可持久化，但不会自动把 repo_id 推断成模型身份。

## 验证

- `go test ./internal/worker ./internal/service` 通过。
- `make verify` 在前一切片已通过；本切片完整验证待重新执行。

## 未验证

- 真实 provider、Storage、PostgreSQL 绑定任务和进程重启联调仍为 `not_verified`。
- 本切片的完整 `make verify` 被自动审批连续拒绝，原因是执行模型容量不足；已单独通过 `buf lint`、`go vet ./...`、`go build -trimpath ./...` 和定向 Go 测试。
