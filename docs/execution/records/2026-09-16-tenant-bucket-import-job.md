# Tenant bucket import job

日期：2026-09-16

本切片将远程导入改为“任务入库后立即唤醒、worker 执行”的流程。PostgreSQL
任务表仍是唯一可靠来源，定时 due scan 继续保留，用于进程重启或唤醒丢失后的恢复。

MinIO 租户 bucket 规则：设置 `ANI_MINIO_TENANT_BUCKETS=true` 后，bucket 名称为
租户 UUID。下载前 adapter 执行 `BucketExists`；不存在时执行一次 `MakeBucket`，
并发创建返回已存在时视为成功。不会删除 bucket、PVC 或管理 MinIO 服务生命周期。
`cmd/minio-provision` 可通过 `ANI_MINIO_TENANT_ID` 显式预创建同名 bucket。

详细日志写入 Model Pod stdout，可使用：

```bash
kubectl -n <model-namespace> logs <model-pod> -f
```

日志阶段包括 `import task claimed`、`bucket ensure started`、`bucket ensured`、
`provider download started`、`provider download completed`、`storage upload completed`、
`import version ready` 和 `import task completed`。凭据、签名 URL 和模型内容不会写入日志。

真实验证：开发机临时 Model gRPC 进程连接 Docker PostgreSQL 和集群 MinIO，使用租户
`99999999-9999-4999-8999-999999999995`。该租户 bucket 首次不存在，由 worker 在
Provider 请求前创建；任务 `8c69c00a-7d81-4f8c-bef7-b65c12ea927e` 随后完成。

关键 stdout：

```text
import task claimed
bucket ensure started
bucket ensured
provider download started
provider download completed bytes=662
storage upload completed bytes=662
import version ready
import task completed
```

最终状态为 `ready`，HTTP 下载状态 `200`，checksum 为
`77a9e9830c3abeba929f5c61e0e97c398f98b4481ad75516c0be5bf038b340b0`，进程退出码为 0。
`import worker wake requested` 已由 worker 单元测试验证；本次真实 E2E 运行早于该日志
追加，因此不把它写入真实 stdout 证据。

验证结果：`go test ./...` 在允许 loopback listener 的运行环境通过；`go vet ./...`、
`go build -trimpath ./...`、Buf lint、`go mod tidy -diff`、`go mod verify` 和
`git diff --check` 通过。默认沙箱运行时测试会被 loopback 权限拒绝，单独提升权限后
`tests/runtime` 通过。第二次真实导入复用已有租户 bucket 的验证尚未完成。

本轮 focused 回归（worker、Storage、service、Model composition root）及静态检查均通过；
未执行 commit、push 或远程部署。

补充真实复验（worker 在发起 `ImportModel` 前启动）：租户
`99999999-9999-4999-8999-999999999993`、task
`d87460bf-976a-4ea7-8f6a-9f8b9a997455`。stdout 首行是
`import worker wake requested`，随后在 7 毫秒内领取任务；bucket ensure、Provider 下载、
MinIO 上传、ready 和 completed 全部成功，最终 HTTP `200`，退出码为 0。

第二次真实导入复用已有租户 bucket 的验证脚本已准备，但执行请求连续被自动审批的
“Selected model is at capacity”拦截，未将其标记为真实通过；同一路径的“已有 bucket 不创建”
由 `internal/data/storage/minio_test.go` 的 fake API 测试覆盖。
