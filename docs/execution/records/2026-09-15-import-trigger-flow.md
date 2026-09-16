# Import trigger flow

日期：2026-09-15

## 当前方案

1. Gateway 通过 Model gRPC `ImportModel` 提交 `source`、`repo_id`、`revision` 和幂等键。
2. Model 从可信 Principal 获取 tenant，`tenant_id` 只做交叉校验。
3. Model 将 provider payload 写入 `public.model_import_tasks`，初始状态为 `pending`，然后立即返回 task ID。
4. 持久 worker 定期扫描 PostgreSQL due tasks，抢占 lease 后进入 `importing`。
5. worker 按 `source` 从 HuggingFace/ModelScope adapter registry 选择 provider，获取外部对象元数据。
6. 通过 Storage adapter 检查对象存在性并验证 SHA256；成功后完成任务，绑定版本的任务先执行 ready CAS；失败则按 retry/CAS 规则重试或置为 `failed`。

上传流程使用 `GetUploadURL` 获取短期上传地址；客户端上传到外部 Storage 后，调用 `CreateModelVersion` 作为确认步骤。Model 校验对象存在性和摘要，写入 `model_artifacts`，再执行 ready CAS；重复确认不会再次插入已完成制品。Model 不创建或管理 bucket、PVC 或文件系统。

## 验证

- `TestImportModelPersistsProviderPayload` 通过，确认 provider 参数不会在受理时丢失。
- worker 的 due scan、lease、CAS fencing 和 Storage 校验已有独立测试。

## 未验证

- 真实 HuggingFace/ModelScope 下载、Storage 上传和 provider 元数据后的模型/版本绑定仍为 `not_verified`。
