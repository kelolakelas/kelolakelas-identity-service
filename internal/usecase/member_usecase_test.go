package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type memberRepositoryStub struct {
	items []domain.MemberResponse
	total int64
	err   error
	query domain.MemberQuery
}

func (s *memberRepositoryStub) List(_ context.Context, _ uuid.UUID, query domain.MemberQuery) ([]domain.MemberResponse, int64, error) {
	s.query = query
	return s.items, s.total, s.err
}

func (s *memberRepositoryStub) GetByID(context.Context, uuid.UUID, uuid.UUID) (*domain.MemberResponse, error) {
	return nil, s.err
}

func (s *memberRepositoryStub) UpdateRole(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.MemberResponse, error) {
	return nil, s.err
}

func (s *memberRepositoryStub) HasPermission(context.Context, uuid.UUID, string) (bool, error) {
	return true, s.err
}

func (s *memberRepositoryStub) ListTutors(context.Context, uuid.UUID, domain.TutorQuery) ([]domain.TutorResponse, int64, error) {
	return nil, 0, s.err
}

func TestMemberUsecaseList(t *testing.T) {
	tests := []struct {
		name        string
		query       domain.MemberQuery
		total       int64
		expectPage  int
		expectSize  int
		expectPages int
		expectError bool
	}{
		{name: "defaults pagination", total: 21, expectPage: 1, expectSize: 20, expectPages: 2},
		{name: "normalizes invalid page size", query: domain.MemberQuery{Page: 0, PageSize: 101}, total: 0, expectPage: 1, expectSize: 20},
		{name: "returns repository error", expectError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &memberRepositoryStub{total: test.total}
			if test.expectError {
				stub.err = errors.New("database unavailable")
			}
			result, err := NewMemberUsecase(stub).List(context.Background(), uuid.New(), test.query)
			if test.expectError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if stub.query.Page != test.expectPage || stub.query.PageSize != test.expectSize {
				t.Fatalf("pagination = %+v", stub.query)
			}
			if result.Pagination.TotalPages != test.expectPages {
				t.Fatalf("total pages = %d", result.Pagination.TotalPages)
			}
		})
	}
}
