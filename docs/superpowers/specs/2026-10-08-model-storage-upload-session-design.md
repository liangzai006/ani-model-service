# Model Import Storage Upload Session

## Scope

Refactor the Kubernetes model import path in `ani-model-service`. The import Job
must use the same `storage.Port` selected by Model Service and must not receive
MinIO-specific endpoint, bucket, or credentials. The later
`ani-resource-service` Storage implementation is outside this change.

## Design

`KubernetesImportExecutor` requests a short-lived upload URL from its
`storage.Port` for the task object reference before creating the Job. The URL is
passed to the Job through a task-scoped Kubernetes Secret. The Job downloads the
model, creates the archive, and uploads it with HTTP PUT to that URL. The Job
reports the stable object reference, size, and SHA256; Model Service continues
to verify the object through the same `storage.Port` and persists Artifact
metadata.

The executor no longer requires a direct MinIO endpoint or MinIO Secret. The
storage implementation may currently be a direct S3-compatible adapter or a
Storage gRPC adapter; this choice is invisible to the Job. The current MinIO
adapter remains an implementation detail during migration and is not part of
the import Job contract.

## Object and retry behavior

The object reference remains deterministic (`imports/<task-id>/model.tar`). The
upload URL is scoped to that object and task. A new task attempt gets a new URL
and a new Job/Secret name. The Secret is deleted with the Job cleanup path. URL
creation failures happen before Job creation and are returned as import errors.

The first implementation uses the existing signed-URL contract. The Job must
set the archive content length when known and fail on non-2xx responses. A
future multipart upload extension can be added to the Storage contract without
reintroducing backend-specific configuration into the Job.

## Configuration boundary

The Model Service deployment may select either its current direct storage
adapter or the external Storage gRPC adapter. Kubernetes import is valid for
both. `ANI_MINIO_*` and `ANI_IMPORT_MINIO_SECRET` are removed from the import
Job contract and from the new deployment configuration. Provider credentials
remain separate.

## Validation

Tests cover URL creation and propagation into the Job Secret, absence of
MinIO-specific Job environment variables, upload failure handling, deterministic
object references, and successful finalization through the selected storage
port. Existing unit and repository verification checks must pass.
