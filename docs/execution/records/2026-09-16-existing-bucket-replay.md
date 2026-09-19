# Existing tenant bucket and ready-version import replay

日期：2026-09-16。接续任务 `01a09e98-2ace-7dc0-ad54-f36cc4546d09` 的未完成验证。

## 范围和结果

使用本机 Docker PostgreSQL `recycling`、集群 MinIO 和真实 HuggingFace 小文件，
复用专用测试租户 `99999999-9999-4999-8999-999999999993`。导入前通过只读
`CheckBucket` 确认租户 bucket 已存在，通过 Model gRPC 查询既有 ready 版本。
没有新增生产实现：上一轮的 `VersionStore.MarkReady` 已支持 ready 重放，
`SetModelVersionChecksum` 已允许相同摘要/大小的 ready 版本重放，本轮验证其真实行为。

可重复测试：`internal/worker/import_replay_integration_test.go`，默认跳过；显式启用后
缺少任意必要配置均失败。每次生成新任务幂等键，完成后再次提交相同请求验证返回
同一 task ID。动态分配 loopback 端口并在测试结束停止 worker 和 gRPC server。

真实执行结果（2026-09-16 11:19 +08:00）：

- task：`e9f6264e-a1fa-48fd-81f5-d39929e48610`，attempt=1，lease_epoch=1，completed。
- 复用版本：`0dbebd03-31b0-4991-b4d4-890c5b6007d6`，重放前后 UUID 和 SHA256 一致。
- 已有 bucket 检查、worker wake、claim、provider 下载、Storage 上传、ready 和 completed 均成功。
- 同幂等键再次调用 ImportModel 返回原 task ID。
- gRPC GetModelDownloadURL 返回地址下载 HTTP 200，实际内容为 662 字节。
- 实际下载 SHA256：`77a9e9830c3abeba929f5c61e0e97c398f98b4481ad75516c0be5bf038b340b0`，与版本摘要相同。
- 同一 1 秒 TTL 地址在过期前 HTTP 200，过期后 HTTP 403；网络错误不算过期通过。
- `TestImportReplayExistingBucket`：PASS，3.08 秒，Go 测试退出码 0。

另以 `MODEL_DATABASE_URL` 显式启用全部 PostgreSQL 集成测试，四项均 PASS：
ArtifactStoreTenantIsolation、ModelAndVersionIdempotency、WorkStoreLeaseRecovery、
WorkStoreRemoteImportWithoutModelBinding。这里的 LeaseRecovery 测试只覆盖租约操作和
旧 owner 拒绝，不能作为真实进程 crash/restart 的证据。

## 复验

先通过外部凭据管理向环境注入 `E2E_DSN`、`E2E_MINIO_ENDPOINT`、
`E2E_MINIO_ACCESS`、`E2E_MINIO_SECRET`，不将凭据写入命令记录或仓库。
仅对明文开发 MinIO 设置 `E2E_MINIO_INSECURE=1`（默认启用 TLS）。
目标必须是允许新增任务和重传对象的专用测试租户，且模型/版本已经 ready，
源必须与该版本原始导入内容一致；不要对业务租户执行。

```bash
MODEL_IMPORT_E2E=1 \
E2E_TENANT_ID=99999999-9999-4999-8999-999999999993 \
E2E_MODEL_ID=tiny-gpt2 E2E_VERSION=main \
E2E_REPO_ID='sshleifer/tiny-gpt2#config.json' E2E_REVISION=main \
go test ./internal/worker -run '^TestImportReplayExistingBucket$' -count=1 -v
```

该测试保留新增任务作为开发环境证据，不删除任何数据库或 MinIO 资源。
测试使用真实 PostgreSQL/MinIO/provider 和 Model handler，通过临时 gRPC server
注入测试 Principal；不证明正式 Kratos composition root、IAM/Gateway 身份链路、
独立 Storage gRPC 服务、Inference 进程联调或 Kubernetes crash/restart。
这些仍为 `not_verified`，跨资源审计事务也仍待完成。

另外新增 `TestWorkStoreExpiredLeaseCanBeReclaimedPostgres`：首个 worker 的短 lease
过期后，第二个 worker 以更高 `lease_epoch` 成功重领，旧 owner 完成操作被拒绝，新 owner
完成成功。真实 PostgreSQL 测试通过；正式 Kubernetes Pod crash/restart 与网络中断仍需
部署环境验证。

本轮继续补充 `internal/service/model_grpc_integration_test.go`：通过 bufconn 运行实际生成的
Model gRPC server，可信 middleware 注入 `tenant-a` Principal，客户端提交 `tenant-b`
请求，服务返回 gRPC `PermissionDenied`。这证明了 transport 层到业务 handler 的 tenant
交叉校验，不代表正式 IAM/Gateway resolver 已接入。

供应链检查补充：宿主机 `govulncheck` 首次发现 grpc v1.82.1、pgx v5.7.6 和
x/crypto v0.54.0 的可达漏洞，已升级至 grpc v1.83.2、pgx v5.9.2、x/crypto v0.56.0。
重新扫描结果为 0 个代码受影响漏洞；Gitleaks 扫描 1 个提交、约 560 KB，未发现泄漏。
`make audit` 的 SBOM 阶段要求 Git 工作区 clean；当前会话按约束未 commit，因此 SBOM
和最终 license gate 保持 `not_verified`，没有绕过该前置条件。

供应链收尾：在 `/tmp` 隔离 clean 快照中完成 `make audit` 的漏洞、Gitleaks、SBOM 和
license gate。CycloneDX BOM 共 52 个组件；`github.com/tinylib/msgp` 的 MIT license
证据根据其 LICENSE 文件补齐，`./scripts/verify-supply-chain` 输出
`supply-chain evidence verified`。生成的 BOM 和 license review 已同步回当前工作区，
但没有在当前仓库创建 commit 或执行 push。
