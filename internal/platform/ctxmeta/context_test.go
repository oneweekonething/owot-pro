package ctxmeta

import (
	"context"
	"testing"
)

func TestCloneDropsCancellationAndUnrelatedValues(t *testing.T) {
	type privateKey struct{}
	ctx, cancel := context.WithCancel(context.Background())
	user := LoginUser{ID: 7, TenantID: 2, UserType: 1}
	ctx = WithIgnoreTenant(WithLoginUser(WithTraceID(WithTenantID(ctx, 2), "trace"), user))
	ctx = context.WithValue(ctx, privateKey{}, "transaction")
	cancel()
	clone := CloneContext(ctx)
	got, ok := User(clone)
	if clone.Err() != nil || clone.Value(privateKey{}) != nil || TenantID(clone) != 2 || TraceID(clone) != "trace" || !IsIgnoreTenant(clone) || !ok || got != user {
		t.Fatalf("clone lost metadata or retained request state: user=%+v err=%v", got, clone.Err())
	}
}
