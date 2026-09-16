# Upload finalization

日期：2026-09-15

## 实现

- `CreateModelVersion` 接收 `storage_path` 时，将其视为上传确认，而不是直接信任客户端字段。
- 通过 Storage adapter 执行对象存在性检查和 SHA256 校验。
- 校验通过后写入 `public.model_artifacts`，再使用 `VersionStateStore.MarkReady` 的 CAS 条件切换版本状态。
- 返回的 ModelVersion 包含 `storage_path`；已完成版本的幂等重放直接返回，不重复创建 artifact。
- Storage 对象和生命周期仍由外部 Storage 服务负责。

## 验证

- `TestCreateModelVersionFinalizesUploadedArtifact` 通过。
- `go test ./internal/service ./cmd/ani-model-service` 通过。

## 未验证

- 真实 Storage 上传地址、对象内容、跨进程 SHA256 和 URL 过期尚未联调，保持 `not_verified`。
