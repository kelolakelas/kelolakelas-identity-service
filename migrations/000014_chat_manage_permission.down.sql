-- Revoke the migration's system grant first. Do not silently revoke custom
-- role grants: if one exists the FK prevents deleting the catalog entry.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE name = 'chat:manage')
  AND role_id IN (SELECT id FROM roles WHERE name = 'Creator' AND tenant_id IS NULL);

DELETE FROM permissions WHERE name = 'chat:manage';
