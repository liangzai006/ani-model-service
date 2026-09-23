# New-cluster foundation installation (2026-09-21)

This record covers only the new `.clusterenv` cluster. No E2E namespace,
Kubeflow component, TLS/IAM component, image registry, gateway, or GPU
component was installed.

## Installed resources

- Namespace: `ani-foundation`
- PostgreSQL StatefulSet: `postgres`, image
  `quay.io/sclorg/postgresql-16-c9s@sha256:8cfb29c886b54dd2c535e24896dffe45ff47a3006b9e880ef864415683f1daed`
- PostgreSQL Service: `postgres.ani-foundation.svc.cluster.local:5432`
- PostgreSQL PVC: `postgres-data-postgres-0`, 20Gi, `rook-ceph-block`
- MinIO StatefulSet: `minio`, image
  `quay.io/minio/minio@sha256:a1a8bd4ac40ad7881a245bab97323e18f971e4d4cba2c2007ec1bedd21cbaba2`
- MinIO API Service: `minio.ani-foundation.svc.cluster.local:9000`
- MinIO console Service port: `minio.ani-foundation.svc.cluster.local:9001`
- MinIO PVC: `minio-data-minio-0`, 100Gi, `rook-ceph-block`
- MinIO bucket: `ani-models`

Credentials are stored only in new-cluster Secrets `postgres-auth` and
`minio-auth` in `ani-foundation`; their values are not recorded here.

## Verification

- Both Pods are `Running`, Ready `1/1`, with zero restarts after recovery.
- Both PVCs are `Bound`.
- PostgreSQL `pg_isready` reports `accepting connections`.
- Cluster DNS resolves both `postgres` and `minio` Services.
- MinIO `/minio/health/ready` succeeds and `mc` can access the API using the
  in-Pod Secret environment.
- The `ani-models` bucket was created successfully.

## Recovery note

The PostgreSQL image initially failed because its UID 26 process could not
create `/var/lib/pgsql/data/userdata` on the newly mounted RBD volume. The
manifest now sets `runAsUser`, `runAsGroup`, and `fsGroup` to 26. The failed
Pod was deleted without deleting its PVC; StatefulSet recreated it and the
database became Ready. The first MinIO image pull was reset by the Quay
connection and succeeded on retry; no image was changed.
