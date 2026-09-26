package domain

import (
	"context"

	"github.com/google/uuid"
)

// Caller is the principal behind an HTTP request, taken from the verified access token by
// AuthMiddleware. Permission checks use it to require that the token still belongs to an
// active membership (KEL-76): a token outlives the membership it was minted for, so the
// tenant and role claims alone are not enough.
type Caller struct {
	UserID uuid.UUID
	// MemberID is the tenant_members row the token was minted for. It is uuid.Nil for tokens
	// issued without the member_id claim; those are matched by (tenant, user, role) instead.
	MemberID uuid.UUID
}

type callerContextKey struct{}

// WithCaller returns a context carrying the verified caller.
func WithCaller(ctx context.Context, caller Caller) context.Context {
	return context.WithValue(ctx, callerContextKey{}, caller)
}

// CallerFromContext returns the verified caller, if AuthMiddleware attached one.
func CallerFromContext(ctx context.Context) (Caller, bool) {
	caller, ok := ctx.Value(callerContextKey{}).(Caller)
	return caller, ok
}

// MemberPermissionQuery identifies the membership a permission decision is made for.
//
// KEL-76: a permission is only granted through a tenant_members row that is still active
// (is_active and not soft-deleted), belongs to TenantID, and currently carries RoleID. A
// token minted for a membership that was removed, deactivated, or moved to another role
// therefore stops authorizing anything even though its signature is still valid.
//
// At least one of MemberID or UserID must be set. MemberID pins the exact membership row the
// token was minted for, so a token from an earlier membership never authorizes a newer one.
// UserID alone matches the caller's membership in the tenant; it serves tokens issued
// without a member_id claim.
type MemberPermissionQuery struct {
	TenantID   uuid.UUID
	RoleID     uuid.UUID
	MemberID   uuid.UUID
	UserID     uuid.UUID
	Permission string
}

// Complete reports whether the query names enough of the membership to be answered. An
// incomplete query must be denied, never widened.
func (q MemberPermissionQuery) Complete() bool {
	return q.TenantID != uuid.Nil && q.RoleID != uuid.Nil && q.Permission != "" && (q.MemberID != uuid.Nil || q.UserID != uuid.Nil)
}
