-- Remote imports may be accepted before provider metadata is resolved to a
-- Model/ModelVersion. Binding is optional until that later step.
ALTER TABLE public.model_import_tasks
    ALTER COLUMN model_id DROP NOT NULL;
