package core

import (
	"context"

	"go.sia.tech/core/rhp/v4"
	"go.sia.tech/core/types"
	"go.sia.tech/indexd/accounts"
	"go.sia.tech/indexd/api"
	"go.sia.tech/indexd/api/admin"
	"go.sia.tech/indexd/hosts"
)

type AdminClient interface {
	Quota(ctx context.Context, key string) (accounts.Quota, error)
	PutQuota(ctx context.Context, key string, req accounts.PutQuotaRequest) error
	DeleteQuota(ctx context.Context, key string) error
	AddAppConnectKey(ctx context.Context, req accounts.AppConnectKeyRequest) (accounts.ConnectKey, error)
	DeleteAppConnectKey(ctx context.Context, key string) error
	DeleteAccount(ctx context.Context, acc rhp.Account) error
	FundingEvents(ctx context.Context, cursor accounts.FundingCursor, limit int) ([]accounts.FundingEvent, error)
	Hosts(ctx context.Context, opts ...admin.HostQueryParameterOption) ([]hosts.Host, error)
	Account(ctx context.Context, ak types.PublicKey) (accounts.Account, error)
	Accounts(ctx context.Context, opts ...api.URLQueryParameterOption) ([]accounts.Account, error)
	PruneSlabs(ctx context.Context, ak types.PublicKey, opts ...api.URLQueryParameterOption) error
	StatsApps(ctx context.Context, offset, limit int) (admin.AppStatsResponse, error)
}
