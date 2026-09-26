-- Never discard operator-authored versions on rollback.
DELETE FROM configuration_heads
WHERE application = 'academic' AND environment = 'platform'
  AND config_key = 'PUBLIC_CATALOG_OPEN' AND latest_version = 0;
