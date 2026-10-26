-- Catalog defaults remain optional for historical versions and old consumers.
ALTER TABLE public.model_versions
    ADD COLUMN recommended_image_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN recommended_image_scope TEXT NOT NULL DEFAULT '',
    ADD CONSTRAINT model_versions_recommended_image_pair CHECK (
        (recommended_image_id = '' AND recommended_image_scope = '') OR
        (btrim(recommended_image_id) <> '' AND recommended_image_id = btrim(recommended_image_id)
         AND recommended_image_scope IN ('platform', 'tenant'))
    );

-- Prepare freezes inherited defaults; Complete never re-reads the source.
ALTER TABLE public.trained_artifact_registrations
    ADD COLUMN recommended_image_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN recommended_image_scope TEXT NOT NULL DEFAULT '',
    ADD CONSTRAINT trained_artifacts_recommended_image_pair CHECK (
        (recommended_image_id = '' AND recommended_image_scope = '') OR
        (btrim(recommended_image_id) <> '' AND recommended_image_id = btrim(recommended_image_id)
         AND recommended_image_scope IN ('platform', 'tenant'))
    );
