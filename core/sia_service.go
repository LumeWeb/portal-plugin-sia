package core

import (
	"context"

	"go.lumeweb.com/portal-plugin-sia/internal/db"
	core "go.lumeweb.com/portal/core"
	"go.lumeweb.com/queryutil"
	"go.sia.tech/core/types"
)

type SiaService interface {
	core.Service

	RegisterAccount(ctx context.Context, userID uint) (*db.SiaAccount, error)
	GetAccount(ctx context.Context, userID uint) (*db.SiaAccount, error)
	GetAccountByID(ctx context.Context, id uint) (*db.SiaAccount, error)
	GetAccountByQuotaKey(ctx context.Context, quotaKey string) (*db.SiaAccount, error)
	AccountExists(ctx context.Context, userID uint) (bool, error)
	DeleteAccount(ctx context.Context, userID uint) error
	RegisterAppAccount(ctx context.Context, siaAccountID uint, accountKey types.PublicKey) (*db.SiaAppAccount, error)
	GetAppAccountByKey(ctx context.Context, accountKey types.PublicKey) (*db.SiaAppAccount, error)
	ListAppAccounts(ctx context.Context, siaAccountID uint) ([]db.SiaAppAccount, error)
	CountAppAccountsBySiaAccount(ctx context.Context) (map[uint]int, error)
	DeleteAppAccountsBySiaAccountID(ctx context.Context, siaAccountID uint) error
	DeleteAppAccount(ctx context.Context, siaAccountID uint, accountKey types.PublicKey) error
	AdminClient() AdminClient
	GetFundingCursor(ctx context.Context) (*db.FundingCursor, error)
	UpdateFundingCursor(ctx context.Context, cursor *db.FundingCursor) error
	ListProvisionedAccounts(ctx context.Context) ([]db.SiaAccount, error)
	UpdateFundTargetBytes(ctx context.Context, userID uint, fundTargetBytes uint64) error
	RegisterSlab(ctx context.Context, siaAppAccountID uint, slabID string) error
	DeleteSlab(ctx context.Context, siaAppAccountID uint, slabID string) error
	ListSlabsByAppAccount(ctx context.Context, siaAppAccountID uint) ([]db.SiaSlab, error)
	DeleteSlabsByAppAccount(ctx context.Context, siaAppAccountID uint) error

	// Object and slab pruning methods
	RegisterObject(ctx context.Context, siaAppAccountID uint, objectID string, slabIDs []string) error
	DeleteObject(ctx context.Context, siaAppAccountID uint, objectID string) error
	DeleteObjectsByAppAccount(ctx context.Context, siaAppAccountID uint) error
	FindOrphanedSlabs(ctx context.Context, siaAppAccountID uint) ([]string, error)
	PruneSlabs(ctx context.Context, siaAppAccountID uint) error

	// App listing and account-level pruning
	ListApps(ctx context.Context, userID uint, filters []queryutil.CrudFilter, sorts []queryutil.Sort, pagination queryutil.Pagination) ([]AppAccount, int64, error)
	PruneAccount(ctx context.Context, userID uint) error

	// StorageStats returns protocol-specific storage statistics for the Sia protocol.
	// Counts slab pins only (excluding virtual object pins which have Size=0),
	// and reports physical slab counts and redundancy-scaled storage sizes.
	StorageStats(ctx context.Context) (*core.ProtocolStorageStats, error)

	// Auth request methods
	StoreAuthRequest(ctx context.Context, requestID string, userID uint) error
	GetAuthRequest(ctx context.Context, requestID string) (*db.AuthRequest, error)
	DeleteAuthRequest(ctx context.Context, requestID string) error
	CleanupExpiredAuthRequests(ctx context.Context) error
}
