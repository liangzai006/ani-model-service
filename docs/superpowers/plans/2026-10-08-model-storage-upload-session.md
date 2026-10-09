# Model Import Storage Upload Session Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Kubernetes model-import Jobs use an upload URL created by the Model Service's selected Storage port, removing MinIO-specific Job configuration while preserving the existing artifact finalization flow.

**Architecture:** The executor computes the deterministic object reference, asks `storage.Port.CreateUploadURL` for a short-lived URL, stores that URL in a task-scoped Kubernetes Secret, and creates the Job with only a Secret reference. The importer archives the downloaded model and uploads it with HTTP PUT; Model Service verifies the resulting object through the same Storage port. Direct S3-compatible and Storage gRPC adapters remain interchangeable behind the port.

**Tech Stack:** Go, Kubernetes client-go, fake Kubernetes clientset, existing Storage port and signed URL contract, HTTP PUT.

## Global Constraints

- Do not change `ani-resource-service` in this slice.
- Do not expose MinIO endpoint, bucket, or credentials in the import Job contract.
- Do not synthesize model-engine startup arguments; this slice only changes model import storage.
- Preserve deterministic object references and artifact checksum verification.
- Keep old environment deployment files untouched; new Model Service configuration must use the generic Storage path.

---

### Task 1: Extend the Kubernetes import adapter for upload Secrets

**Files:**
- Modify: `internal/data/kubernetes/import_job.go`
- Test: `internal/data/kubernetes/import_job_test.go`

- [ ] **Step 1: Write the failing test**

Add a test that creates a task-scoped Secret through `ImportJobClient.CreateSecret`, verifies the secret data, and deletes it through `DeleteSecret`.

- [ ] **Step 2: Run the focused test and verify it fails**

Run: `go test ./internal/data/kubernetes -run TestImportJobClientSecretLifecycle -count=1`
Expected: FAIL because the client methods do not exist.

- [ ] **Step 3: Implement the minimal client methods**

Add `CreateSecret(ctx, *corev1.Secret) (*corev1.Secret, error)` and `DeleteSecret(ctx, namespace, name string) error`, with the same nil-client and wrapped-error behavior as PVC methods.

- [ ] **Step 4: Run the focused test and verify it passes**

Run: `go test ./internal/data/kubernetes -run TestImportJobClientSecretLifecycle -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/data/kubernetes/import_job.go internal/data/kubernetes/import_job_test.go
git commit -m "feat: manage model import upload secrets"
```

### Task 2: Make KubernetesImportExecutor request and inject upload URLs

**Files:**
- Modify: `internal/worker/kubernetes_executor.go`
- Test: `internal/worker/kubernetes_executor_test.go`

- [ ] **Step 1: Write failing tests**

Add a Storage fake implementing `CreateUploadURL`, `ObjectExists`, and `VerifyChecksum`. Test that `jobSpec` requests the deterministic object reference and that the resulting Job uses `ANI_IMPORT_UPLOAD_URL` through a Secret reference while containing no `ANI_MINIO_*` environment variables. Test that an expired/empty signed URL prevents Job creation.

- [ ] **Step 2: Run the focused tests and verify they fail**

Run: `go test ./internal/worker -run 'TestKubernetesImportExecutor(JobSpecUsesUploadURL|RejectsInvalidUploadURL)' -count=1`
Expected: FAIL because executor still requires MinIO fields and does not create upload Secrets.

- [ ] **Step 3: Implement the minimal executor flow**

Replace MinIO fields with an upload-secret name or derive it from the deterministic Job name. In `Execute`, first look up the deterministic Job; only when it is not found and is about to be created, compute the object reference, call `Storage.CreateUploadURL`, validate it with `storage.ValidateSignedURL`, create a Secret containing `ANI_IMPORT_UPLOAD_URL`, then create the Job. Add cleanup for the upload Secret on success/failure. Keep provider Secret handling unchanged. Job env must contain the object reference and a SecretKeyRef for the upload URL, never a literal URL or MinIO configuration.

- [ ] **Step 4: Run focused tests and verify they pass**

Run: `go test ./internal/worker -run 'TestKubernetesImportExecutor(JobSpecUsesUploadURL|RejectsInvalidUploadURL)' -count=1`
Expected: PASS.

- [ ] **Step 5: Update existing executor tests and commit**

Replace MinIO-specific assertions with upload-Secret assertions, run `go test ./internal/worker -count=1`, then commit:

```bash
git add internal/worker/kubernetes_executor.go internal/worker/kubernetes_executor_test.go
git commit -m "feat: use storage upload URLs for import jobs"
```

### Task 3: Change the importer Job to upload through the signed URL

**Files:**
- Modify: `cmd/minio-provision/main.go`
- Modify: `cmd/minio-provision/main_test.go`

- [ ] **Step 1: Write failing tests**

Add tests for missing `ANI_IMPORT_UPLOAD_URL`, successful HTTP PUT with the archive content length, and non-2xx upload response. Keep provider download command tests unchanged.

- [ ] **Step 2: Run focused tests and verify they fail**

Run: `go test ./cmd/minio-provision -run 'TestRun(RequiresUploadURL|UploadsArchiveThroughSignedURL|RejectsUploadFailure)' -count=1`
Expected: FAIL because the command still requires MinIO credentials and uses `minio-go`.

- [ ] **Step 3: Implement the minimal HTTP uploader**

Rename the command package comments and replace MinIO client setup, bucket creation, and `PutObject` with an HTTP PUT to `ANI_IMPORT_UPLOAD_URL`. Archive the staging directory to a temporary file or equivalent bounded stream so the request sets `Content-Length`; calculate SHA256 and size while producing the archive. Preserve the stable `ANI_IMPORT_RESULT` JSON and reject non-2xx responses. Remove `ANI_MINIO_*` reads from the command.

- [ ] **Step 4: Run focused tests and verify they pass**

Run: `go test ./cmd/minio-provision -run 'TestRun(RequiresUploadURL|UploadsArchiveThroughSignedURL|RejectsUploadFailure)' -count=1`
Expected: PASS.

- [ ] **Step 5: Run the full importer tests and commit**

```bash
go test ./cmd/minio-provision -count=1
git add cmd/minio-provision/main.go cmd/minio-provision/main_test.go
git commit -m "feat: upload imported models through signed URLs"
```

### Task 4: Remove MinIO-specific Model Service wiring and update deployment contract

**Files:**
- Modify: `cmd/ani-model-service/main.go`
- Modify: `cmd/ani-model-service/config_test.go`
- Modify: `cmd/ani-model-service/main_test.go`
- Modify: `docs/deploy/model/model-service.yaml`
- Modify: `docs/deploy/helm/ani-services.md`
- Modify: `docs/deploy/import-job.yaml`

- [ ] **Step 1: Write failing configuration tests**

Update the configuration tests to assert that Storage gRPC can configure Kubernetes import without `ANI_MINIO_ENDPOINT` or `ANI_IMPORT_MINIO_SECRET`, and that no direct-MinIO-only error is returned.

- [ ] **Step 2: Run focused tests and verify they fail**

Run: `go test ./cmd/ani-model-service -run 'Test.*Storage|Test.*Import' -count=1`
Expected: FAIL because startup still requires direct MinIO for Kubernetes import and passes MinIO fields to the executor.

- [ ] **Step 3: Implement the wiring change**

Remove the `minioEndpoint` guard around Kubernetes import. Construct the executor with the selected `storagePort`, generic upload-Secret behavior, provider Secret, and storage class. Remove `ANI_IMPORT_MINIO_SECRET` validation. Keep direct MinIO adapter support only as an internal transition path if required by existing tests; do not expose its names in the Job contract or new deployment.

- [ ] **Step 4: Update the new deployment contract**

Remove `ANI_IMPORT_MINIO_SECRET` and all MinIO-specific variables from the importer Job contract. The Model Service may continue using its existing direct S3-compatible adapter or Storage gRPC adapter; both selected `storage.Port` implementations must be able to create upload URLs. Do not force the Resource Storage gRPC path before its API is defined, and do not modify old-environment deployment manifests.

Grant the Model Service service account only the `secrets` create/get/delete permissions needed for task-scoped upload Secrets in the import namespace. Remove the old MinIO Secret setup and `ANI_IMPORT_MINIO_SECRET` references from `docs/deploy/import-job.yaml`; retain provider Secret handling.

- [ ] **Step 5: Run focused tests and commit**

```bash
go test ./cmd/ani-model-service -run 'Test.*Storage|Test.*Import' -count=1
git add cmd/ani-model-service docs/deploy/model/model-service.yaml docs/deploy/helm/ani-services.md
git commit -m "feat: remove direct MinIO requirement from model imports"
```

### Task 5: Full verification and documentation consistency

**Files:**
- Modify: `docs/execution/records/2026-10-08-model-storage-upload-session.md`

- [ ] **Step 1: Run targeted package tests**

```bash
go test ./internal/data/kubernetes ./internal/worker ./cmd/minio-provision ./cmd/ani-model-service -count=1
```

- [ ] **Step 2: Run repository verification**

```bash
make verify
```

- [ ] **Step 3: Check for leaked MinIO Job contract references**

```bash
rg -n 'ANI_MINIO_|MinIOSecretName|MinIOEndpoint|MinIOBucket|MinIOTenantBuckets|MinIOSecure' internal/worker cmd/minio-provision docs/deploy/helm/ani-services.md
```

Expected: no import Job contract or new Storage gRPC deployment references remain; any remaining direct adapter references must be limited to the temporary internal S3-compatible adapter and old-environment manifests.

- [ ] **Step 4: Record validation and commit**

Append the test commands and results to the execution record and commit:

```bash
git add docs/execution/records/2026-10-08-model-storage-upload-session.md
git commit -m "docs: record model import storage validation"
```
