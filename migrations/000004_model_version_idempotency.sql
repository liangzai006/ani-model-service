ALTER TABLE public.models
  ADD COLUMN idempotency_key TEXT NOT NULL DEFAULT '',
  ADD COLUMN request_fingerprint TEXT NOT NULL DEFAULT '';

ALTER TABLE public.model_versions
  ADD COLUMN idempotency_key TEXT NOT NULL DEFAULT '',
  ADD COLUMN request_fingerprint TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX models_idempotency_key_uq
  ON public.models (tenant_id, idempotency_key)
  WHERE idempotency_key <> '';

CREATE UNIQUE INDEX model_versions_idempotency_key_uq
  ON public.model_versions (tenant_id, idempotency_key)
  WHERE idempotency_key <> '';
