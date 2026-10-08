# Model storage upload cluster E2E — 2026-10-08

This verification used only the new `.clusterenv` cluster:

- Kubernetes API: `https://192.168.102.68:6443`
- Context: `kubernetes-admin@kubekey`
- Nodes: `192.168.102.68`, `192.168.102.72`, `192.168.102.73`

The old context points to `https://10.10.1.66:6443`; it was not selected or
contacted.

## Deployment

The Model Deployment was rolled to
`docker.changqingyun.cn/ani/model-service@sha256:71e9b7ecde91d33a248ce99c8f35bedac4c3592e3b8fd68266bc24df92c5a274`.
The import Job image was
`docker.changqingyun.cn/ani/model-import-worker@sha256:f93eda5753b010eab241ede0e9bb1b2d6cee9b045b3db79aacf3ed8371d0b64f`.
The Model Pod reached `1/1 Ready` with zero restarts. The deployment no longer
contains `ANI_IMPORT_MINIO_SECRET`, `ANI_IMPORT_JOB_SERVICE_ACCOUNT`, or
`ANI_IMPORT_EXECUTION_MODE`; it retains only the selected Storage backend and
Job/provider settings.

The import controller Role now permits the Model Service account to delete
terminal Jobs and staging PVCs, in addition to creating and observing them. The
worker deletes the Job before deleting its PVC, because a completed Job Pod
otherwise keeps PVC protection active. Task-scoped upload Secrets are deleted
as before.

## Real import path

A real gRPC request created a Model and Version, then submitted this ModelScope
import:

```text
repo: Qwen/Qwen2-0.5B-Instruct#config.json
revision: master
source: modelscope
```

The worker created a Kubernetes Job on the CephFS StorageClass. The Job pulled
`config.json`, streamed a tar archive through the Storage-generated signed PUT
URL, and emitted an `ANI_IMPORT_RESULT` with:

```text
object_ref=imports/4434119b-a282-4797-abdb-a3bea5d22443/model.tar
size_bytes=2560
sha256=839e7633cafe5e1619989ac782c32773d62bc37fbda2d264dfbb43aec07258dd
```

The public Model gRPC polling result was `completed`, `progress=100`,
`attempt=1`; the bound Version became `ready` with the same size, checksum and
storage path. Five seconds after completion, exact-label queries found no Job,
Pod, upload Secret or staging PVC for task
`4434119b-a282-4797-abdb-a3bea5d22443`.

An earlier task (`aa081cb4-8e78-4d05-a244-649984807722`) completed successfully
before the Job-delete RBAC permission was added. Its Secret was removed, while
its PVC remained protected by the completed Pod. The exact Job was then removed
manually; the PVC disappeared. The Role and worker cleanup ordering were fixed,
then the final task above verified automatic cleanup.

This test intentionally downloaded one small repository file to validate the
new signed-upload and cleanup path. It is not a full 0.5B-weight download or a
large-model capacity test. Model/Version records and uploaded test objects are
kept under dedicated test tenants as development evidence.

## Verification gates

The targeted race tests passed for Model Service, importer, worker and
Kubernetes adapters. `make verify` passed all repository tests, `go vet`, the
build, module verification and `git diff --check`.
