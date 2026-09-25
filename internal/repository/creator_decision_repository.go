package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CreatorDecisionRepository interface {
	Decide(ctx context.Context, requestID, actorID uuid.UUID, approve bool, reason string) (*domain.CreatorRequest, *domain.TenantInvitation, string, error)
}

type creatorDecisionRepository struct{ db *gorm.DB }

func NewCreatorDecisionRepository(db *gorm.DB) CreatorDecisionRepository {
	return &creatorDecisionRepository{db: db}
}

func (r *creatorDecisionRepository) Decide(ctx context.Context, requestID, actorID uuid.UUID, approve bool, reason string) (*domain.CreatorRequest, *domain.TenantInvitation, string, error) {
	var request domain.CreatorRequest
	var invitation *domain.TenantInvitation
	var tenantName string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock the live admin assignment for the whole decision; revocation cannot race the grant.
		var adminID string
		if err := tx.Raw(`SELECT id FROM platform_admin_assignments WHERE user_id = ? AND is_active = true FOR SHARE`, actorID).Scan(&adminID).Error; err != nil {
			return err
		}
		if adminID == "" {
			return domain.ErrCreatorRequestForbidden
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", requestID).First(&request).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrCreatorRequestNotFound
			}
			return err
		}
		if request.Status != "pending" {
			return domain.ErrCreatorRequestDecided
		}
		var tenant domain.Tenant
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ? AND status = ? AND deleted_at IS NULL", request.TenantID, "active").First(&tenant).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrCreatorRequestStale
			}
			return err
		}
		tenantName = tenant.Name
		now := time.Now().UTC()
		if approve {
			// Re-resolve the email under lock: a target ID or account appearing after submission must never redirect a grant.
			var user domain.User
			lookup := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("LOWER(email) = ? AND deleted_at IS NULL", request.TargetEmail).First(&user).Error
			if lookup != nil && !errors.Is(lookup, gorm.ErrRecordNotFound) {
				return lookup
			}
			if request.TargetUserID != nil {
				if lookup != nil || user.ID != *request.TargetUserID {
					return domain.ErrCreatorRequestStale
				}
			}
			if lookup == nil {
				if request.TargetUserID == nil {
					return domain.ErrCreatorRequestStale
				}
				if user.IsParent {
					return domain.ErrCreatorRequestStale
				}
				var role domain.Role
				if err := tx.Where("name = ? AND tenant_id IS NULL", "Creator").First(&role).Error; err != nil {
					return err
				}
				var member domain.TenantMember
				err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND user_id = ?", request.TenantID, user.ID).First(&member).Error
				switch {
				case errors.Is(err, gorm.ErrRecordNotFound):
					member = domain.TenantMember{ID: uuid.New(), TenantID: request.TenantID, UserID: user.ID, RoleID: role.ID, IsActive: true}
					if err := tx.Create(&member).Error; err != nil {
						return err
					}
				case err != nil:
					return err
				case member.DeletedAt.Valid || !member.IsActive:
					return domain.ErrCreatorRequestStale
				default:
					if member.RoleID == role.ID {
						return domain.ErrCreatorTargetAlreadyCreator
					}
					if err := tx.Model(&member).Update("role_id", role.ID).Error; err != nil {
						return err
					}
				}
				request.TargetUserID = &user.ID
			} else {
				if request.TargetUserID != nil {
					return domain.ErrCreatorRequestStale
				}
				var role domain.Role
				if err := tx.Where("name = ? AND tenant_id IS NULL", "Creator").First(&role).Error; err != nil {
					return err
				}
				inv := &domain.TenantInvitation{ID: uuid.New(), TenantID: request.TenantID, RoleID: role.ID, Email: request.TargetEmail, Token: uuid.NewString(), ExpiresAt: now.Add(48 * time.Hour), CreatedAt: now, UpdatedAt: now}
				if err := tx.Create(inv).Error; err != nil {
					return err
				}
				request.InvitationID = &inv.ID
				invitation = inv
			}
			request.Status = "approved"
		} else {
			if strings.TrimSpace(reason) == "" || len(reason) > 2000 {
				return domain.ErrCreatorRequestInvalid
			}
			reason = strings.TrimSpace(reason)
			request.Status = "rejected"
			request.RejectionReason = &reason
		}
		request.DecidedBy = &actorID
		request.DecidedAt = &now
		request.UpdatedAt = now
		if err := tx.Save(&request).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO creator_request_audit (id, request_id, tenant_id, actor_user_id, decision, rejection_reason, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, uuid.New(), request.ID, request.TenantID, actorID, request.Status, request.RejectionReason, now).Error
	})
	if err != nil {
		return nil, nil, "", err
	}
	return &request, invitation, tenantName, nil
}
