DROP TABLE platform_factor_challenges;
ALTER TABLE platform_admin_assignments DROP COLUMN factor_secret,
    DROP COLUMN enrollment_allowed, DROP COLUMN factor_version;
