# Import checksum finalization

日期：2026-09-15

远程 provider 的内容响应可能没有 SHA256。worker 现在在流式上传、对象存在性和
Storage 内容校验完成后，把计算出的摘要及实际大小通过
`SetModelVersionChecksum` CAS 写入版本（仅允许 pending/importing，且原摘要为空或
与新摘要相同）。随后 artifact 持久化和 ready CAS 才会执行，因此空摘要版本也能
完成导入；已有摘要不一致时不会被覆盖。

验证：新增 `TestImportExecutorPersistsComputedChecksumForUnspecifiedVersion`，并通过
整仓 `go test ./...`、`go vet ./...`、`go build -trimpath ./...`、`buf lint` 和
`go mod verify`。真实 PostgreSQL checksum CAS、MinIO 对象和完整导入仍为
`not_verified`。
