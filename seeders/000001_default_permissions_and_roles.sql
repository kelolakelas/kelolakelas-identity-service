INSERT INTO permissions (id, name, description)
VALUES
    (gen_random_uuid(), 'tenant:read', 'Melihat informasi profil bimbel/tenant'),
    (gen_random_uuid(), 'tenant:update', 'Mengubah informasi profil bimbel/tenant'),
    (gen_random_uuid(), 'member:invite', 'Mengundang anggota tim atau pengajar baru'),
    (gen_random_uuid(), 'member:read', 'Melihat daftar anggota tim/pengajar bimbel'),
    (gen_random_uuid(), 'member:update', 'Mengubah peran atau status anggota tim'),
    (gen_random_uuid(), 'member:delete', 'Menghapus anggota tim dari bimbel'),
    (gen_random_uuid(), 'role:create', 'Membuat role kustom baru'),
    (gen_random_uuid(), 'role:read', 'Melihat daftar role dan hak akses'),
    (gen_random_uuid(), 'role:update', 'Mengubah role kustom dan hak akses'),
    (gen_random_uuid(), 'role:delete', 'Menghapus role kustom'),
    (gen_random_uuid(), 'category:create', 'Membuat kategori mata pelajaran baru'),
    (gen_random_uuid(), 'category:read', 'Melihat daftar dan detail kategori'),
    (gen_random_uuid(), 'category:update', 'Mengubah informasi kategori mata pelajaran'),
    (gen_random_uuid(), 'category:delete', 'Menghapus kategori mata pelajaran'),
    (gen_random_uuid(), 'class:create', 'Membuat kelas baru'),
    (gen_random_uuid(), 'class:read', 'Melihat daftar dan detail kelas'),
    (gen_random_uuid(), 'class:update', 'Mengubah informasi kelas'),
    (gen_random_uuid(), 'class:delete', 'Menghapus kelas'),
    (gen_random_uuid(), 'schedule:create', 'Membuat jadwal pelajaran kelas'),
    (gen_random_uuid(), 'schedule:read', 'Melihat jadwal pelajaran kelas'),
    (gen_random_uuid(), 'schedule:update', 'Mengubah jadwal pelajaran kelas'),
    (gen_random_uuid(), 'schedule:delete', 'Menghapus jadwal pelajaran kelas'),
    (gen_random_uuid(), 'student:create', 'Mendaftarkan data siswa baru'),
    (gen_random_uuid(), 'student:read', 'Melihat daftar dan profil siswa'),
    (gen_random_uuid(), 'student:update', 'Mengubah data profil siswa'),
    (gen_random_uuid(), 'student:delete', 'Menghapus data siswa'),
    (gen_random_uuid(), 'enrollment:create', 'Mendaftarkan siswa ke kelas'),
    (gen_random_uuid(), 'enrollment:read', 'Melihat daftar pendaftaran kelas'),
    (gen_random_uuid(), 'enrollment:update', 'Mengubah status pendaftaran siswa'),
    (gen_random_uuid(), 'enrollment:delete', 'Membatalkan pendaftaran kelas siswa'),
    (gen_random_uuid(), 'attendance:create', 'Mencatat presensi/kehadiran siswa'),
    (gen_random_uuid(), 'attendance:read', 'Melihat rekap presensi/kehadiran siswa'),
    (gen_random_uuid(), 'attendance:update', 'Mengubah catatan presensi/kehadiran siswa'),
    (gen_random_uuid(), 'student_note:create', 'Membuat catatan siswa (akademik/perilaku/medis)'),
    (gen_random_uuid(), 'student_note:read', 'Melihat catatan siswa'),
    (gen_random_uuid(), 'student_note:update', 'Mengubah catatan siswa'),
    (gen_random_uuid(), 'student_note:delete', 'Menghapus catatan siswa'),
    (gen_random_uuid(), 'report:create', 'Membuat laporan evaluasi/nilai siswa'),
    (gen_random_uuid(), 'report:read', 'Melihat laporan evaluasi/nilai siswa'),
    (gen_random_uuid(), 'report:update', 'Mengubah laporan evaluasi/nilai siswa'),
    (gen_random_uuid(), 'report:delete', 'Menghapus laporan evaluasi/nilai siswa'),
    (gen_random_uuid(), 'billing:read', 'Melihat saldo wallet dan riwayat transaksi/mutasi'),
    (gen_random_uuid(), 'billing:withdraw', 'Melakukan penarikan saldo/dana bimbel'),
    (gen_random_uuid(), 'voucher:create', 'Membuat kode voucher diskon baru'),
    (gen_random_uuid(), 'voucher:read', 'Melihat daftar voucher diskon'),
    (gen_random_uuid(), 'voucher:update', 'Mengubah informasi/status voucher diskon'),
    (gen_random_uuid(), 'voucher:delete', 'Menghapus voucher diskon')
ON CONFLICT (name) DO UPDATE
SET description = EXCLUDED.description;

INSERT INTO roles (id, tenant_id, name, description)
SELECT gen_random_uuid(), NULL, 'Creator', 'System Default Creator Role (Full Access)'
WHERE NOT EXISTS (
    SELECT 1 FROM roles WHERE name = 'Creator' AND tenant_id IS NULL
);

INSERT INTO roles (id, tenant_id, name, description)
SELECT gen_random_uuid(), NULL, 'Teacher', 'System Default Teacher Role'
WHERE NOT EXISTS (
    SELECT 1 FROM roles WHERE name = 'Teacher' AND tenant_id IS NULL
);

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'Creator' AND r.tenant_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'Teacher'
  AND r.tenant_id IS NULL
  AND split_part(p.name, ':', 1) IN ('schedule', 'attendance', 'student_note', 'report')
ON CONFLICT DO NOTHING;