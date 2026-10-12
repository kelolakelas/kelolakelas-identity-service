DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE name = 'category:update');
DELETE FROM permissions WHERE name = 'category:update';
