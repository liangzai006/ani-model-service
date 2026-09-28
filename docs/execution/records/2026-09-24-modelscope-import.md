# ModelScope 模型导入验证 — 2026-09-24

## 范围

本次只使用新集群显式上下文 `kubernetes-admin@kubekey`
（`https://192.168.102.68:6443`）。旧集群、旧项目和已有推理服务未操作。
Hugging Face 因新节点网络出口问题暂不作为本阶段依赖；后续远程模型导入统一优先使用
ModelScope。

## 已验证链路

1. 通过 Model gRPC `ImportModel` 创建任务 `22cd917d-66b7-4e4a-a984-a18ff165d16c`，
   显式绑定 Model `349cb63a-ba0d-40e1-a9d0-65c3b51edfa7` 和版本
   `9e402032-f244-4826-9952-3fbbb6edc22e`。
2. ModelScope 源为 `Qwen/Qwen2-0.5B-Instruct`，revision 为 `master`，provider
   为 `modelscope`。独立 Kubernetes Job 完成快照下载、打包和 MinIO 上传。
3. 任务状态为 `completed`，版本状态为 `ready`；制品大小为 `999602688` 字节，
   SHA256 为
   `e1fc761d399f82372857717c6b38c87ad238f9dc85cf033111b7b2d07bb89e1c`，
   storage path 为 `imports/22cd917d-66b7-4e4a-a984-a18ff165d16c/model.tar`。
4. 已推送的 r2 import worker 使用 provider 兼容形式：
   `download --revision master --local-dir <staging> Qwen/Qwen2-0.5B-Instruct`，
   并已对单文件和完整仓库下载做退出码验证。后续任务使用 r2，不再使用旧的
   `--model`/`--local_dir` 组合；本次历史任务的完成结果以 ModelScope 日志、MinIO
   制品和 ready 版本三方事实核对。

## 镜像与限制

- Model service：
  `docker.changqingyun.cn/ani/model-service:ani-model-new-20260924-r15@sha256:e63c00da18ff3057a5be7d0bdab1e1cef0504150ad672f44ed2d9572db6fe877`
- Import worker（后续任务）：
  `docker.changqingyun.cn/ani/model-import-worker:ani-model-new-20260924-r2@sha256:9b1a603d19ab0411eae070fcb1a94b819d7010c9c1dee40572e14ef655d5422e`
- 独立 Job 路径按制品实际大小工作，不受旧的 512MiB 进程内限制影响。
- 常驻 worker 的空 revision 默认值也按 provider 区分：ModelScope 使用 `master`，
  Hugging Face 使用 `main`；Model service 已滚动到 r15。

随后用同一 0.5B 制品做 KServe CPU 冒烟时，vLLM 在 4Gi 内存限制下 OOM；worker-2
当前内存请求约 87%，没有继续放大测试的调度余量。该 KServe 测试服务、预测器、物化
Job/PVC/Secret 和绑定记录已按 service-id 精确清理，ModelScope 模型制品保留。
这次记为容量限制，不记为 ModelScope 导入失败。
