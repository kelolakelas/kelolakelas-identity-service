package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/hash"
)

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) domain.UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(ctx context.Context, user *domain.User) error {
	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return domain.ErrUserAlreadyExists
		}
		// In case GORM driver doesn't return ErrDuplicatedKey directly, we can also check for uniqueness
		return err
	}
	return nil
}

func (r *userRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).First(&user, "email = ?", email).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) Update(ctx context.Context, user *domain.User) error {
	if err := r.db.WithContext(ctx).Save(user).Error; err != nil {
		return err
	}
	return nil
}

func (r *userRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.db.WithContext(ctx).Delete(&domain.User{}, "id = ?", id).Error; err != nil {
		return err
	}
	return nil
}

func (r *userRepository) RegisterTenantTx(ctx context.Context, user *domain.User, tenant *domain.Tenant) (*domain.TenantMember, error) {
	var member domain.TenantMember

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Create User
		if err := tx.Create(user).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return domain.ErrUserAlreadyExists
			}
			return err
		}

		// 2. Create Tenant
		if err := tx.Create(tenant).Error; err != nil {
			return err
		}

		// 2b. Create Tenant Wallet
		tenantWallet := domain.TenantWallet{
			ID:               uuid.New(),
			TenantID:         tenant.ID,
			AvailableBalance: 0,
			PendingBalance:   0,
		}
		if err := tx.Create(&tenantWallet).Error; err != nil {
			return err
		}

		// 2c. Create User Wallet
		userWallet := domain.UserWallet{
			ID:      uuid.New(),
			UserID:  user.ID,
			Balance: 0,
		}
		if err := tx.Create(&userWallet).Error; err != nil {
			return err
		}

		// 3. Find Role with name 'Creator' (system default, where tenant_id is NULL)
		var role domain.Role
		if err := tx.Where("name = ? AND tenant_id IS NULL", "Creator").First(&role).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Fallback to create system-level Creator role if not found
				role = domain.Role{
					ID:          uuid.New(),
					TenantID:    nil,
					Name:        "Creator",
					Description: "System Default Creator Role",
				}
				if err := tx.Create(&role).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		}

		// 4. Create TenantMember connecting user, tenant, and role
		member = domain.TenantMember{
			ID:       uuid.New(),
			TenantID: tenant.ID,
			UserID:   user.ID,
			RoleID:   role.ID,
			IsActive: true,
		}
		if err := tx.Create(&member).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &member, nil
}

func (r *userRepository) GetPermissionsByRoleId(ctx context.Context, roleID uuid.UUID) ([]string, error) {
	var permissions []string

	err := r.db.WithContext(ctx).
		Table("role_permissions").
		Select("permissions.name").
		Joins("join permissions on role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", roleID).
		Pluck("permissions.name", &permissions).
		Error

	if err != nil {
		return nil, err
	}

	return permissions, nil
}

func (r *userRepository) GetTenantMemberByUserID(ctx context.Context, userID uuid.UUID) (*domain.TenantMember, error) {
	var member domain.TenantMember
	if err := r.db.WithContext(ctx).Where("user_id = ? AND is_active = ?", userID, true).First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &member, nil
}

func (r *userRepository) RegisterInvitedUserTx(ctx context.Context, token, firstName, lastName, password string) (*domain.User, error) {
	var user domain.User

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Fetch & validate invitation token
		var invitation domain.TenantInvitation
		if err := tx.Where("token = ?", token).First(&invitation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrInvitationNotFound
			}
			return err
		}

		if invitation.IsUsed {
			return domain.ErrInvitationUsed
		}

		if time.Now().After(invitation.ExpiresAt) {
			return domain.ErrInvitationExpired
		}

		// Check if user with invitation email already exists
		var existingUser domain.User
		if err := tx.Where("email = ?", invitation.Email).First(&existingUser).Error; err == nil {
			return domain.ErrUserAlreadyExists
		}

		// Hash password
		hashedPassword, err := hash.HashPassword(password)
		if err != nil {
			return err
		}

		// 2. Insert to Users table
		user = domain.User{
			ID:           uuid.New(),
			Email:        invitation.Email,
			PasswordHash: hashedPassword,
			FirstName:    firstName,
			LastName:     lastName,
			IsParent:     false,
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}

		// 3. Insert to Tenant_Members table
		member := domain.TenantMember{
			ID:       uuid.New(),
			TenantID: invitation.TenantID,
			UserID:   user.ID,
			RoleID:   invitation.RoleID,
			IsActive: true,
		}
		if err := tx.Create(&member).Error; err != nil {
			return err
		}

		// 4. Insert to User_Wallets table (initial balance 0)
		wallet := domain.UserWallet{
			ID:      uuid.New(),
			UserID:  user.ID,
			Balance: 0,
		}
		if err := tx.Create(&wallet).Error; err != nil {
			return err
		}

		// 5. Update Tenant_Invitations table set is_used = true
		invitation.IsUsed = true
		invitation.UpdatedAt = time.Now()
		if err := tx.Save(&invitation).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &user, nil
}
