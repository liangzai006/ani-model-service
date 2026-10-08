# Model import storage upload session

This change makes model import use the Model Service's selected Storage port
rather than a MinIO-specific Job contract. It does not change the existing
legacy environment or the separate `ani-resource-service` Storage design.

## Implemented path

1. The import worker ensures the staging PVC exists and checks for the
   deterministic Job name.
2. For a new Job, the worker asks the configured Storage port for a short-lived
   signed PUT URL for `imports/<task-id>/model.tar`.
3. The URL is stored in a task-scoped Kubernetes Secret. The Job receives only
   that Secret key through `ANI_IMPORT_UPLOAD_URL`; it does not receive MinIO
   endpoint, bucket, or credentials.
4. The import image downloads from ModelScope or Hugging Face, creates a tar
   stream, and uploads it with HTTP PUT. It sets the exact `Content-Length`,
   computes SHA-256 while streaming, rejects redirects, and closes the upload
   stream when the server responds early.
5. The worker verifies the resulting object through the same Storage port and
   removes the upload Secret and staging PVC on success or failure.

Both the direct S3-compatible adapter currently used by Model Service and the
Storage gRPC adapter can supply the signed URL. The Job contract is therefore
S3-compatible through the Storage port without naming MinIO. The direct adapter
still has its existing internal `ANI_MINIO_*` configuration because that is the
current backend implementation; it is not exposed to the import Job.

## Configuration boundary

`ANI_IMPORT_MINIO_SECRET` is no longer required or used. The Model Service
requires the import Job image and Kubernetes namespace, and may still use
`ANI_IMPORT_PROVIDER_SECRET` for provider credentials. PVC size continues to be
estimated from the provider manifest; there is no manual import size override.
The Model Service service account now has create/get/delete permission for the
short-lived upload Secrets.

## Verification

Targeted Go tests and `make verify` passed, including all repository tests,
`go vet`, build, module verification, and whitespace checks. No cluster E2E,
large-model production upload, multipart upload, or `ani-resource-service`
implementation was performed in this change.
