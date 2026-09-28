# New Cluster Foundation Installation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prepare only the new `.clusterenv` Kubernetes cluster for the refactored ANI Model/Inference services and a later Kubeflow installation, without touching the existing cluster.

**Architecture:** Keep the new cluster's Rook-Ceph as the storage layer. Run PostgreSQL and an S3-compatible service together in one dedicated foundation namespace, expose Model/Inference only through explicitly owned Services and Gateway resources, and reserve Kubeflow namespaces for a later phase. GPU and vLLM are a separate phase because the current new nodes expose no NVIDIA resources.

**Tech Stack:** Kubernetes v1.37.0, Calico, Rook-Ceph (`cephfs`, `rook-ceph-block`), PostgreSQL, MinIO or Ceph RGW, cert-manager, an explicitly selected gateway (APISIX or Istio/Kubeflow ingress), ANI Model/Inference services, and later Kubeflow components.

## Global Constraints

- Every command must target the new control plane at `192.168.102.68` through its dedicated kubeconfig/SSH wrapper.
- Do not change `~/.kube/config`, the current old-cluster context, or any old-cluster resource.
- Use dedicated new-cluster namespaces and Secrets; never reuse old-cluster namespaces, PVCs, databases, buckets, or credentials.
- Existing `cephfs` is the default RWX class; `rook-ceph-block` is the RBD class. Both currently use `WaitForFirstConsumer` and `Delete` reclaim policy.
- Do not install the full Kubeflow distribution in the foundation phase.
- Do not install GPU-dependent components until GPU hardware, NVIDIA drivers, and a device plugin are available.
- Pin all container images by immutable digest before applying workloads.
- Preserve old project resources and all existing old image tags. New project
  images have explicit new-project tags in addition to their pinned digests.
- Trusted IAM, quota, and TLS integration remain deferred by the user. Gateway
  infrastructure can be installed independently; it does not add an identity
  resolver or disable the service's existing authentication checks.

---

### Task 1: Lock the new-cluster boundary

**Files:**
- Read: `.clusterenv`
- Read: `docs/execution/records/2026-09-21-new-cluster-preflight.md`
- Create locally only: a new-cluster kubeconfig or SSH wrapper outside the repository

- [x] Keep the old context unchanged and verify `kubectl config current-context` before every operation.
- [x] Use the new control-plane admin configuration without copying it into the default kubeconfig.
- [x] Verify the three new nodes, `cephfs`, `rook-ceph-block`, and Rook Ceph health before installing workloads.

Expected result: all subsequent commands can be audited as new-cluster-only operations.

### Task 2: Create foundation namespaces and RBAC boundaries

**Resources:**
- `ani-foundation` for PostgreSQL, MinIO/RGW integration, and shared test-only secrets.
- `kubeflow` remains reserved for the later Kubeflow phase.

- [x] Create `ani-foundation` with labels identifying the new-cluster test environment.
- [ ] Create separate service accounts for PostgreSQL/MinIO administration, Model, import Jobs, and Inference.
- [ ] Add only namespace-scoped Roles and RoleBindings required by each service.
- [ ] Add ResourceQuota/LimitRange after the initial resource sizes are selected.

Expected result: no service account can access the old environment or unrelated new-cluster namespaces.

### Task 3: Install PostgreSQL for Model and Inference state

- [x] Deploy one new-cluster PostgreSQL instance on `rook-ceph-block` for the validation environment.
- [ ] Create separate databases/users for `ani_model` and `ani_inference`; do not reuse old-cluster credentials.
- [x] Apply the Model migrations from `migrations/` to the Model database.
- [ ] Apply the Inference migrations from `/root/kubercon/ani-inference-service/migrations/` to the Inference database.
- [x] Expose PostgreSQL only as an internal ClusterIP Service.
- [ ] Verify readiness, migration replay, tenant-scoped tables, and a backup/restore point before starting the services.

Expected result: both control planes have durable state isolated to the new cluster.

### Task 4: Install S3-compatible model storage

- [x] Choose exactly one backend for this validation: MinIO in `ani-foundation`, or a separately configured Rook CephObjectStore/RGW.
- [x] If using MinIO, persist it on a new-cluster Ceph volume and expose only an internal S3 Service.
- [x] Create new-cluster-only access credentials as a Secret; never put values in Git or command output.
- [x] Create the `ani-models` bucket and verify authenticated API access.
- [ ] Configure and test tenant-specific bucket creation when the Model worker is deployed.
- [ ] Verify upload, checksum readback, short-lived download URL expiry, and object isolation before Model deployment.

Expected result: the Model adapter and Import Job can use S3 without depending on the old MinIO instance.

### Task 5: Install identity and TLS foundations

Deferred by the user; not a prerequisite for the gateway infrastructure step.

- [ ] Install cert-manager in the new cluster if it is selected as the certificate authority.
- [ ] Create a new-cluster-only CA/issuer and certificates for Model gRPC and Inference-to-Model gRPC.
- [ ] Select the trusted IAM/Gateway principal injection path; the refactored Model service has no local principal fallback.
- [ ] Verify an authenticated tenant-scoped gRPC request and an unauthenticated rejection.

Expected result: service-to-service TLS and tenant identity are real deployment paths, not test-only context injection.

### Task 6: Deploy the refactored Model service

- [x] Publish immutable Model and Import Job images to a registry reachable by the new nodes.
- [x] Apply Model Deployment, gRPC/admin Services, ConfigMap, Secret references, ServiceAccount, and import-controller RBAC in the dedicated `ani-model` namespace.
- [x] Configure `ANI_DATABASE_DSN`, `ANI_MINIO_ENDPOINT`, `ANI_IMPORT_JOB_IMAGE`, `ANI_IMPORT_MINIO_SECRET`, `ANI_IMPORT_KUBERNETES_NAMESPACE`, and `ANI_IMPORT_STORAGE_CLASS=cephfs`; imports always run as Kubernetes Jobs and size their staging PVC from the provider manifest.
- [x] Verify `/healthz`, `/readyz`, PostgreSQL/Storage readiness, and namespace-scoped import RBAC without touching unrelated namespaces.
- [ ] Verify an authenticated Model gRPC request and run one import after the trusted IAM resolver is available.
- [ ] Run one small-model import and verify the ModelVersion artifact checksum and ready state.

Expected result: Model import and metadata APIs are ready in the isolated new-cluster slice.

### Task 7: Install the gateway/runtime prerequisites for Inference

- [ ] Install one Gateway API implementation and one externally owned gateway. Do not create competing APISIX and Istio routes for the same service.
- [ ] If the ANI publication design remains APISIX, install APISIX and its Gateway API controller in the new cluster; if Kubeflow ingress will own the path, use the approved Istio/Kubeflow ingress instead.
- [ ] Apply the Inference CRD `inferenceservices.ani.kubercloud.com` and namespace-scoped controller RBAC.
- [ ] Provide the materializer image and `ANI_MODEL_STORAGE_CLASS=cephfs`.
- [ ] Deploy Inference with its new-cluster PostgreSQL DSN, Model gRPC TLS address, namespace, and publication base URL.

Expected result: control-plane lifecycle can be tested without claiming a GPU completion.

### Task 8: Add GPU/vLLM only when hardware is ready

- [ ] Add GPU nodes to the new cluster.
- [ ] Install NVIDIA driver/operator and device plugin, then verify `nvidia.com/gpu` allocatable resources.
- [ ] Publish the pinned vLLM image and model materializer/import images.
- [ ] Run a single-replica, single-GPU inference smoke test before enabling distributed/LWS workloads.

Expected result: a real completion is validated only after the scheduler and device plugin expose an actual GPU.

### Task 9: Install Kubeflow in a separate phase

- [ ] Install the chosen Kubeflow components in the user's later phase after
  checking their own storage/network requirements; deferred ANI IAM/quota
  integration does not block installing independent infrastructure components.
- [ ] Keep Kubeflow Profiles/Namespaces separate from ANI tenant identity and Model/Inference PostgreSQL state.
- [ ] Add KServe only if it is selected as the Inference runtime provider; add Trainer, Pipelines, and Katib only when their corresponding use cases exist.
- [ ] Confirm Kubeflow controllers do not own the same Deployment, Service, CR, or route as the ANI controller.

Expected result: Kubeflow remains an execution-layer option and cannot bypass ANI IAM, tenant, quota, Model, or publication boundaries.
