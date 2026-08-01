package database

import (
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type PermissionSeed struct {
	Name        string
	Description string
}

func GetDefaultPermissions() []PermissionSeed {
	return []PermissionSeed{
		// Tenant & Member Management
		{Name: "tenant:read", Description: "Melihat informasi profil bimbel/tenant"},
		{Name: "tenant:update", Description: "Mengubah informasi profil bimbel/tenant"},
		{Name: "member:invite", Description: "Mengundang anggota tim atau pengajar baru"},
		{Name: "member:read", Description: "Melihat daftar anggota tim/pengajar bimbel"},
		{Name: "member:update", Description: "Mengubah peran atau status anggota tim"},
		{Name: "member:delete", Description: "Menghapus anggota tim dari bimbel"},

		// Role & Permission Management
		{Name: "role:create", Description: "Membuat role kustom baru"},
		{Name: "role:read", Description: "Melihat daftar role dan hak akses"},
		{Name: "role:update", Description: "Mengubah role kustom dan hak akses"},
		{Name: "role:delete", Description: "Menghapus role kustom"},

		// Category Management
		{Name: "category:create", Description: "Membuat kategori mata pelajaran baru"},
		{Name: "category:read", Description: "Melihat daftar dan detail kategori"},
		{Name: "category:update", Description: "Mengubah informasi kategori mata pelajaran"},
		{Name: "category:delete", Description: "Menghapus kategori mata pelajaran"},

		// Class Management
		{Name: "class:create", Description: "Membuat kelas baru"},
		{Name: "class:read", Description: "Melihat daftar dan detail kelas"},
		{Name: "class:update", Description: "Mengubah informasi kelas"},
		{Name: "class:delete", Description: "Menghapus kelas"},

		// Schedule Management
		{Name: "schedule:create", Description: "Membuat jadwal pelajaran kelas"},
		{Name: "schedule:read", Description: "Melihat jadwal pelajaran kelas"},
		{Name: "schedule:update", Description: "Mengubah jadwal pelajaran kelas"},
		{Name: "schedule:delete", Description: "Menghapus jadwal pelajaran kelas"},

		// Student Management
		{Name: "student:create", Description: "Mendaftarkan data siswa baru"},
		{Name: "student:read", Description: "Melihat daftar dan profil siswa"},
		{Name: "student:update", Description: "Mengubah data profil siswa"},
		{Name: "student:delete", Description: "Menghapus data siswa"},

		// Enrollment Management
		{Name: "enrollment:create", Description: "Mendaftarkan siswa ke kelas"},
		{Name: "enrollment:read", Description: "Melihat daftar pendaftaran kelas"},
		{Name: "enrollment:update", Description: "Mengubah status pendaftaran siswa"},
		{Name: "enrollment:delete", Description: "Membatalkan pendaftaran kelas siswa"},

		// Attendance Management
		{Name: "attendance:create", Description: "Mencatat presensi/kehadiran siswa"},
		{Name: "attendance:read", Description: "Melihat rekap presensi/kehadiran siswa"},
		{Name: "attendance:update", Description: "Mengubah catatan presensi/kehadiran siswa"},

		// Student Notes Management
		{Name: "student_note:create", Description: "Membuat catatan siswa (akademik/perilaku/medis)"},
		{Name: "student_note:read", Description: "Melihat catatan siswa"},
		{Name: "student_note:update", Description: "Mengubah catatan siswa"},
		{Name: "student_note:delete", Description: "Menghapus catatan siswa"},

		// Report Management
		{Name: "report:create", Description: "Membuat laporan evaluasi/nilai siswa"},
		{Name: "report:read", Description: "Melihat laporan evaluasi/nilai siswa"},
		{Name: "report:update", Description: "Mengubah laporan evaluasi/nilai siswa"},
		{Name: "report:delete", Description: "Menghapus laporan evaluasi/nilai siswa"},

		// Billing & Wallet Management
		{Name: "billing:read", Description: "Melihat saldo wallet dan riwayat transaksi/mutasi"},
		{Name: "billing:withdraw", Description: "Melakukan penarikan saldo/dana bimbel"},

		// Voucher Management
		{Name: "voucher:create", Description: "Membuat kode voucher diskon baru"},
		{Name: "voucher:read", Description: "Melihat daftar voucher diskon"},
		{Name: "voucher:update", Description: "Mengubah informasi/status voucher diskon"},
		{Name: "voucher:delete", Description: "Menghapus voucher diskon"},
	}
}

func SeedPermissions(db *gorm.DB) error {
	slog.Info("Seeding permissions...")

	seeds := GetDefaultPermissions()
	count := 0

	for _, seed := range seeds {
		var perm domain.Permission
		err := db.Where("name = ?", seed.Name).First(&perm).Error
		if err == gorm.ErrRecordNotFound {
			perm = domain.Permission{
				ID:          uuid.New(),
				Name:        seed.Name,
				Description: seed.Description,
			}
			if err := db.Create(&perm).Error; err != nil {
				return fmt.Errorf("failed to seed permission %s: %w", seed.Name, err)
			}
			count++
		} else if err != nil {
			return fmt.Errorf("error checking permission %s: %w", seed.Name, err)
		} else {
			if perm.Description != seed.Description {
				db.Model(&perm).Update("description", seed.Description)
			}
		}
	}

	slog.Info("Permissions seeded successfully", "new_permissions_added", count, "total_defined", len(seeds))
	return nil
}

func SeedSystemRoles(db *gorm.DB) error {
	slog.Info("Seeding system default roles...")

	var adminRole domain.Role
	err := db.Where("name = ? AND tenant_id IS NULL", "Admin").First(&adminRole).Error
	if err == gorm.ErrRecordNotFound {
		adminRole = domain.Role{
			ID:          uuid.New(),
			TenantID:    nil,
			Name:        "Creator",
			Description: "System Default Creator Role (Full Access)",
		}
		if err := db.Create(&adminRole).Error; err != nil {
			return fmt.Errorf("failed to seed Admin system role: %w", err)
		}
		slog.Info("Created system default Admin role")
	} else if err != nil {
		return fmt.Errorf("error checking Admin system role: %w", err)
	}

	var allPermissions []domain.Permission
	if err := db.Find(&allPermissions).Error; err != nil {
		return fmt.Errorf("failed to fetch permissions for Admin role assignment: %w", err)
	}

	for _, perm := range allPermissions {
		rp := domain.RolePermission{
			RoleID:       adminRole.ID,
			PermissionID: perm.ID,
		}
		db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rp)
	}

	slog.Info("System default roles seeded successfully")
	return nil
}

func SeedAll(db *gorm.DB) error {
	if err := SeedPermissions(db); err != nil {
		return err
	}
	if err := SeedSystemRoles(db); err != nil {
		return err
	}
	return nil
}
