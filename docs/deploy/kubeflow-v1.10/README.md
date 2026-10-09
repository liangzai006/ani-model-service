# 新集群 Kubeflow v1.10

本文记录 `.clusterenv` 对应新集群中的 Kubeflow v1.10。Kubeflow 与 ANI
模型/推理业务隔离，不接入 ANI 的网关、业务认证或配额流程。

## 版本

- Kubeflow manifests：`v1.10-branch`，提交 `ef152a9`
- Kubeflow Pipelines：2.5.0
- KServe：0.15.0；KServe Models Web App：0.14.0
- Training Operator：1.9.2（Kubeflow v1.10 自带版本）
- Notebook Web App、Dashboard、Profiles、PVC Viewer、TensorBoard：1.10.0
- Istio：1.26.1
- Knative Serving：1.16.2
- cert-manager：1.16.1
- Dex：2.41.1；OAuth2 Proxy：7.7.1

## 存储与访问

Kubeflow 自己的 MySQL 和 MinIO 使用新集群的 `cephfs` StorageClass，并在
`kubeflow` 命名空间使用独立 PVC。它们不复用 ANI 的 PostgreSQL 或 MinIO。
Istio ingress 和集群内部网关均为 `ClusterIP` Service；当前不为 Kubeflow
增加公共路由。

临时内部访问请在运维开发机上使用端口转发。把 Dashboard 分享给其他人前，
请先轮换 Dex 示例账号的默认密码。

## 镜像

公共仓库中不可用或下载缓慢的镜像已经复制到 Harbor Kubeflow 仓库；必要时也
已导入节点的 containerd。后续如果还需要公共镜像，可以在开发机使用
`oras cp` 复制到 `docker.changqingyun.cn/mirror/`，再通过只针对新集群的
patch 引用，不要修改旧环境镜像标签。

当前集群没有 `nvidia.com/gpu` 资源。KServe 已安装，CPU 控制面已就绪；
GPU 推理需要后续准备 GPU 节点、驱动和设备插件后再验证。
