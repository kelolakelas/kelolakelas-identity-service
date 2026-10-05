-- KEL-152: manual refund is restricted to the system Creator by default.
INSERT INTO permissions (id, name, description)
VALUES (gen_random_uuid(), 'billing:refund', 'Mencatat refund manual transaksi paid')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r
JOIN permissions p ON p.name = 'billing:refund'
WHERE r.name = 'Creator' AND r.tenant_id IS NULL
ON CONFLICT DO NOTHING;
