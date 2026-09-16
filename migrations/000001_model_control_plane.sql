CREATE TABLE public.models (
    tenant_id UUID NOT NULL,
    id UUID NOT NULL,
    name TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL CHECK (source IN ('upload','huggingface','modelscope','builtin')),
    source_repo_id TEXT NOT NULL DEFAULT '',
    capabilities JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL CHECK (status IN ('pending','downloading','ready','error','deleted')),
    error_message TEXT NOT NULL DEFAULT '',
    total_size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (total_size_bytes >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    deleted_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, name)
);

CREATE TABLE public.model_versions (
    tenant_id UUID NOT NULL,
    id UUID NOT NULL,
    model_id UUID NOT NULL,
    version TEXT NOT NULL,
    format TEXT NOT NULL CHECK (format IN ('safetensors','gguf','pytorch')),
    status TEXT NOT NULL CHECK (status IN ('pending','importing','ready','error','deleted')),
    is_encrypted BOOLEAN NOT NULL DEFAULT false,
    encrypt_algo TEXT NOT NULL DEFAULT '',
    encrypt_hint TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    checksum_sha256 TEXT NOT NULL DEFAULT '',
    engine_type TEXT NOT NULL DEFAULT '',
    startup_command TEXT NOT NULL DEFAULT '',
    startup_args JSONB NOT NULL DEFAULT '[]'::jsonb,
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, model_id, version),
    FOREIGN KEY (tenant_id, model_id) REFERENCES public.models(tenant_id, id)
);

CREATE TABLE public.model_artifacts (
    tenant_id UUID NOT NULL,
    id UUID NOT NULL,
    model_version_id UUID NOT NULL,
    provider TEXT NOT NULL,
    reference TEXT NOT NULL,
    format TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
    sha256 TEXT NOT NULL CHECK (sha256 ~ '^[a-fA-F0-9]{64}$'),
    is_encrypted BOOLEAN NOT NULL DEFAULT false,
    encrypt_algo TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, model_version_id),
    FOREIGN KEY (tenant_id, model_version_id) REFERENCES public.model_versions(tenant_id, id)
);

CREATE TABLE public.model_import_tasks (
    tenant_id UUID NOT NULL,
    id UUID NOT NULL,
    model_id UUID,
    model_version_id UUID,
    task_type TEXT NOT NULL CHECK (task_type IN ('upload','huggingface','modelscope')),
    source TEXT NOT NULL DEFAULT '',
    repo_id TEXT NOT NULL DEFAULT '',
    revision TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending','importing','completed','failed','cancelled')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    progress_pct INTEGER NOT NULL DEFAULT 0 CHECK (progress_pct BETWEEN 0 AND 100),
    lease_owner TEXT NOT NULL DEFAULT '',
    lease_epoch BIGINT NOT NULL DEFAULT 0 CHECK (lease_epoch >= 0),
    lease_until TIMESTAMPTZ,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, idempotency_key),
    FOREIGN KEY (tenant_id, model_id) REFERENCES public.models(tenant_id, id),
    FOREIGN KEY (tenant_id, model_version_id) REFERENCES public.model_versions(tenant_id, id)
);

CREATE TABLE public.audit_events (
    tenant_id UUID NOT NULL,
    id UUID NOT NULL,
    actor TEXT NOT NULL DEFAULT '',
    workload TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT '',
    task_id UUID,
    action TEXT NOT NULL,
    before_state TEXT NOT NULL DEFAULT '',
    after_state TEXT NOT NULL DEFAULT '',
    error_class TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, task_id) REFERENCES public.model_import_tasks(tenant_id, id)
);

CREATE INDEX model_versions_ready_idx ON public.model_versions (tenant_id, model_id, status);
CREATE INDEX model_import_due_idx ON public.model_import_tasks (next_attempt_at, lease_until);
