# New-cluster Model deployment (2026-09-21)

This record covers the refactored Model service in the new `.clusterenv`
cluster. It does not install an E2E namespace, gateway, TLS/IAM resolver, GPU
runtime, or Kubeflow component.

## Deployed resources

- Namespace: `ani-model`
- Model Deployment: `ani-model/ani-model-service`, one replica
- gRPC Service: `ani-model/ani-model-grpc:19090`, internal ClusterIP
- Admin Service: `ani-model/ani-model-admin:19091`, internal ClusterIP
- ServiceAccount: `ani-model-service`; dynamically created import Jobs reuse it
- Import controller Role/RoleBinding: namespace-scoped Job, PVC, Pod and
  Pod-log access only
- Runtime Secrets: `ani-model/ani-model-database` and
  `ani-model/ani-model-minio`; values remain in the cluster and are not
  recorded here

The service uses the `ani_model` database and the internal MinIO endpoint in
`ani-foundation`. Shared bucket mode is enabled (`ANI_MINIO_TENANT_BUCKETS=false`)
because the current tenant-bucket import Job key and finalizer lookup paths do
not agree; tenant buckets remain deferred until that contract is fixed and
regression-tested.

## Immutable images

Both images also have the explicit new-project tag
`ani-model-new-20260921-r1`. The Deployment manifest includes that tag and the
digest together. Retagging preserved both digests and all old image tags; the
new-cluster rollout completed with one Ready replica and zero restarts.

- Model service:
  `docker.changqingyun.cn/ani/model-service@sha256:63c3234b574285a81c1cd12ce17fb61cd03008388c9d2d1253373b85cbf34d68`
- Import Job:
  `docker.changqingyun.cn/ani/model-import-worker@sha256:c8283c4e5b45a246c4684d6cb3ef6e298846c3c0c7798468f557e6cd22ba8e16`

The Model image was built from this repository with `CGO_ENABLED=0` and
includes the non-sensitive `configs/config.yaml`. The Import Job image is the
repository's `Dockerfile.import` output with `/ani-model-import`, ModelScope,
and Hugging Face tooling.

## Verification

- Focused Go tests passed before image build:
  `./cmd/ani-model-service`, `./internal/server`, `./internal/service`, and
  `./internal/worker`.
- Deployment rollout completed with `1/1` available and zero restarts on the
  current Pod.
- `/healthz` returned `{"status":"ok"}` and `/readyz` returned
  `{"status":"ready"}` through the internal admin Service.
- The gRPC `ListModels` call reached the service and returned
  `Unauthenticated: trusted principal is required`, which is the expected
  result while the trusted IAM resolver is deferred.
- The import image pulled successfully on both new-cluster workers
  `192.168.102.72` and `192.168.102.73`.
- The Model ServiceAccount can create Jobs/PVCs and read Pod logs in
  `ani-model`, but cannot read Secrets in `ani-foundation`.

## Resolved deployment mistakes

- An existing Harbor tag with the same old `model-service` name was initially
  deployed to the new namespace. Inspection after its startup failure found
  it to be `github.com/kubercloud/ani/services/model-service`.
  Its failed Deployment was deleted from `ani-model` only.
- The replacement scratch image initially lacked a static binary and then the
  non-sensitive config file; both failures were caught with local container
  runs before the final digest was deployed.

## Next phase boundary

The user explicitly deferred trusted IAM and quota integration and asked about
the gateway as the next infrastructure step. Read-only checks found no Gateway
API CRDs, GatewayClass, Gateway, or APISIX namespace in the new cluster. No
gateway was installed during this check. APISIX infrastructure and a basic
routing check can precede IAM/quota integration; Model HTTP-to-gRPC business
mapping and authenticated RPC acceptance remain separate work.

## 2026-09-22 validation update

For the isolated new-cluster validation, the Model service now accepts the
request tenant directly. A trusted Principal, Model gRPC TLS certificate, and
`ANI_MODEL_GRPC_SERVER_NAME` are not part of this deployment. The deployment
was rolled to `ani-model-new-20260922-r5` at
`sha256:d6815273baf628f0ff326ff8d245142b8d1b6a91d39963a96cb4f375eedd3ad6`.

An actual plaintext gRPC `ListModels` request without credentials returned
success (`models=0`) through the new-cluster Service. The sole Pod is `1/1`
Ready with zero restarts.
