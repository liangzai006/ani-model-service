# Kubeflow v1.10 on the new cluster

This record describes the Kubeflow v1.10 installation on the cluster configured in `.clusterenv`. It is separate from the existing ANI environment and does not change the ANI namespaces, APISIX routes, IAM, trusted identity, or quota configuration.

## Versions

- Kubeflow manifests: `v1.10-branch`, commit `ef152a9`
- Kubeflow Pipelines: 2.5.0
- KServe: 0.15.0; KServe models web app: 0.14.0
- Training Operator: 1.9.2 (the Training Operator component shipped with Kubeflow v1.10)
- Notebook Web App, Dashboard, Profiles, PVC Viewer, TensorBoard: 1.10.0
- Istio: 1.26.1
- Knative Serving: 1.16.2
- cert-manager: 1.16.1
- Dex: 2.41.1; OAuth2 Proxy: 7.7.1

## Storage and access

Kubeflow's own MySQL and MinIO use the new cluster's `cephfs` StorageClass and their own PVCs in the `kubeflow` namespace. They do not reuse the ANI PostgreSQL or MinIO instances. The Istio ingress and cluster-local gateways are `ClusterIP` services; no public route or APISIX route was added.

For temporary internal access, use a port-forward from an operator workstation. Rotate the default Dex example account before sharing the dashboard.

## Images

Images that were unavailable or slow from public registries were mirrored into the Harbor Kubeflow repository and, where needed, imported into node containerd. Future public image pulls can be mirrored from the development machine with `oras cp` into `docker.changqingyun.cn/mirror/`, then referenced by a new-cluster-only patch.

The cluster has no `nvidia.com/gpu` resource. KServe is installed and its CPU control plane is ready; GPU inference requires a later GPU node/runtime installation.
