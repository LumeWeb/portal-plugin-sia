package quota

import (
	"context"
	"errors"
	"fmt"
	"math"

	quotaCore "go.lumeweb.com/portal-plugin-quota/core"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	siaDB "go.lumeweb.com/portal-plugin-sia/internal/db"
	internalPkg "go.lumeweb.com/portal-plugin-sia/internal"
	quotaPkg "go.lumeweb.com/portal-plugin-sia/internal/quota"
	core "go.lumeweb.com/portal/core"
	db "go.lumeweb.com/portal/db"
	"go.sia.tech/core/types"
	"go.sia.tech/indexd/accounts"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var _ pluginCore.QuotaService = (*QuotaService)(nil)

type QuotaService struct {
	*core.BaseComponent
	siaService pluginCore.SiaService
}

func NewQuotaService() (core.Service, []core.ContextBuilderOption, error) {
	svc := &QuotaService{}
	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			svc.siaService = core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

			quotaCore.OnQuotaPlanChanged(ctx, func(_ context.Context, event quotaCore.QuotaPlanChangedEvent) error {
				if err := svc.EnforceFundingTarget(event.Ctx, event.UserID); err != nil {
					svc.Logger().Error("failed to enforce funding target on quota plan change", zap.Uint("userID", event.UserID), zap.Error(err))
				}
				return nil
			})

			return nil
		}),
	)
	return svc, opts, nil
}

func (s *QuotaService) ID() string {
	return pluginCore.QUOTA_SERVICE
}

func (s *QuotaService) Start() error {
	return nil
}

func (s *QuotaService) ProvisionAccount(ctx context.Context, userID uint) error {
	ctx, span := core.TraceMethod(ctx, "QuotaService.ProvisionAccount")
	defer span.End()

	return core.MetricTrack(
		ProvisionAccountDuration.WithLabelValues(),
		ProvisionAccountTotal.WithLabelValues(LabelStatusError),
		func() error {
			if s.siaService.AdminClient() == nil {
				return errors.New("admin client not configured")
			}

			// Only provision verified users
			var verified bool
			core.WithService[core.UserService](s.Context(), core.USER_SERVICE, func(us core.UserService) error {
				v, err := us.IsAccountVerified(ctx, userID)
				if err == nil {
					verified = v
				}
				return nil
			})
			if !verified {
				return errors.New("user account not verified")
			}

			account, err := s.siaService.RegisterAccount(ctx, userID)
			if err != nil {
				return fmt.Errorf("failed to get/create sia account: %w", err)
			}

			if account.QuotaKey != "" && account.ConnectKey != "" {
				return nil
			}

			quotaKey := fmt.Sprintf("user-%d", userID)

			var fundTargetBytes uint64
		core.WithService[quotaCore.QuotaService](s.Context(), quotaCore.QUOTA_SERVICE, func(qs quotaCore.QuotaService) error {
			configManager := qs.GetConfigManager()
			if configManager != nil {
				limits, err := configManager.ResolveEffectiveLimits(ctx, userID)
				if err == nil && limits != nil && limits.HasStorageLimitConfig && limits.StorageLimitConfig != nil {
					fundTargetBytes = internalPkg.CalculateFundTargetBytes(limits.StorageLimitConfig.Bytes)
				}
			}
			return nil
		})

		quotaReq := accounts.PutQuotaRequest{
			Description:     fmt.Sprintf("Portal user %d", userID),
			TotalUses:       math.MaxInt32,
			FundTargetBytes: &fundTargetBytes,
		}

			if err := s.siaService.AdminClient().PutQuota(ctx, quotaKey, quotaReq); err != nil {
				return fmt.Errorf("failed to create indexd quota: %w", err)
			}

			connectKeyResp, err := s.siaService.AdminClient().AddAppConnectKey(ctx, accounts.AppConnectKeyRequest{
				Description: fmt.Sprintf("Portal connect key for user %d", userID),
				Quota:       quotaKey,
			})
			if err != nil {
				return fmt.Errorf("failed to create indexd connect key: %w", err)
			}

			account.QuotaKey = quotaKey
			account.ConnectKey = connectKeyResp.Key
			account.FundTargetBytes = fundTargetBytes
			if err := db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Save(account)
			}); err != nil {
				return fmt.Errorf("failed to update sia account: %w", err)
			}

			return nil
		},
	)
}

func (s *QuotaService) EnforceFundingTarget(ctx context.Context, userID uint) error {
	ctx, span := core.TraceMethod(ctx, "QuotaService.EnforceFundingTarget")
	defer span.End()

	return core.MetricTrack(
		EnforceFundingTargetDuration.WithLabelValues(),
		EnforceFundingTargetTotal.WithLabelValues(LabelStatusError),
		func() error {
			account, err := s.siaService.GetAccount(ctx, userID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return fmt.Errorf("failed to get sia account: %w", err)
			}

			if account.QuotaKey == "" {
				return nil
			}

			var fundTargetBytes uint64
			overLimit := false

			core.WithService[quotaCore.QuotaService](s.Context(), quotaCore.QUOTA_SERVICE, func(qs quotaCore.QuotaService) error {
				// Resolve effective limits for this user
				configManager := qs.GetConfigManager()
				if configManager == nil {
					return nil
				}

				limits, err := configManager.ResolveEffectiveLimits(ctx, userID)
				if err != nil || limits == nil {
					return nil
				}

				// No storage limit means no Sia access
				if !limits.HasStorageLimitConfig || limits.StorageLimitConfig == nil {
					fundTargetBytes = 0
					overLimit = true
					return nil
				}

				fundTargetBytes = limits.StorageLimitConfig.Bytes

				// Check storage quota
				storageResult, err := quotaPkg.CheckStorageQuota(ctx, s.Context(), userID, 0)
				if err != nil {
					s.Logger().Warn("failed to check storage quota", zap.Uint("userID", userID), zap.Error(err))
				} else if storageResult != nil && !storageResult.Allowed {
					overLimit = true
				}

				// Check upload quota
				uploadResult, err := quotaPkg.CheckUploadQuota(ctx, s.Context(), userID, 0)
				if err != nil {
					s.Logger().Warn("failed to check upload quota", zap.Uint("userID", userID), zap.Error(err))
				} else if uploadResult != nil && !uploadResult.Allowed {
					overLimit = true
				}

				// Check download quota
				downloadResult, err := quotaPkg.CheckDownloadQuota(ctx, s.Context(), userID, 0)
				if err != nil {
					s.Logger().Warn("failed to check download quota", zap.Uint("userID", userID), zap.Error(err))
				} else if downloadResult != nil && !downloadResult.Allowed {
					overLimit = true
				}

				return nil
			})

			if overLimit {
				fundTargetBytes = 0
			}

			// Skip update if unchanged
			if fundTargetBytes == account.FundTargetBytes {
				return nil
			}

			s.Logger().Debug("updating fund target bytes", zap.Uint("userID", userID), zap.Uint64("fundTargetBytes", fundTargetBytes))

			if err := s.siaService.UpdateFundTargetBytes(ctx, userID, fundTargetBytes); err != nil {
				s.Logger().Error("failed to update fund target bytes", zap.Uint("userID", userID), zap.Error(err))
				return err
			}

			return nil
		},
	)
}

func (s *QuotaService) Stop() error {
	return nil
}

func (s *QuotaService) SyncFunding(ctx context.Context) error {
	ctx, span := core.TraceMethod(ctx, "QuotaService.SyncFunding")
	defer span.End()

	return core.MetricTrack(
		SyncFundingDuration.WithLabelValues(),
		SyncFundingTotal.WithLabelValues(LabelStatusError),
		func() error {
			if s.siaService.AdminClient() == nil {
				return nil
			}

			// 1. Read global cursor from our DB (single row, single table lookup)
			cursor, err := s.siaService.GetFundingCursor(ctx)
			if err != nil {
				return err
			}

			// 2. Walk the funding feed from our cursor position
			fundingCursor := accounts.FundingCursor{
				After: cursor.LastFundingEventAt,
				ID:    cursor.LastFundingEventID,
			}

			const pageSize = 500
			hasMore := true

			for hasMore {
				events, err := s.siaService.AdminClient().FundingEvents(ctx, fundingCursor, pageSize)
				if err != nil {
					s.Logger().Error("failed to fetch funding events", zap.Error(err))
					return err
				}

				if len(events) == 0 {
					break
				}

				// 3. Process each event — look up account on-demand by AccountKey via SiaAppAccount join
				for _, event := range events {
					appAccount, err := s.siaService.GetAppAccountByKey(ctx, types.PublicKey(event.AccountKey))
					if err != nil {
						continue
					}

					var account siaDB.SiaAccount
					if err := db.RetryableComponentLock(s, func(tx *gorm.DB) *gorm.DB {
						return tx.Where("id = ?", appAccount.SiaAccountID).First(&account)
					}); err != nil {
						continue
					}

					// Dedup: skip events we've already processed for this account
					if event.ID <= account.LastFundingEventID {
						continue
					}

					// Update account cursor
					account.LastFundingEventID = event.ID
					account.LastFundingEventAt = &event.CreatedAt
					if err := db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
						return tx.Save(&account)
					}); err != nil {
						s.Logger().Error("failed to update account funding state", zap.Uint("userID", account.UserID), zap.Error(err))
						continue
					}

					// Record bytes with quota system
					if event.EstimatedUploadBytes > 0 {
						if err := quotaPkg.RecordUpload(ctx, s.Context(), account.UserID, 0, event.EstimatedUploadBytes, ""); err != nil {
							s.Logger().Error("failed to record upload", zap.Uint("userID", account.UserID), zap.Error(err))
						}
					}
					if event.EstimatedDownloadBytes > 0 {
						if err := quotaPkg.RecordDownload(ctx, s.Context(), account.UserID, 0, event.EstimatedDownloadBytes, ""); err != nil {
							s.Logger().Error("failed to record download", zap.Uint("userID", account.UserID), zap.Error(err))
						}
					}
				}

				// 4. Advance cursor to last event of this page
				last := events[len(events)-1]
				fundingCursor = accounts.FundingCursor{
					After: last.CreatedAt,
					ID:    last.ID,
				}

				hasMore = len(events) == pageSize

				// 5. Persist global cursor after each page (crash recovery — don't lose progress)
				cursor.LastFundingEventID = last.ID
				cursor.LastFundingEventAt = last.CreatedAt
				if err := s.siaService.UpdateFundingCursor(ctx, cursor); err != nil {
					s.Logger().Error("failed to update funding cursor", zap.Error(err))
					return err
				}
			}

			return nil
		},
	)
}
