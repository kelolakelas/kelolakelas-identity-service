-- KEL-97: the singleton head is the durable default-open policy. Version 0
-- means no operator has changed the policy yet; no synthetic user/report is
-- needed, so bootstrap and the first tenant remain independent.
INSERT INTO configuration_heads (application, environment, config_key, latest_version)
VALUES ('identity', 'platform', 'TENANT_REGISTRATION_OPEN', 0)
ON CONFLICT (application, environment, config_key) DO NOTHING;
