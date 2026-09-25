CREATE TABLE configuration_heads (
    application varchar(64) NOT NULL,
    environment varchar(64) NOT NULL,
    config_key varchar(255) NOT NULL,
    latest_version bigint NOT NULL DEFAULT 0 CHECK (latest_version >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (application, environment, config_key)
);

CREATE TABLE configuration_versions (
    id bigserial PRIMARY KEY,
    application varchar(64) NOT NULL,
    environment varchar(64) NOT NULL,
    config_key varchar(255) NOT NULL,
    version bigint NOT NULL CHECK (version > 0),
    value jsonb NOT NULL,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    rollback_of_version_id bigint REFERENCES configuration_versions(id),
    UNIQUE (application, environment, config_key, version),
    CHECK (jsonb_typeof(value) IN ('string', 'number', 'boolean'))
);
CREATE INDEX idx_configuration_versions_scope_created
    ON configuration_versions (application, environment, config_key, created_at DESC);

CREATE TABLE configuration_reports (
    id bigserial PRIMARY KEY,
    version_id bigint NOT NULL REFERENCES configuration_versions(id),
    status varchar(16) NOT NULL CHECK (status IN ('applied', 'failed', 'rollback')),
    reported_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_configuration_reports_version_created
    ON configuration_reports (version_id, created_at DESC);
