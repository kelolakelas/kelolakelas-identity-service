package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// An existing account may only receive the system Teacher role. This is not
// the platform-approved Creator linking path or a parent conversion API.
func existingTeacher(tx *gorm.DB, userID, roleID uuid.UUID) error {
	var user domain.User
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&user, "id = ?", userID).Error; err != nil {
		return err
	}
	if user.IsParent {
		return domain.ErrExistingTeacherRequired
	}
	var role domain.Role
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&role, "id = ?", roleID).Error; err != nil {
		return err
	}
	if role.TenantID != nil || role.Name != "Teacher" {
		return domain.ErrExistingTeacherRequired
	}
	var choice domain.TenantChoice
	err := ownTenantChoices(tx, userID).Where("ro.tenant_id IS NULL AND ro.name = 'Teacher'").Clauses(clause.Locking{Strength: "SHARE"}).Take(&choice).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrExistingTeacherRequired
	}
	return err
}

func invitationTargetMember(tx *gorm.DB, tenantID, userID uuid.UUID) (*domain.TenantMember, error) {
	var member domain.TenantMember
	err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND user_id = ?", tenantID, userID).First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !member.IsActive || member.DeletedAt.Valid {
		return nil, domain.ErrInvitationMemberInactive
	}
	return &member, nil
}

func (r *invitationRepository) ValidateExistingTeacher(ctx context.Context, tenantID, roleID, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := existingTeacher(tx, userID, roleID); err != nil {
			return err
		}
		member, err := invitationTargetMember(tx, tenantID, userID)
		if err != nil {
			return err
		}
		if member != nil {
			return domain.ErrAlreadyTenantMember
		}
		return nil
	})
}

// AcceptExisting serializes with revoke/resend and locks live account and role
// facts through commit. The callback rechecks the signed session against the
// database's reset boundary while the user row is locked.
func (r *invitationRepository) AcceptExisting(ctx context.Context, token string, userID uuid.UUID, validate func(*domain.User) error) (*domain.TenantMember, error) {
	var result *domain.TenantMember
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Match ReplaceActive's tenant -> invitation lock order.
		var hint domain.TenantInvitation
		if err := tx.Where("token = ?", token).First(&hint).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrInvitationNotFound
			}
			return err
		}
		var tenant domain.Tenant
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ? AND status = 'active'", hint.TenantID).First(&tenant).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrTenantSessionForbidden
			}
			return err
		}
		var invitation domain.TenantInvitation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&invitation, "id = ?", hint.ID).Error; err != nil {
			return err
		}
		// UPDATE also serializes accepts of different invitations for one account.
		var user domain.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrTenantSessionInvalid
			}
			return err
		}
		if err := validate(&user); err != nil {
			return err
		}
		if domain.NormalizeEmail(user.Email) != domain.NormalizeEmail(invitation.Email) {
			return domain.ErrInvitationAccountMismatch
		}
		if err := existingTeacher(tx, userID, invitation.RoleID); err != nil {
			return err
		}
		member, err := invitationTargetMember(tx, invitation.TenantID, userID)
		if err != nil {
			return err
		}
		if err := validate(&user); err != nil {
			return err
		}
		if invitation.IsUsed {
			if invitation.AcceptedMemberID != nil && member != nil && *invitation.AcceptedMemberID == member.ID {
				result = member
				return nil
			}
			return domain.ErrInvitationUsed
		}
		if !time.Now().UTC().Before(invitation.ExpiresAt) {
			return domain.ErrInvitationExpired
		}
		if member == nil {
			member = &domain.TenantMember{ID: uuid.New(), TenantID: invitation.TenantID, UserID: user.ID, RoleID: invitation.RoleID, IsActive: true, JoinedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			if err := tx.Create(member).Error; err != nil {
				return err
			}
		}
		// An already active target membership is acknowledged without changing its
		// role. Inactive/deleted collisions were explicitly refused above.
		if err := tx.Model(&invitation).Updates(map[string]interface{}{"is_used": true, "accepted_member_id": member.ID, "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		result = member
		return nil
	})
	return result, err
}
