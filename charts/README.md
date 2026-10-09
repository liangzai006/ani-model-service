# ANI Helm Charts

这些 Chart 用于部署 Model 和 Inference 两个业务服务：

| Chart | 管理的资源 | 不管理的资源 |
| --- | --- | --- |
| `ani-model` | Model gRPC/admin Deployment、S3 兼容存储适配、导入 Job 控制器、ServiceAccount 和命名空间级 RBAC | PostgreSQL、S3/MinIO、数据库迁移 Job、KServe、Higress |
| `ani-inference` | Inference 控制面 Deployment、gRPC/admin Service、KServe/LWS 和 Gateway API RBAC | KServe/LWS CRD、Higress 安装、PostgreSQL、模型工作负载 |

Chart 只描述新环境中的命名空间级期望状态。它们不会创建 Namespace，不会
安装集群级 Operator，也不会连接或操作其他集群。

## 前置条件

新环境初始化时先单独安装基础服务和 Operator，然后创建 Chart 所需的业务
Secret：

- PostgreSQL 和业务 Secret：`ani-model` 命名空间中的 `ani-model-database`
  以及 `ani-inference` 命名空间中的 `inference-database`，两个 Secret
  都使用 `dsn` 作为 key。服务启动前各执行一次 Model 和 Inference SQL
  migration。migration Job 清单位于 `docs/deploy/foundation/`，Chart
  不会安装可能被重复执行的 migration Job。
- S3 兼容 endpoint、新建的 `ani-model-minio` Secret（包含
  `ANI_MINIO_ACCESS_KEY` 和 `ANI_MINIO_SECRET_KEY`），以及已创建的共享
  `ani-models` bucket；或者准备 Storage gRPC endpoint 及其 TLS Secret。
  Chart values 使用 `storage` 命名，当前 Model 二进制内部仍通过
  `ANI_MINIO_*` 环境变量读取 S3 兼容配置。
- 新建的 `ani-model-public-ca` ConfigMap，包含
  `ca-certificates.crt`，供 HTTPS provider 下载使用。只有在镜像已经包含
  所需信任根时，才设置 `ani-model.caBundle.enabled=false`。
- KServe InferenceService/LLMInferenceService CRD、LeaderWorkerSet CRD 及
  控制器，Gateway API，以及新环境 Higress 网关和插件。CRD 与 Operator
  有意不放入 Chart。

查看或安装时始终显式指定新环境 context：

```bash
export KUBE_CONTEXT="${KUBE_CONTEXT:-kubernetes-admin@kubekey}"
kubectl --context "$KUBE_CONTEXT" config current-context
kubectl --context "$KUBE_CONTEXT" create namespace ani-model --dry-run=client -o yaml |
  kubectl --context "$KUBE_CONTEXT" apply -f -
kubectl --context "$KUBE_CONTEXT" label namespace ani-model \
  ani.liangzai006.io/environment=new-cluster --overwrite
kubectl --context "$KUBE_CONTEXT" create namespace ani-inference --dry-run=client -o yaml |
  kubectl --context "$KUBE_CONTEXT" apply -f -
kubectl --context "$KUBE_CONTEXT" label namespace ani-inference \
  ani.liangzai006.io/environment=new-cluster --overwrite
```

不要使用旧集群 context 执行这些命令。

## 安装

先渲染并检查结果，再安装：

```bash
helm lint charts/ani-model
helm template ani-model charts/ani-model \
  --namespace ani-model \
  --kube-context "$KUBE_CONTEXT" \
  -f charts/ani-model/values-new-environment.yaml > /tmp/ani-model.yaml
helm upgrade --install ani-model charts/ani-model \
  --namespace ani-model \
  --kube-context "$KUBE_CONTEXT" \
  -f charts/ani-model/values-new-environment.yaml \
  --wait --timeout 10m

export GATEWAY_NODE_IP="${GATEWAY_NODE_IP:?请设置新环境网关节点 IP}"
export HIGRESS_PUBLIC_BASE_URL="http://${GATEWAY_NODE_IP}:30090"
helm lint charts/ani-inference --set higress.publicBaseURL="$HIGRESS_PUBLIC_BASE_URL"
helm template ani-inference charts/ani-inference \
  --namespace ani-inference \
  --kube-context "$KUBE_CONTEXT" \
  -f charts/ani-inference/values-new-environment.yaml \
  --set higress.publicBaseURL="$HIGRESS_PUBLIC_BASE_URL" > /tmp/ani-inference.yaml
helm upgrade --install ani-inference charts/ani-inference \
  --namespace ani-inference \
  --kube-context "$KUBE_CONTEXT" \
  -f charts/ani-inference/values-new-environment.yaml \
  --set higress.publicBaseURL="$HIGRESS_PUBLIC_BASE_URL" \
  --wait --timeout 10m
```

`ani-inference` 必须设置公共 URL，因为该地址会写入生成的 Higress 路由。
默认 values 和新环境 values 都将它留空，避免渲染时静默发布到其他环境。

只使用 ModelScope 时，可在 values 文件或安装时设置 provider endpoint：

```yaml
import:
  modelscopeBaseURL: https://www.modelscope.cn
  huggingfaceBaseURL: ""
```

两个 Chart 都不会生成模型引擎启动命令。运行时 command 和 args 必须由推理
API 请求提供。

## 可选 Storage gRPC 模式

默认 Model 配置使用新环境中的 S3 兼容 endpoint。如果改用 Storage 服务，
请同时提供下面三个 gRPC 配置和 TLS Secret：

```yaml
storage:
  mode: grpc
  grpc:
    address: storage.ani-resource.svc.cluster.local:8443
    serverName: storage.ani-resource.svc.cluster.local
    caCertSecret: ani-storage-ca
    caCertKey: ca.crt
```

应用会拒绝同时配置两种存储模式。GPU 和 Accelerator 默认关闭；Inference
Chart 只有在 `accelerator.enabled=true` 且 TLS 配置完整时才会渲染相关内容。

## 新环境边界

这些 Chart 只用于干净的新环境，不会接管 raw YAML 或其他 Helm release 创建
的资源。如果目标 Namespace 已存在同名资源，应先规划独立迁移，不要使用 force
或跨集群清理命令。

## 验证

```bash
helm lint charts/ani-model
helm lint charts/ani-inference --set higress.publicBaseURL="$HIGRESS_PUBLIC_BASE_URL"
helm template ani-model charts/ani-model --namespace ani-model \
  --kube-context "$KUBE_CONTEXT" \
  -f charts/ani-model/values-new-environment.yaml >/tmp/ani-model.yaml
helm template ani-inference charts/ani-inference --namespace ani-inference \
  --kube-context "$KUBE_CONTEXT" \
  -f charts/ani-inference/values-new-environment.yaml \
  --set higress.publicBaseURL="$HIGRESS_PUBLIC_BASE_URL" >/tmp/ani-inference.yaml
```

应用前检查渲染结果中的 Secret 引用、ServiceAccount 名称、镜像 digest、KServe
权限和 Higress 路由 URL。
