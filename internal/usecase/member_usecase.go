package usecase

import (
	"context"
	"math"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/repository"
)

type memberUsecase struct {
	repo repository.MemberRepository
}

func NewMemberUsecase(repo repository.MemberRepository) domain.MemberUsecase {
	return &memberUsecase{repo: repo}
}

func (u *memberUsecase) List(ctx context.Context, tenantID uuid.UUID, query domain.MemberQuery) (*domain.MemberListResponse, error) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		query.PageSize = 20
	}
	items, total, err := u.repo.List(ctx, tenantID, query)
	if err != nil {
		return nil, err
	}
	return &domain.MemberListResponse{Items: items, Pagination: domain.Pagination{
		Page: query.Page, PageSize: query.PageSize, TotalItems: total,
		TotalPages: int(math.Ceil(float64(total) / float64(query.PageSize))),
	}}, nil
}

func (u *memberUsecase) GetByID(ctx context.Context, tenantID, memberID uuid.UUID) (*domain.MemberResponse, error) {
	return u.repo.GetByID(ctx, tenantID, memberID)
}

func (u *memberUsecase) UpdateRole(ctx context.Context, tenantID, callerRoleID, memberID, roleID uuid.UUID) (*domain.MemberResponse, error) {
	allowed, err := u.repo.HasPermission(ctx, callerRoleID, "member:update")
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrMemberPermission
	}
	return u.repo.UpdateRole(ctx, tenantID, memberID, roleID)
}

func (u *memberUsecase) ListTutors(ctx context.Context, tenantID uuid.UUID, query domain.TutorQuery) (*domain.TutorListResponse, error) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		query.PageSize = 20
	}
	items, total, err := u.repo.ListTutors(ctx, tenantID, query)
	if err != nil {
		return nil, err
	}
	return &domain.TutorListResponse{Items: items, Pagination: domain.Pagination{Page: query.Page, PageSize: query.PageSize, TotalItems: total, TotalPages: int(math.Ceil(float64(total) / float64(query.PageSize)))}}, nil
}
