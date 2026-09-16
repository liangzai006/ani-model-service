# ani-model-service

Module: `github.com/zhangzhe-ctrl/ani-model-service`

This repository was generated from ANI's pinned Kratos layout. It is an
independent source snapshot: builds and runtime do not require the layout.

`THIRD_PARTY_NOTICES.go-kratos-layout.txt` preserves the upstream template's
MIT notice. This generated repository intentionally has no project `LICENSE`;
its owner must make that choice before publication.

## Local commands

```bash
make tools
make verify
go run ./cmd/ani-model-service -conf ./configs
```

导入下载不是写入 Model 进程本地磁盘，而是由 Provider 流式传给已配置的
Storage adapter，再写入外部 Storage（MinIO 时为配置的 bucket）。导入阶段日志写到
进程标准输出，包含 `tenant_id`、`task_id`、Provider、字节数和状态阶段；不会输出凭据
或签名 URL。查看方式：

```bash
# 本地
go run ./cmd/ani-model-service -conf ./configs 2>&1 | tee model-service.log

# Kubernetes
kubectl -n <model-namespace> get pods
kubectl -n <model-namespace> logs <model-pod> -f
```

真实导入成功后，`public.model_artifacts.reference` 保存 Storage 对象引用，
`GetModelDownloadURL` 只返回短期签名地址；对象本身要在 Storage 的 bucket 中查看。

After the initial source commit, run `make supply-chain-tools` and `make audit`;
review and commit `docs/scaffold/bom.cdx.json`. CI deliberately fails when that
runtime SBOM is missing or stale and reruns the vulnerability, secret,
notice-integrity, and license-evidence gates.

The committed listeners are loopback-only local defaults. Override them through
the typed `ANI` environment configuration when the deployment design is added.

## Runtime shell

Business requests follow `Client --HTTP--> independent Gateway --gRPC--> Model`; Model performs the model business processing.
The Gateway lives outside this repository and owns the public HTTP entry point
and HTTP-to-gRPC forwarding. Model uses the pinned layout's Kratos gRPC transport.
Its separate admin HTTP listener serves only `/healthz`, `/readyz`, and `/metrics`;
it is not a business API or a Gateway.

Storage integration must use the external service's confirmed, versioned contract.
The earlier speculative HTTP adapter has been withdrawn; the Storage domain port
remains, and real Storage integration is `not_verified`.

During development Model and Inference temporarily use the same PostgreSQL
database account. Model still writes only the `public` schema; credential and
schema-permission separation is a later deployment hardening step.

When MinIO is used directly, set `ANI_MINIO_ENDPOINT`,
`ANI_MINIO_ACCESS_KEY`, and `ANI_MINIO_SECRET_KEY`; the credentials should be
injected from a Kubernetes Secret. `ANI_MINIO_BUCKET` defaults to
`ani-models` for shared-bucket compatibility. With
`ANI_MINIO_TENANT_BUCKETS=true`, each tenant uses a bucket named by its UUID and
the import job checks/creates that bucket immediately before downloading.

The provisioning command can pre-create a tenant bucket, while the tenant-bucket
import path also creates a missing bucket idempotently at download time:

```bash
go run ./cmd/minio-provision
```

- Kratos lifecycle with graceful shutdown.
- gRPC and a separate admin HTTP server.
- Structured redacted logs, tracing, metrics, and middleware.
- `/healthz` reports process liveness.
- `/readyz` reports only completion of the local runtime start hook; add checks
  for real dependencies when the first vertical slice introduces them.
- `/metrics` exports the local Prometheus registry.

Domain, persistence and handler code is in progress. Production dependency wiring
and real integration evidence are tracked in `docs/execution/status.md`.
