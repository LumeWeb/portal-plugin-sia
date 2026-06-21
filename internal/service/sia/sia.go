package sia

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	pluginConfig "go.lumeweb.com/portal-plugin-sia/internal/config"
	siaDB "go.lumeweb.com/portal-plugin-sia/internal/db"
	"go.lumeweb.com/portal-plugin-sia/internal"
	"go.lumeweb.com/portal-plugin-sia/internal/events"
	"go.lumeweb.com/portal-plugin-sia/internal/quota"
	core "go.lumeweb.com/portal/core"
	db "go.lumeweb.com/portal/db"
	"go.sia.tech/core/rhp/v4"
	"go.sia.tech/core/types"
	"go.sia.tech/indexd/accounts"
	"go.sia.tech/indexd/api/admin"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ pluginCore.SiaService = (*SiaService)(nil)

type SiaService struct {
	*core.BaseComponent
	adminClient pluginCore.AdminClient
	mu          sync.Map
}

func NewSiaService() (core.Service, []core.ContextBuilderOption, error) {
	svc := &SiaService{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			pluginCfg := core.GetProtocolConfig[*pluginConfig.ProtocolConfig](ctx, "sia")
			if pluginCfg != nil && pluginCfg.URL != "" && pluginCfg.Key != "" {
				svc.adminClient = admin.NewClient(pluginCfg.URL, pluginCfg.Key)
			}

			return nil
		}),
	)

	return svc, opts, nil
}

func NewSiaServiceWithAdminClient(adminClient pluginCore.AdminClient) (core.Service, []core.ContextBuilderOption, error) {
	svc := &SiaService{
		adminClient: adminClient,
	}

	return svc, core.ContextOptions(), nil
}

func (s *SiaService) ID() string {
	return pluginCore.SIA_SERVICE
}

func (s *SiaService) Start() error {
	return nil
}

// StoreAuthRequest stores a mapping between an indexd request ID and the user who
// initiated the auth request. This allows looking up the user's ConnectKey for
// approval without requiring JWT authentication.
func (s *SiaService) StoreAuthRequest(ctx context.Context, requestID string, userID uint) error {
	ctx, span := core.TraceMethod(ctx, "SiaService.StoreAuthRequest")
	defer span.End()

	ar := &siaDB.AuthRequest{
		RequestID: requestID,
		UserID:    userID,
	}

	return db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Create(ar)
	})
}

// GetAuthRequest retrieves an auth request by its request ID.
// Returns error if not found or expired.
func (s *SiaService) GetAuthRequest(ctx context.Context, requestID string) (*siaDB.AuthRequest, error) {
	ctx, span := core.TraceMethod(ctx, "SiaService.GetAuthRequest")
	defer span.End()

	var ar siaDB.AuthRequest
	err := db.RetryableComponentLock(s, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("request_id = ?", requestID).First(&ar)
	})
	if err != nil {
		return nil, err
	}

	if ar.IsExpired() {
		return nil, fmt.Errorf("auth request expired")
	}

	return &ar, nil
}

// DeleteAuthRequest deletes an auth request record by its request ID.
func (s *SiaService) DeleteAuthRequest(ctx context.Context, requestID string) error {
	ctx, span := core.TraceMethod(ctx, "SiaService.DeleteAuthRequest")
	defer span.End()

	return db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("request_id = ?", requestID).Delete(&siaDB.AuthRequest{})
	})
}

// CleanupExpiredAuthRequests deletes all expired auth request records.
func (s *SiaService) CleanupExpiredAuthRequests(ctx context.Context) error {
	ctx, span := core.TraceMethod(ctx, "SiaService.CleanupExpiredAuthRequests")
	defer span.End()

	return db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("created_at < ?", time.Now().Add(-internal.AuthRequestTTL)).Delete(&siaDB.AuthRequest{})
	})
}

func (s *SiaService) Stop() error {
	return nil
}

func (s *SiaService) AdminClient() pluginCore.AdminClient {
	return s.adminClient
}

func (s *SiaService) RegisterAppAccount(ctx context.Context, siaAccountID uint, accountKey types.PublicKey) (*siaDB.SiaAppAccount, error) {
	ctx, span := core.TraceMethod(ctx, "SiaService.RegisterAppAccount")
	defer span.End()

	appAccount := &siaDB.SiaAppAccount{
		SiaAccountID: siaAccountID,
		AccountKey:   siaDB.DBAccountKeyFromPublicKey(accountKey),
	}

	err := db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "account_key"}},
			DoNothing: true,
		}).Create(appAccount)
	})
	if err != nil {
		return nil, err
	}

	if appAccount.ID == 0 {
		return s.GetAppAccountByKey(ctx, accountKey)
	}

	return appAccount, nil
}

func (s *SiaService) GetAppAccountByKey(ctx context.Context, accountKey types.PublicKey) (*siaDB.SiaAppAccount, error) {
	ctx, span := core.TraceMethod(ctx, "SiaService.GetAppAccountByKey")
	defer span.End()

	var appAccount siaDB.SiaAppAccount
	err := db.RetryableComponentLock(s, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("account_key = ?", siaDB.DBAccountKeyFromPublicKey(accountKey)).First(&appAccount)
	})
	if err != nil {
		return nil, err
	}
	return &appAccount, nil
}

func (s *SiaService) ListAppAccounts(ctx context.Context, siaAccountID uint) ([]siaDB.SiaAppAccount, error) {
	ctx, span := core.TraceMethod(ctx, "SiaService.ListAppAccounts")
	defer span.End()

	var appAccounts []siaDB.SiaAppAccount
	err := db.RetryableComponentLock(s, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("sia_account_id = ?", siaAccountID).Find(&appAccounts)
	})
	return appAccounts, err
}

func (s *SiaService) DeleteAppAccountsBySiaAccountID(ctx context.Context, siaAccountID uint) error {
	ctx, span := core.TraceMethod(ctx, "SiaService.DeleteAppAccountsBySiaAccountID")
	defer span.End()

	return db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("sia_account_id = ?", siaAccountID).Delete(&siaDB.SiaAppAccount{})
	})
}

func (s *SiaService) RegisterAccount(ctx context.Context, userID uint) (*siaDB.SiaAccount, error) {
	ctx, span := core.TraceMethod(ctx, "SiaService.RegisterAccount")
	defer span.End()

	timer := prometheus.NewTimer(RegisterAccountDuration.WithLabelValues())
	defer timer.ObserveDuration()

	account := &siaDB.SiaAccount{
		UserID: userID,
	}

	err := db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoNothing: true,
		}).Create(account)
	})
	if err != nil {
		RegisterAccountTotal.WithLabelValues(LabelStatusError).Inc()
		return nil, err
	}

	isNew := account.ID != 0

	account, err = s.GetAccount(ctx, userID)
	if err != nil {
		RegisterAccountTotal.WithLabelValues(LabelStatusError).Inc()
		return nil, err
	}

	if isNew {
		core.Fire(s.Context(), events.EVENT_SIA_ACCOUNT_REGISTERED,
			events.NewSiaAccountRegisteredEvent(ctx, account))
	}

	RegisterAccountTotal.WithLabelValues(LabelStatusSuccess).Inc()
	return account, nil
}

func (s *SiaService) GetAccount(ctx context.Context, userID uint) (*siaDB.SiaAccount, error) {
	ctx, span := core.TraceMethod(ctx, "SiaService.GetAccount")
	defer span.End()

	return core.MetricTrackResult(
		GetAccountDuration.WithLabelValues(),
		GetAccountTotal.WithLabelValues(LabelStatusError),
		func() (*siaDB.SiaAccount, error) {
			var account siaDB.SiaAccount
			err := db.RetryableComponentLock(s, func(tx *gorm.DB) *gorm.DB {
				return tx.Where("user_id = ?", userID).First(&account)
			})
			if err != nil {
				return nil, err
			}
			return &account, nil
		},
	)
}

func (s *SiaService) GetAccountByID(ctx context.Context, id uint) (*siaDB.SiaAccount, error) {
	ctx, span := core.TraceMethod(ctx, "SiaService.GetAccountByID")
	defer span.End()

	var account siaDB.SiaAccount
	err := db.RetryableComponentLock(s, func(tx *gorm.DB) *gorm.DB {
		return tx.First(&account, id)
	})
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func (s *SiaService) GetAccountByQuotaKey(ctx context.Context, quotaKey string) (*siaDB.SiaAccount, error) {
	ctx, span := core.TraceMethod(ctx, "SiaService.GetAccountByQuotaKey")
	defer span.End()

	var account siaDB.SiaAccount
	err := db.RetryableComponentLock(s, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("quota_key = ?", quotaKey).First(&account)
	})
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func (s *SiaService) AccountExists(ctx context.Context, userID uint) (bool, error) {
	ctx, span := core.TraceMethod(ctx, "SiaService.AccountExists")
	defer span.End()

	return core.MetricTrackResult(
		AccountExistsDuration.WithLabelValues(),
		AccountExistsTotal.WithLabelValues(LabelStatusError),
		func() (bool, error) {
			var count int64
			err := db.RetryableComponentLock(s, func(tx *gorm.DB) *gorm.DB {
				return tx.Model(&siaDB.SiaAccount{}).Where("user_id = ?", userID).Count(&count)
			})
			return count > 0, err
		},
	)
}

func (s *SiaService) DeleteAccount(ctx context.Context, userID uint) error {
	ctx, span := core.TraceMethod(ctx, "SiaService.DeleteAccount")
	defer span.End()

	return core.MetricTrack(
		DeleteAccountDuration.WithLabelValues(),
		DeleteAccountTotal.WithLabelValues(LabelStatusError),
		func() error {
			if s.adminClient == nil {
				return errors.New("admin client not configured")
			}

			account, err := s.GetAccount(ctx, userID)
			if err != nil {
				return fmt.Errorf("failed to get sia account: %w", err)
			}

			appAccounts, err := s.ListAppAccounts(ctx, account.ID)
			if err != nil {
				return fmt.Errorf("failed to list app accounts: %w", err)
			}

			for _, appAccount := range appAccounts {
				protoAccount := rhp.Account(appAccount.AccountKey.PublicKey())
				if err := s.adminClient.DeleteAccount(ctx, protoAccount); err != nil {
					return fmt.Errorf("failed to delete indexd account: %w", err)
				}
			}

			if account.QuotaKey != "" {
				if err := s.adminClient.DeleteQuota(ctx, account.QuotaKey); err != nil {
					return fmt.Errorf("failed to delete quota: %w", err)
				}
			}

			if account.ConnectKey != "" {
				if err := s.adminClient.DeleteAppConnectKey(ctx, account.ConnectKey); err != nil {
					return fmt.Errorf("failed to delete app connect key: %w", err)
				}
			}

			if err := s.DeleteAppAccountsBySiaAccountID(ctx, account.ID); err != nil {
				return fmt.Errorf("failed to delete app account records: %w", err)
			}

			if err := db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Delete(account)
			}); err != nil {
				return fmt.Errorf("failed to delete sia account record: %w", err)
			}

			s.mu.Delete(userID)

			return nil
		},
	)
}

func (s *SiaService) GetFundingCursor(ctx context.Context) (*siaDB.FundingCursor, error) {
	ctx, span := core.TraceMethod(ctx, "SiaService.GetFundingCursor")
	defer span.End()

	return core.MetricTrackResult(
		GetFundingCursorDuration.WithLabelValues(),
		GetFundingCursorTotal.WithLabelValues(LabelStatusError),
		func() (*siaDB.FundingCursor, error) {
			var cursor siaDB.FundingCursor
			err := db.RetryableComponentLock(s, func(tx *gorm.DB) *gorm.DB {
				return tx.Where("id = ?", 1).First(&cursor)
			})
			if err != nil {
				return nil, err
			}
			return &cursor, nil
		},
	)
}

func (s *SiaService) UpdateFundingCursor(ctx context.Context, cursor *siaDB.FundingCursor) error {
	ctx, span := core.TraceMethod(ctx, "SiaService.UpdateFundingCursor")
	defer span.End()

	return core.MetricTrack(
		UpdateFundingCursorDuration.WithLabelValues(),
		UpdateFundingCursorTotal.WithLabelValues(LabelStatusError),
		func() error {
			return db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Save(cursor)
			})
		},
	)
}

func (s *SiaService) ListProvisionedAccounts(ctx context.Context) ([]siaDB.SiaAccount, error) {
	ctx, span := core.TraceMethod(ctx, "SiaService.ListProvisionedAccounts")
	defer span.End()

	return core.MetricTrackResult(
		ListProvisionedAccountsDuration.WithLabelValues(),
		ListProvisionedAccountsTotal.WithLabelValues(LabelStatusError),
		func() ([]siaDB.SiaAccount, error) {
			var accounts []siaDB.SiaAccount
			err := db.RetryableComponentLock(s, func(tx *gorm.DB) *gorm.DB {
				return tx.Where("quota_key != ?", "").Find(&accounts)
			})
			return accounts, err
		},
	)
}

func (s *SiaService) UpdateFundTargetBytes(ctx context.Context, userID uint, fundTargetBytes uint64) error {
	ctx, span := core.TraceMethod(ctx, "SiaService.UpdateFundTargetBytes")
	defer span.End()

	return core.MetricTrack(
		UpdateFundTargetBytesDuration.WithLabelValues(),
		UpdateFundTargetBytesTotal.WithLabelValues(LabelStatusError),
		func() error {
			if s.adminClient == nil {
				return errors.New("admin client not configured")
			}

			// Per-user lock to prevent TOCTOU race between Quota read and PutQuota write
			mu, _ := s.mu.LoadOrStore(userID, &sync.Mutex{})
			userMu := mu.(*sync.Mutex)
			userMu.Lock()
			defer userMu.Unlock()

			account, err := s.GetAccount(ctx, userID)
			if err != nil {
				return fmt.Errorf("failed to get sia account: %w", err)
			}

			if account.QuotaKey == "" {
				return nil
			}

			// Read existing quota to preserve all fields (PutQuota is full replace)
			existingQuota, err := s.adminClient.Quota(ctx, account.QuotaKey)
			if err != nil {
				return fmt.Errorf("failed to read existing quota: %w", err)
			}

			quotaReq := accounts.PutQuotaRequest{
				Description:     existingQuota.Description,
				FundTargetBytes: &fundTargetBytes,
				MaxPinnedData:   existingQuota.MaxPinnedData,
				TotalUses:       existingQuota.TotalUses,
			}

			if err := s.adminClient.PutQuota(ctx, account.QuotaKey, quotaReq); err != nil {
				return fmt.Errorf("failed to update indexd quota fundTargetBytes: %w", err)
			}

			// Update our local record
			account.FundTargetBytes = fundTargetBytes
			if err := db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Save(account)
			}); err != nil {
				return fmt.Errorf("failed to update sia account fund target: %w", err)
			}

			return nil
		},
	)
}

func (s *SiaService) RegisterSlab(ctx context.Context, siaAppAccountID uint, slabID string) error {
	ctx, span := core.TraceMethod(ctx, "SiaService.RegisterSlab")
	defer span.End()

	slab := &siaDB.SiaSlab{
		SiaAppAccountID: siaAppAccountID,
		SlabID:          slabID,
	}

	return db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "sia_app_account_id"}, {Name: "slab_id"}},
			DoNothing: true,
		}).Create(slab)
	})
}

func (s *SiaService) DeleteSlab(ctx context.Context, siaAppAccountID uint, slabID string) error {
	ctx, span := core.TraceMethod(ctx, "SiaService.DeleteSlab")
	defer span.End()

	return db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("sia_app_account_id = ? AND slab_id = ?", siaAppAccountID, slabID).Delete(&siaDB.SiaSlab{})
	})
}

func (s *SiaService) ListSlabsByAppAccount(ctx context.Context, siaAppAccountID uint) ([]siaDB.SiaSlab, error) {
	ctx, span := core.TraceMethod(ctx, "SiaService.ListSlabsByAppAccount")
	defer span.End()

	var slabs []siaDB.SiaSlab
	err := db.RetryableComponentLock(s, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("sia_app_account_id = ?", siaAppAccountID).Find(&slabs)
	})
	return slabs, err
}

func (s *SiaService) DeleteSlabsByAppAccount(ctx context.Context, siaAppAccountID uint) error {
	ctx, span := core.TraceMethod(ctx, "SiaService.DeleteSlabsByAppAccount")
	defer span.End()

	return db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("sia_app_account_id = ?", siaAppAccountID).Delete(&siaDB.SiaSlab{})
	})
}

// RegisterObject registers an object with its slab relationships (idempotent)
func (s *SiaService) RegisterObject(ctx context.Context, siaAppAccountID uint, objectID string, slabIDs []string) error {
	ctx, span := core.TraceMethod(ctx, "SiaService.RegisterObject")
	defer span.End()

	return db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
		// Create/update object record (finding existing first by composite key)
		var siaObject siaDB.SiaObject
		result := tx.Where("sia_app_account_id = ? AND object_id = ? AND deleted_at IS NULL", siaAppAccountID, objectID).First(&siaObject)
		if result.Error != nil {
			// Need to create a new object
			siaObject = siaDB.SiaObject{
				SiaAppAccountID: siaAppAccountID,
				ObjectID:        objectID,
			}
			if err := tx.Create(&siaObject).Error; err != nil {
				return tx
			}
		}

		// Resolve slab IDs to SiaSlab records and create join entries
		for _, slabID := range slabIDs {
			var slab siaDB.SiaSlab
			if err := tx.Where("sia_app_account_id = ? AND slab_id = ? AND deleted_at IS NULL", siaAppAccountID, slabID).First(&slab).Error; err != nil {
				// Slab doesn't exist for this app account yet, shouldn't happen but skip
				continue
			}

			// Create join record with conflict handling
			tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "sia_object_id"}, {Name: "sia_slab_id"}},
				DoNothing: true,
			}).Create(&siaDB.SiaObjectSlab{
				SiaObjectID: siaObject.ID,
				SiaSlabID:   slab.ID,
			})
		}

		return tx
	})
}

// DeleteObject deletes an object and its slab relationships for a given app account
func (s *SiaService) DeleteObject(ctx context.Context, siaAppAccountID uint, objectID string) error {
	ctx, span := core.TraceMethod(ctx, "SiaService.DeleteObject")
	defer span.End()

	return db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
		var siaObject siaDB.SiaObject
		if err := tx.Where("sia_app_account_id = ? AND object_id = ? AND deleted_at IS NULL", siaAppAccountID, objectID).First(&siaObject).Error; err != nil {
			// Object not found, nothing to delete
			return tx
		}

		// Cascade delete join records and object record
		tx.Where("sia_object_id = ?", siaObject.ID).Delete(&siaDB.SiaObjectSlab{})
		return tx.Delete(&siaObject)
	})
}

// DeleteObjectsByAppAccount bulk deletes all objects and their relationships for an app account
func (s *SiaService) DeleteObjectsByAppAccount(ctx context.Context, siaAppAccountID uint) error {
	ctx, span := core.TraceMethod(ctx, "SiaService.DeleteObjectsByAppAccount")
	defer span.End()

	return db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
		// First delete all join records for objects of this app account (subquery approach not universal)
		var objects []siaDB.SiaObject
		if err := tx.Where("sia_app_account_id = ? AND deleted_at IS NULL", siaAppAccountID).Find(&objects).Error; err == nil {
			for _, obj := range objects {
				tx.Where("sia_object_id = ?", obj.ID).Delete(&siaDB.SiaObjectSlab{})
			}
		}

		// Then delete all objects
		return tx.Where("sia_app_account_id = ?", siaAppAccountID).Delete(&siaDB.SiaObject{})
	})
}

// FindOrphanedSlabs returns slabIDs that have no referencing objects across ANY app account
func (s *SiaService) FindOrphanedSlabs(ctx context.Context, siaAppAccountID uint) ([]string, error) {
	ctx, span := core.TraceMethod(ctx, "SiaService.FindOrphanedSlabs")
	defer span.End()

	var orphanedSlabs []siaDB.SiaSlab

	subQuery := s.DB().Model(&siaDB.SiaObjectSlab{}).
		Select("sia_object_slabs.sia_slab_id").
		Joins("JOIN sia_objects ON sia_objects.id = sia_object_slabs.sia_object_id AND sia_objects.deleted_at IS NULL").
		Where("sia_object_slabs.deleted_at IS NULL")

	err := db.RetryableComponentLock(s, func(tx *gorm.DB) *gorm.DB {
		return tx.
			Where("sia_slabs.sia_app_account_id = ? AND sia_slabs.deleted_at IS NULL", siaAppAccountID).
			Where("sia_slabs.id NOT IN (?)", subQuery).
			Find(&orphanedSlabs)
	})
	if err != nil {
		return nil, err
	}

	slabIDs := make([]string, len(orphanedSlabs))
	for i, slab := range orphanedSlabs {
		slabIDs[i] = slab.SlabID
	}
	return slabIDs, nil
}

// PruneSlabs orchestrates full prune logic: finds orphaned slabs, cleans up records,
// and deletes pins/uploads only when no other references exist
func (s *SiaService) PruneSlabs(ctx context.Context, siaAppAccountID uint) error {
	ctx, span := core.TraceMethod(ctx, "SiaService.PruneSlabs")
	defer span.End()

	// 1. Delete all sia_objects + sia_object_slabs for this app account (indexd pruned them)
	if err := s.DeleteObjectsByAppAccount(ctx, siaAppAccountID); err != nil {
		return fmt.Errorf("failed to delete objects: %w", err)
	}

	// 2. Find orphaned slabs
	orphanedSlabIDs, err := s.FindOrphanedSlabs(ctx, siaAppAccountID)
	if err != nil {
		return fmt.Errorf("failed to find orphaned slabs: %w", err)
	}
	if len(orphanedSlabIDs) == 0 {
		return nil
	}

	// Get services for pin/upload cleanup
	pinSvc := core.GetService[core.PinService](s.Context(), core.PIN_SERVICE)
	uploadSvc := core.GetService[core.UploadService](s.Context(), core.UPLOAD_SERVICE)

	// 3. Process each orphaned slab
	for _, slabID := range orphanedSlabIDs {
		// 3a. Delete this app account's slab record
		if err := s.DeleteSlab(ctx, siaAppAccountID, slabID); err != nil {
			s.Logger().Error("failed to delete slab record",
				zap.String("slabID", slabID),
				zap.Uint("appAccountID", siaAppAccountID),
				zap.Error(err))
			continue
		}

		// 3b. Check: does ANY sia_slab record still exist for this slabID (other app accounts)?
		var stillInUse bool
		err := db.RetryableComponentLock(s, func(tx *gorm.DB) *gorm.DB {
			var count int64
			result := tx.Model(&siaDB.SiaSlab{}).
				Where("slab_id = ? AND sia_app_account_id != ? AND deleted_at IS NULL", slabID, siaAppAccountID).
				Count(&count)
			stillInUse = count > 0
			return result
		})
		if err != nil {
			s.Logger().Error("failed to check slab usage",
				zap.String("slabID", slabID),
				zap.Error(err))
			continue
		}
		if stillInUse {
			// Slab still referenced by another app account - skip
			continue
		}

		// 3c. Check: does the corresponding upload have any pins globally?
		storageHash, hashErr := internal.NewSiaHash(slabID)
		if hashErr != nil {
			s.Logger().Error("invalid slab ID hash",
				zap.String("slabID", slabID),
				zap.Error(hashErr))
			continue
		}
		pinned, err := pinSvc.UploadPinnedGlobal(ctx, storageHash)
		if err != nil {
			s.Logger().Error("failed to check global pin status",
				zap.String("slabID", slabID),
				zap.Error(err))
			continue
		}
		if pinned {
			// Upload still pinned globally by someone - skip
			continue
		}

		// 3d. Truly orphaned - delete pins and uploads to reduce quota
		// Find all pins for this hash (across all users)
		allPins, err := pinSvc.GetAllPinsByHash(ctx, storageHash)
		if err != nil {
			s.Logger().Error("failed to get pins by hash",
				zap.String("slabID", slabID),
				zap.Error(err))
			continue
		}

		for _, pin := range allPins {
			if err := pinSvc.DeletePin(ctx, pin.ID); err != nil {
				s.Logger().Error("failed to delete pin",
					zap.Uint("pinID", pin.ID),
					zap.Error(err))
			}
			// Emit unpinned event for quota adjustment
			quota.EmitStorageObjectUnpinned(ctx, s.Context(), pin, "")
		}

		// Delete the upload
		if err := uploadSvc.DeleteUpload(ctx, storageHash); err != nil {
			s.Logger().Error("failed to delete upload",
				zap.String("slabID", slabID),
				zap.Error(err))
		}
	}

	return nil
}
