-- KEL-99: the singleton head is the explicit applied baseline of the platform
-- fee policy. Version 0 means 0 bps + Rp0: no operator version exists yet, so
-- every new transaction keeps today's zero platform fee until a new version is
-- acknowledged as applied. Billing refuses to invoice when this head is missing.
INSERT INTO configuration_heads (application, environment, config_key, latest_version)
VALUES ('billing', 'platform', 'PLATFORM_FEE_POLICY', 0)
ON CONFLICT (application, environment, config_key) DO NOTHING;
