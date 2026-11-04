package app

import (
	"context"
	"fmt"
	"slices"
	"time"

	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

func wrap(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func validIDs(ids []int64) bool {
	if len(ids) == 0 || len(ids) > 1000 {
		return false
	}
	for _, id := range ids {
		if id <= 0 {
			return false
		}
	}
	return true
}

func uniqueIDs(ids []int64) []int64 {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	return slices.Compact(ids)
}

func validStatusFilter(status *int) bool { return status == nil || domain.ValidStatus(*status) }

func validRange(from, to *time.Time) bool { return from == nil || to == nil || !from.After(*to) }

func requireTenant(ctx context.Context) error {
	if ctxmeta.TenantID(ctx) <= 0 {
		return domain.ErrTenantRequired
	}
	return nil
}
