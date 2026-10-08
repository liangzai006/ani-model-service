# Task 4 report: remove direct MinIO import wiring

## Changes

- Removed the Kubernetes import startup guard that required
  `ANI_MINIO_ENDPOINT`.
- Removed MinIO-specific fields and `ANI_IMPORT_MINIO_SECRET` validation from
  the Model Service composition root. The executor now receives the selected
  `storage.Port`, provider Secret name, storage class, and generic Job image.
- Updated configuration tests for direct storage and Storage gRPC import paths;
  neither path requires a task Job MinIO Secret.
- Added task-scoped upload Secret create/get/delete RBAC to the new-cluster
  Model Service manifest and removed `ANI_IMPORT_MINIO_SECRET` from its
  Deployment contract.
- Updated `docs/deploy/import-job.yaml` to document the signed upload Secret
  lifecycle and provider Secret only.

The existing uncommitted changes in `docs/deploy/model/model-service.yaml`
(image digest, inference address, and CA volume) were left unstaged and are not
part of this task. The pre-existing untracked Storage gRPC example was also
left out of the task commit.

## Verification

- `go test ./cmd/ani-model-service -run 'Test.*Storage|Test.*Import' -count=1`
- `go test ./cmd/ani-model-service -count=1`
- `make verify`
- PyYAML parsing of `docs/deploy/model/model-service.yaml`,
  `docs/deploy/import-job.yaml`, and the pre-existing Storage gRPC example.
