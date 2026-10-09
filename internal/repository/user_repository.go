package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
	user.Email = domain.NormalizeEmail(user.Email)
	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		if isUserEmailConflict(err) {
			return domain.ErrUserAlreadyExists
		}
		return err
	}
	return nil
}

// Unique indexes that make an account email unique. uq_users_email_lower is
// the case-insensitive rule (migration 000011, KEL-89); users_email_key is the
// original exact-match constraint from the initial schema.
var userEmailUniqueConstraints = map[string]bool{"uq_users_email_lower": true, "users_email_key": true}

// isUserEmailConflict reports whether a users insert lost the uniqueness race on
// email. The lookup before insert is only advisory: two concurrent
// registrations can both miss it, and the database index then decides.
func isUserEmailConflict(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && userEmailUniqueConstraints[pgErr.ConstraintName]
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
	// Legacy rows may be stored mixed-case; LOWER(email) matches them and is
	// served by the uq_users_email_lower expression index.
	if err := r.db.WithContext(ctx).First(&user, "LOWER(email) = ?", domain.NormalizeEmail(email)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) SessionValidAfter(ctx context.Context, id uuid.UUID) (*time.Time, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).Select("id", "session_valid_after").First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return user.SessionValidAfter, nil
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
		// Hold a shared lock on the policy head through commit. Applying a
		// version takes an exclusive lock on this same row; whichever lock
		// wins determines the ordering of close and registration. Read the
		// applied value on this transaction's connection, never a separate
		// connection that could observe a stale policy.
		open, err := registrationOpenInTx(tx)
		if err != nil {
			return domain.ErrRegistrationClosed
		}
		if !open {
			return domain.ErrRegistrationClosed
		}

		// 1. Create User
		user.Email = domain.NormalizeEmail(user.Email)
		if err := tx.Create(user).Error; err != nil {
			if isUserEmailConflict(err) {
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
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token = ?", token).First(&invitation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrInvitationNotFound
			}
			return err
		}

		if invitation.IsUsed {
			return domain.ErrInvitationUsed
		}

		if time.Now().UTC().After(invitation.ExpiresAt) {
			return domain.ErrInvitationExpired
		}

		// Legacy invitations may predate the Creator grant guard. Never redeem one
		// into a privileged membership, even when its token remains valid.
		var invitedRole domain.Role
		if err := tx.First(&invitedRole, "id = ?", invitation.RoleID).Error; err != nil {
			return err
		}
		// Invitations stored before KEL-89 may carry a mixed-case address; the
		// account is always matched on, and created with, the canonical form.
		invitedEmail := domain.NormalizeEmail(invitation.Email)
		if invitedRole.TenantID == nil && invitedRole.Name == "Creator" {
			// Only a platform-approved, email-bound request may redeem a Creator invitation.
			var approved int64
			if err := tx.Model(&domain.CreatorRequest{}).Where("invitation_id = ? AND tenant_id = ? AND LOWER(target_email) = ? AND status = ? AND target_user_id IS NULL", invitation.ID, invitation.TenantID, invitedEmail, "approved").Count(&approved).Error; err != nil {
				return err
			}
			if approved != 1 {
				return domain.ErrCreatorGrantForbidden
			}
			var tenant domain.Tenant
			if err := tx.Where("id = ? AND status = ? AND deleted_at IS NULL", invitation.TenantID, "active").First(&tenant).Error; err != nil {
				return domain.ErrCreatorRequestStale
			}
		}

		// Check if user with invitation email already exists
		var existingUser domain.User
		if err := tx.Where("LOWER(email) = ?", invitedEmail).First(&existingUser).Error; err == nil {
			return domain.ErrUserAlreadyExists
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// Hash password
		hashedPassword, err := hash.HashPassword(password)
		if err != nil {
			return err
		}

		// 2. Insert to Users table
		user = domain.User{
			ID:           uuid.New(),
			Email:        invitedEmail,
			PasswordHash: hashedPassword,
			FirstName:    firstName,
			LastName:     lastName,
			IsParent:     false,
		}
		if err := tx.Create(&user).Error; err != nil {
			if isUserEmailConflict(err) {
				return domain.ErrUserAlreadyExists
			}
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
		invitation.UpdatedAt = time.Now().UTC()
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
