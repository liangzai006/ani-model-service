# New cluster preflight (2026-09-21)

This preflight is read-only. It targets the cluster described by
`.clusterenv` and must not change the existing development cluster.

## Isolation boundary

- Existing kubeconfig context: `kubernetes-admin@kubernetes`, API nodes in the
  `10.10.1.x` network. Only read-only commands were run against it during this
  audit.
- New cluster hosts from `.clusterenv`: `192.168.102.68` (control plane),
  `192.168.102.72` and `192.168.102.73` (workers). The new cluster API server
  is reached through the control-plane host's `/etc/kubernetes/admin.conf`.
- Future commands must use an explicit new-cluster kubeconfig or the SSH
  wrapper for `192.168.102.68`; never change `~/.kube/config`, the current
  context, or resources in the old cluster.

## New cluster evidence

- Kubernetes `v1.37.0`; all three nodes are `Ready`.
- Namespaces currently contain only the base cluster, Calico, and `rook-ceph`.
  There is no application namespace for Model or Inference.
- Storage classes are `cephfs` (default, RWX-capable CephFS) and
  `rook-ceph-block` (RBD). Both use `WaitForFirstConsumer` and `Delete`.
- Rook Ceph `CephCluster/rook-ceph` and `CephFilesystem/cephfs` are `Ready`.
  Capacity is about 270 GiB raw with three OSDs. Ceph reports
  `HEALTH_WARN` only for insecure AES client key types currently allowed by
  the freshly installed CSI configuration; no PG or OSD health failure was
  observed.
- No `CephObjectStore` exists, so Ceph currently provides block/file storage,
  not an S3 endpoint for the Model Storage adapter.
- No node advertises `nvidia.com/gpu`; the hosts do not have `nvidia-smi`.
  GPU inference/vLLM cannot be validated on this cluster until GPU hardware,
  drivers, and the device plugin are installed.
- No APISIX, Gateway API controller, KServe, LWS, or Kubeflow components are
  installed. No PostgreSQL or MinIO workload/service exists.

## Required before a Model import smoke test

1. Dedicated test namespace and service accounts/RBAC.
2. PostgreSQL instance/database and the Model migrations (`migrations/*.sql`),
   with a new-cluster-only DSN and credentials.
3. MinIO S3 endpoint (or a separately approved Ceph RGW ObjectStore), bucket
   credentials, and a new-cluster-only Secret. The repository's documented
   default expects `ANI_MINIO_ENDPOINT` and `ANI_MINIO_*` variables.
4. Immutable Model service image, Deployment, gRPC/admin Services, config,
   readiness probes, and the import-controller Role/RoleBinding.
5. Immutable import Job image containing `/ani-model-import` plus the selected
   Hugging Face or ModelScope CLI, provider egress, and a PVC using `cephfs`.
6. A trusted IAM/Gateway identity resolver. The refactored Model process does
   not derive tenant/actor from ordinary request fields and has no local
   development principal fallback.

## Additional requirements for full Inference validation

- A separately deployed Inference service and its `ani_inference` migrations,
  with `ANI_MODEL_GRPC_ADDR` using TLS 1.3 and the Model service name.
- The Inference CRD from `ani-inference-service` and a service account allowed
  to create/watch its owned runtime resources.
- A materializer image and `ANI_MODEL_STORAGE_CLASS=cephfs`.
- A Gateway API controller plus APISIX and a Gateway/HTTPRoute publication
  target, or an explicitly scoped internal-only invocation path.
- GPU nodes/device plugin and the pinned vLLM image for a real GPU completion;
  CPU-only control-plane tests do not prove this path.

The Kubeflow integration proposal does not require installing the full
Kubeflow distribution for the Model import slice. KServe/Trainer/Pipelines/
Katib remain optional later execution layers and must not be installed merely
to validate Model metadata and import behavior.
