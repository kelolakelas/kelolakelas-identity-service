-- KEL-98: the singleton head is the durable default-open public catalog
-- policy. Version 0 means no operator has changed it yet, so the catalog keeps
-- its current behaviour until a closing version is acknowledged as applied.
INSERT INTO configuration_heads (application, environment, config_key, latest_version)
VALUES ('academic', 'platform', 'PUBLIC_CATALOG_OPEN', 0)
ON CONFLICT (application, environment, config_key) DO NOTHING;
