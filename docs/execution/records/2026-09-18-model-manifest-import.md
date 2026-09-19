# 完整模型仓库导入与任务重试验收

日期：2026-09-18。该记录闭合 AI 服务基础功能计划的 C 切片。

## 验收结果

显式运行 `TestImportManifestRepositoryE2E`，每次使用随机专用租户，凭据由宿主机环境注入，未写入命令记录或仓库。测试使用真实 PostgreSQL、集群 MinIO 和 HuggingFace API，worker 负责按租户幂等创建 bucket。

- provider：`sshleifer/tiny-gpt2`，revision `main`。
- manifest：9 个文件，worker 安全打包为 tar；路径校验拒绝绝对路径、父目录、反斜杠和 NUL。
- Storage：对象 `<tenant>/sshleifer/tiny-gpt2/main/model.tar`，实际大小 4,742,656 字节。
- checksum：`361d696078e595f9ef50fdcf9036b08f216f180a7c4f6def9a63eff3fc114dd4`，下载内容 SHA256 与 artifact 和 ModelVersion 一致。
- ModelVersion：由 pending 进入 ready，artifact reference、format、大小和摘要均已持久化。
- ImportTask：首次 `GetImportTask` 查询到 `completed`、`progress_pct=100`、完成时间；随后注入失败状态，`RetryImportTask` 返回 `pending`、`attempt_count=0`、`progress_pct=0` 且清空错误，再次由 worker 处理到 `completed`。
- 归档内容包含 `config.json`、`merges.txt`、`pytorch_model.bin`、`special_tokens_map.json`、`tokenizer_config.json`、`vocab.json` 等 9 个条目。

测试最终退出码为 0，耗时约 14 秒。测试资源保留在随机租户下作为开发环境证据，不涉及生产租户。
最新复验（退出码 0，26.04 秒）使用租户 `3db58b8e-ae3a-4a7c-b615-6faa9cea53c1`、任务 `ac66fcb7-046b-42f2-bc76-6c1b37c9ecbe` 和版本 `fb4ec42e-e0db-4b6a-b0f4-8eac6dac17c6`；对象大小、SHA256 和重试结果与上述一致。

## 复验命令

凭据注入后运行：

```bash
MODEL_IMPORT_MANIFEST_E2E=1 \
E2E_DSN='postgres://…/recycling?sslmode=disable' \
E2E_MINIO_ENDPOINT='10.10.1.68:30900' \
E2E_MINIO_ACCESS='…' E2E_MINIO_SECRET='…' E2E_MINIO_INSECURE=1 \
go test ./internal/worker -run '^TestImportManifestRepositoryE2E$' -count=1 -v
```

该测试证明 Model 内部 worker 及 PostgreSQL/MinIO/provider 链路，不证明正式 Gateway/IAM、独立 Storage gRPC 服务、Inference 生命周期或 Kubernetes Pod crash/restart。
