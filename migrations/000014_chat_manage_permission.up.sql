-- KEL-117: expose chat management in the tenant permission catalog and grant it
-- only to the system Creator role. A database without seeded roles still gets
-- the catalog entry; seeding later supplies the Creator grant.
INSERT INTO permissions (id, name, description)
VALUES (gen_random_uuid(), 'chat:manage', 'Menangani chat tenant dengan orang tua dan pengajar')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.name = 'chat:manage'
WHERE r.name = 'Creator' AND r.tenant_id IS NULL
ON CONFLICT DO NOTHING;
