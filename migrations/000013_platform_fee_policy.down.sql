-- Never discard operator-authored versions on rollback: only the untouched
-- baseline head is removed. With the head gone billing fails closed.
DELETE FROM configuration_heads
WHERE application = 'billing' AND environment = 'platform'
  AND config_key = 'PLATFORM_FEE_POLICY' AND latest_version = 0;
