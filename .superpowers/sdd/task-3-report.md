# Task 3 Report

## Changes

- Replaced MinIO client, bucket creation, and `ANI_MINIO_*` configuration in the importer Job with `ANI_IMPORT_UPLOAD_URL` validation and an HTTP PUT.
- Archived the staging directory to a temporary tar file while calculating byte size and SHA256, then sent the archive with an explicit `Content-Length` and `application/x-tar` content type.
- Preserved `ANI_IMPORT_RESULT` fields and rejected non-2xx upload responses.
- Added focused tests for missing upload URLs, successful signed URL uploads, content length, and upload failures.

## Tests

- `go test ./cmd/minio-provision -run 'TestRun(RequiresUploadURL|UploadsArchiveThroughSignedURL|RejectsUploadFailure)' -count=1` (red before implementation; green after implementation)
- `go test ./cmd/minio-provision -count=1` (passed)

Review follow-up:

- Content length is measured by a first archive pass, then the archive is streamed from the staging PVC through a bounded pipe; no temporary archive is written to container `/tmp`.
- Redirects are disabled for the signed URL client, so every 3xx response is rejected. Added redirect rejection coverage.

Final validation:

- `go test ./cmd/minio-provision -count=1` (passed)
