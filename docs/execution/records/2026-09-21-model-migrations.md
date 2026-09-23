# New-cluster Model migrations (2026-09-21)

This record covers only the Model database in the new `.clusterenv` cluster.
The existing cluster and its databases were not changed. Inference migrations
remain deferred until its separate database and service slice is selected.

## Applied resources

- PostgreSQL Service: `postgres.ani-foundation.svc.cluster.local:5432`
- Database/user: `ani_model` / `ani_model`, with credentials held only in the
  new-cluster Secret `ani-foundation/postgres-auth`
- One-shot Job: `ani-foundation/model-migrations-20260921`
- Migration source: repository `migrations/000001` through `000006`
- Migration image: `quay.io/sclorg/postgresql-16-c9s@sha256:8cfb29c886b54dd2c535e24896dffe45ff47a3006b9e880ef864415683f1daed`

The six SQL files were copied into the new control plane's temporary staging
directory and created as the `ani-foundation/model-migrations-20260921`
ConfigMap. The staging copy and ConfigMap contain no credentials.

## Verification

- Job condition: `Complete`, `1/1`, with `model_migrations=5` in its log.
- PostgreSQL readiness: `127.0.0.1:5432 - accepting connections`.
- The five expected public tables exist: `models`, `model_versions`,
  `model_artifacts`, `model_import_tasks`, and `audit_events`.
- The external `models.model_id` column exists.
- The migration Job reads the ConfigMap's projected SQL symlinks; the manifest
  explicitly accepts both regular files and symlinks. This avoids silently
  completing without applying files.

## Isolation evidence

- The migration Job ran only in `ani-foundation` on the new control plane via
  `/etc/kubernetes/admin.conf` on `192.168.102.68`.
- The local kubeconfig context remained `kubernetes-admin@kubernetes` before
  and after the operation; no apply/delete command targeted that context.
