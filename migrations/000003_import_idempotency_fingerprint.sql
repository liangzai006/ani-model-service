-- Persist the request shape used for idempotency replay. The fingerprint is
-- scoped by tenant_id and idempotency_key; it is not an authentication token.
ALTER TABLE public.model_import_tasks
  ADD COLUMN request_fingerprint TEXT NOT NULL DEFAULT '';
