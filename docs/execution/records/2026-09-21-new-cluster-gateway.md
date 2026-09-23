# New-cluster gateway installation (2026-09-21)

Scope: the Kubernetes cluster described by `.clusterenv`, using the control
plane at `192.168.102.68` with `/etc/kubernetes/admin.conf`. The local
`kubernetes-admin@kubernetes` context and all legacy resources were left
untouched.

## Installed components

- Gateway API standard CRDs v1.6.1, applied server-side. The downloaded
  `standard-install.yaml` SHA256 is
  `24d931f22abd8e40c973264319ead7cfa09d0fb7716b7ab1ee2ff174cb063a73`.
- APISIX Helm chart 2.17.0 (APISIX 3.18.0). The chart archive SHA256 is
  `32614444bee4e185f52a66af624875b56f048ef7bcb9600fd3b99d0f7859bb78`;
  Helm release `ani-apisix` is revision 1 in `ingress-apisix`.
- APISIX Ingress Controller 2.2.0, with Gateway API enabled and the
  controller name `apisix.apache.org/ani-apisix-ingress-controller`.
- One APISIX etcd replica with a 1Gi `rook-ceph-block` PVC.
- APISIX proxy Service `ani-apisix-gateway` as NodePort `30090`; the admin
  Service remains an internal ClusterIP.

All gateway images were copied into new Harbor tags
`docker.changqingyun.cn/ani/<name>:new-cluster-20260921-r1` without changing
existing tags:

| image | Harbor tag digest |
| --- | --- |
| `apisix` | `sha256:779f0069844f458f9f0d70c52ee905325a9ee2ed174a34c5ced912b23406901b` |
| `apisix-ingress-controller` | `sha256:8b0edac17ec048312d908f208827a12b8955421fced6f0c697af5b42e0f0edda` |
| `apisix-adc` | `sha256:6a37a30cf82d839d27789d9133c3ed15f7a2910ba24cfb8ad0d1033d8970878f` |
| `apisix-etcd` | `sha256:c5f810e973f9d0985c4075650cbfaf6c2fb31d6508fdb2e5ed03dc58c5f5d2a6` |
| `apisix-busybox` | `sha256:74f634b1bc1bd74535d5209589734efbd44a25f4e2dc96d78784576a3eb5b335` |

The isolated values file is
`docs/deploy/apisix/new-cluster-values.yaml`. The legacy
`docs/deploy/apisix/values.yaml` was not modified; it still describes the old
environment and its storage class.

## Verification

- APISIX, etcd and the two-container ingress controller are all `Running` and
  ready; the etcd PVC is `Bound` on `rook-ceph-block`.
- `GatewayClass/ani-apisix` is `Accepted=True`.
- `Gateway/ingress-apisix/ani-apisix` is `Accepted=True` and `Programmed=True`.
- The APISIX NodePort returned HTTP `404` with `{"error_msg":"404 Route Not Found"}`.
  This confirms the proxy endpoint is reachable while no backend route exists.
- `kubectl get httproute -A` returned no routes. The legacy
  `docs/deploy/apisix/smoke-route.yaml` was not applied because it references
  the old E2E namespace and vLLM Service.
- Existing new-cluster Model, PostgreSQL, and MinIO workloads remained ready;
  their Pods, Services and PVCs were unchanged by the gateway installation.

IAM/trusted principal, quota, TLS, GPU, Kubeflow and an Inference backend are
still intentionally deferred. A real HTTPRoute and inference request can be
added only after the new-cluster Inference service is deployed.
