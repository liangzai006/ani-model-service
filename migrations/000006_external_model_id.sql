-- model_id is the stable identifier exposed to callers (for example
-- Qwen3-32B). The UUID id remains the internal row primary key.
ALTER TABLE public.models
  ADD COLUMN model_id TEXT NOT NULL DEFAULT '';

UPDATE public.models
SET model_id = name
WHERE model_id = '';

CREATE UNIQUE INDEX models_external_model_id_uq
  ON public.models (tenant_id, model_id)
  WHERE model_id <> '';
