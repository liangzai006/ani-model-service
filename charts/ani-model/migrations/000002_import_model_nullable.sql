-- Remote ImportModel requests do not carry a model_id in the compatible
-- contract. The worker binds the task to a model/version after provider
-- metadata is resolved; NULL keeps the durable task receivable without
-- inventing a model identity from repo_id.
ALTER TABLE public.model_import_tasks
  ALTER COLUMN model_id DROP NOT NULL;
