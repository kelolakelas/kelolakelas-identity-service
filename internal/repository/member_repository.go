package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type memberRepository struct {
	db *gorm.DB
}

func (r *memberRepository) GetByID(ctx context.Context, tenantID, memberID uuid.UUID) (*domain.MemberResponse, error) {
	var row memberRow
	query := r.db.WithContext(ctx).Table("tenant_members tm").
		Joins("JOIN users u ON u.id = tm.user_id").Joins("JOIN roles ro ON ro.id = tm.role_id").
		Where("tm.tenant_id = ? AND tm.id = ?", tenantID, memberID).
		Select("tm.id, tm.user_id, tm.tenant_id, u.email, u.first_name, u.last_name, u.phone, CASE WHEN tm.is_active THEN 'active' ELSE 'inactive' END AS status, ro.id AS role_id, ro.name AS role_name, (ro.tenant_id IS NULL) AS is_system_role, tm.joined_at AS created_at, tm.updated_at").First(&row)
	if errors.Is(query.Error, gorm.ErrRecordNotFound) {
		return nil, domain.ErrMemberNotFound
	}
	if query.Error != nil {
		return nil, query.Error
	}
	return &domain.MemberResponse{ID: row.ID, UserID: row.UserID, TenantID: row.TenantID, Email: row.Email, FirstName: row.FirstName, LastName: row.LastName, Phone: row.Phone, Status: row.Status, Role: domain.MemberRoleResponse{ID: row.RoleID, Name: row.RoleName, IsSystemRole: row.IsSystemRole}, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

func (r *memberRepository) UpdateRole(ctx context.Context, tenantID, memberID, roleID uuid.UUID) (*domain.MemberResponse, error) {
	var member domain.TenantMember
	var role domain.Role
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND tenant_id = ?", memberID, tenantID).First(&member).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrMemberNotFound
			}
			return err
		}
		if err := tx.Where("id = ? AND (tenant_id = ? OR tenant_id IS NULL)", roleID, tenantID).First(&role).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrMemberRoleConflict
			}
			return err
		}
		var current domain.Role
		if err := tx.First(&current, "id = ?", member.RoleID).Error; err != nil {
			return err
		}
		if current.TenantID == nil {
			return domain.ErrMemberRoleForbidden
		}
		return tx.Model(&member).Update("role_id", roleID).Error
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, tenantID, memberID)
}

func (r *memberRepository) HasPermission(ctx context.Context, roleID uuid.UUID, permission string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("role_permissions rp").Joins("JOIN permissions p ON p.id = rp.permission_id").Where("rp.role_id = ? AND p.name = ?", roleID, permission).Count(&count).Error
	return count > 0, err
}

func (r *memberRepository) ListTutors(ctx context.Context, tenantID uuid.UUID, query domain.TutorQuery) ([]domain.TutorResponse, int64, error) {
	db := r.db.WithContext(ctx).Table("tenant_members tm").
		Joins("JOIN users u ON u.id = tm.user_id").Joins("JOIN roles ro ON ro.id = tm.role_id").
		Where("tm.tenant_id = ? AND tm.is_active = ?", tenantID, true).
		Where("LOWER(ro.name) IN ?", []string{"teacher", "tutor", "pengajar"})
	if query.Status == "inactive" {
		db = db.Where("1 = 0")
	}
	if query.Search != "" {
		like := "%" + query.Search + "%"
		db = db.Where("u.email ILIKE ? OR u.first_name ILIKE ? OR u.last_name ILIKE ?", like, like, like)
	}
	var total int64
	if err := db.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []struct {
		UserID    uuid.UUID `gorm:"column:user_id"`
		FirstName string    `gorm:"column:first_name"`
		LastName  string    `gorm:"column:last_name"`
		Email     string    `gorm:"column:email"`
	}
	err := db.Select("u.id AS user_id, u.first_name, u.last_name, u.email").Order("u.last_name ASC, u.first_name ASC").Limit(query.PageSize).Offset((query.Page - 1) * query.PageSize).Scan(&rows).Error
	items := make([]domain.TutorResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.TutorResponse{ID: row.UserID, FirstName: row.FirstName, LastName: row.LastName, Email: row.Email, Status: "active"})
	}
	return items, total, err
}

func NewMemberRepository(db *gorm.DB) MemberRepository {
	return &memberRepository{db: db}
}

type memberRow struct {
	ID           uuid.UUID `gorm:"column:id"`
	UserID       uuid.UUID `gorm:"column:user_id"`
	TenantID     uuid.UUID `gorm:"column:tenant_id"`
	Email        string    `gorm:"column:email"`
	FirstName    string    `gorm:"column:first_name"`
	LastName     string    `gorm:"column:last_name"`
	Phone        *string   `gorm:"column:phone"`
	Status       string    `gorm:"column:status"`
	RoleID       uuid.UUID `gorm:"column:role_id"`
	RoleName     string    `gorm:"column:role_name"`
	IsSystemRole bool      `gorm:"column:is_system_role"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (r *memberRepository) List(ctx context.Context, tenantID uuid.UUID, query domain.MemberQuery) ([]domain.MemberResponse, int64, error) {
	base := r.db.WithContext(ctx).Table("tenant_members tm").
		Joins("JOIN users u ON u.id = tm.user_id").
		Joins("JOIN roles ro ON ro.id = tm.role_id").
		Where("tm.tenant_id = ?", tenantID)

	if query.Search != "" {
		like := "%" + query.Search + "%"
		base = base.Where("u.email ILIKE ? OR u.first_name ILIKE ? OR u.last_name ILIKE ?", like, like, like)
	}
	if query.Status == "active" {
		base = base.Where("tm.is_active = ?", true)
	} else if query.Status == "inactive" {
		base = base.Where("tm.is_active = ?", false)
	}
	if query.RoleID != nil {
		base = base.Where("tm.role_id = ?", *query.RoleID)
	}

	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	orderColumn := map[string]string{"created_at": "tm.created_at", "updated_at": "tm.updated_at", "email": "u.email", "name": "u.last_name"}[query.Sort]
	if orderColumn == "" {
		orderColumn = "tm.created_at"
	}
	orderDirection := "ASC"
	if query.Order == "desc" {
		orderDirection = "DESC"
	}

	var rows []memberRow
	if err := base.Select("tm.id, tm.user_id, tm.tenant_id, u.email, u.first_name, u.last_name, u.phone, CASE WHEN tm.is_active THEN 'active' ELSE 'inactive' END AS status, ro.id AS role_id, ro.name AS role_name, (ro.tenant_id IS NULL) AS is_system_role, tm.joined_at AS created_at, tm.updated_at").
		Order(fmt.Sprintf("%s %s", orderColumn, orderDirection)).
		Limit(query.PageSize).Offset((query.Page - 1) * query.PageSize).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	roleIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		roleIDs = append(roleIDs, row.RoleID)
	}
	permissionsByRole := make(map[uuid.UUID][]domain.PermissionResponse)
	if len(roleIDs) > 0 {
		var permissions []struct {
			RoleID      uuid.UUID `gorm:"column:role_id"`
			ID          uuid.UUID `gorm:"column:id"`
			Name        string    `gorm:"column:name"`
			Description string    `gorm:"column:description"`
		}
		if err := r.db.WithContext(ctx).Table("role_permissions rp").
			Select("rp.role_id, p.id, p.name, p.description").
			Joins("JOIN permissions p ON p.id = rp.permission_id").
			Where("rp.role_id IN ?", roleIDs).Scan(&permissions).Error; err != nil {
			return nil, 0, err
		}
		for _, permission := range permissions {
			permissionsByRole[permission.RoleID] = append(permissionsByRole[permission.RoleID], domain.PermissionResponse{
				ID: permission.ID, Name: permission.Name, Description: permission.Description,
			})
		}
	}

	items := make([]domain.MemberResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.MemberResponse{
			ID: row.ID, UserID: row.UserID, TenantID: row.TenantID, Email: row.Email,
			FirstName: row.FirstName, LastName: row.LastName, Phone: row.Phone, Status: row.Status,
			Role:      domain.MemberRoleResponse{ID: row.RoleID, Name: row.RoleName, IsSystemRole: row.IsSystemRole, Permissions: permissionsByRole[row.RoleID]},
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
	}
	return items, total, nil
}
