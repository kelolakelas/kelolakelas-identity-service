DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE name = 'billing:refund');
DELETE FROM permissions WHERE name = 'billing:refund';
