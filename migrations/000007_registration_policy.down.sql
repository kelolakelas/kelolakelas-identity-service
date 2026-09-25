-- Never discard operator-authored versions on rollback.
DELETE FROM configuration_heads
WHERE application = 'identity' AND environment = 'platform'
  AND config_key = 'TENANT_REGISTRATION_OPEN' AND latest_version = 0;
