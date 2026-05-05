package core

import (
	"context"

	core "go.lumeweb.com/portal/core"
)

type QuotaService interface {
	core.Service

	ProvisionAccount(ctx context.Context, userID uint) error
	SyncFunding(ctx context.Context) error
	EnforceFundingTarget(ctx context.Context, userID uint) error
}
