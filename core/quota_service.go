package core

import (
	"context"

	core "go.lumeweb.com/portal/core"
)

// ConnectQuotaResult describes the outcome of a predictive quota check
// before showing the Connect UI.
type ConnectQuotaResult struct {
	// HasQuota is true if the user has sufficient upload and download quota
	// for their predicted next-interval usage.
	HasQuota bool
}

type QuotaService interface {
	core.Service

	ProvisionAccount(ctx context.Context, userID uint) error
	SyncFunding(ctx context.Context) error
	EnforceFundingTarget(ctx context.Context, userID uint) error

	// ConnectQuotaCheck performs a predictive quota check for the Connect UI.
	// It estimates the user's next-interval upload and download usage based on
	// a physical bandwidth ceiling and Sia erasure coding overhead, then
	// verifies those bytes are within the user's quota limits.
	ConnectQuotaCheck(ctx context.Context, userID uint) (*ConnectQuotaResult, error)
}
