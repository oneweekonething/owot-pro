// Package ctxmeta carries request identity without depending on transport frameworks.
package ctxmeta

import "context"

type key uint8

const (
	tenantKey key = iota
	ignoreTenantKey
	userKey
	traceKey
	dataScopeKey
)

type LoginUser struct {
	ID         int64
	TenantID   int64
	UserType   int
	SuperAdmin bool
}

// DataScope is resolved once by the request guard. Absence means an internal
// identity lookup; a present empty scope deliberately grants no business rows.
type DataScope struct {
	All, Self bool
	DeptIDs   []int64
	UserID    int64
}

func WithDataScope(ctx context.Context, scope DataScope) context.Context {
	return context.WithValue(ctx, dataScopeKey, &scope)
}
func WithoutDataScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, dataScopeKey, (*DataScope)(nil))
}
func Scope(ctx context.Context) (DataScope, bool) {
	s, _ := ctx.Value(dataScopeKey).(*DataScope)
	if s == nil {
		return DataScope{}, false
	}
	return *s, true
}

func WithTenantID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, tenantKey, id)
}

func TenantID(ctx context.Context) int64 {
	id, _ := ctx.Value(tenantKey).(int64)
	return id
}

func WithIgnoreTenant(ctx context.Context) context.Context {
	return context.WithValue(ctx, ignoreTenantKey, true)
}

func IsIgnoreTenant(ctx context.Context) bool {
	ignore, _ := ctx.Value(ignoreTenantKey).(bool)
	return ignore
}

func WithLoginUser(ctx context.Context, user LoginUser) context.Context {
	return context.WithValue(ctx, userKey, user)
}

func User(ctx context.Context) (LoginUser, bool) {
	u, ok := ctx.Value(userKey).(LoginUser)
	return u, ok
}

func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceKey, id)
}

func TraceID(ctx context.Context) string {
	id, _ := ctx.Value(traceKey).(string)
	return id
}

// CloneContext keeps request metadata but drops cancellation and transaction values.
func CloneContext(ctx context.Context) context.Context {
	clone := WithTraceID(WithTenantID(context.Background(), TenantID(ctx)), TraceID(ctx))
	if u, ok := User(ctx); ok {
		clone = WithLoginUser(clone, u)
	}
	if IsIgnoreTenant(ctx) {
		clone = WithIgnoreTenant(clone)
	}
	if scope, ok := Scope(ctx); ok {
		scope.DeptIDs = append([]int64(nil), scope.DeptIDs...)
		clone = WithDataScope(clone, scope)
	}
	return clone
}
