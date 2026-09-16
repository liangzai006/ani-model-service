# Provider streaming worker

日期：2026-09-15

## 实现

- 新增 HuggingFace/ModelScope HTTP source adapter，要求 `repo_id` 使用 `repo#file` 明确选择单个文件，revision 默认为 `main`。
- provider 以 HTTP response body 流式返回，不把模型文件整体读入内存。
- Storage adapter 通过 gRPC `CreateUploadURL` 获取短期地址，再以 HTTP PUT 流式上传；Model 不创建 bucket 或管理对象生命周期。
- worker 上传后检查对象存在性并调用 Storage checksum 校验。
- 组合根在 PostgreSQL、Storage 和 `ANI_IMPORT_WORKER_TENANT_ID` 都配置时构造 provider registry、ImportExecutor 和 WorkerSupervisor；缺少任一依赖时保持未就绪。

## 配置

```text
ANI_IMPORT_WORKER_TENANT_ID=<tenant UUID>
ANI_IMPORT_WORKER_OWNER=<optional worker owner>
ANI_HUGGINGFACE_BASE_URL=<optional API base, default https://huggingface.co>
ANI_MODELSCOPE_BASE_URL=<optional API base, default https://www.modelscope.cn>
```

## 验证

- HTTP provider 路径、revision、文件选择和流式响应测试通过。
- worker provider 内容流到 Storage、对象存在性和 checksum 测试通过。
- 组合根编译和相关包测试通过。
- 开发机完整 `make verify` 通过。

## 未验证

- 尚未连接真实外部 HuggingFace/ModelScope 与 Storage 服务端；真实下载、上传、凭据、限流和大文件恢复仍为 `not_verified`。
