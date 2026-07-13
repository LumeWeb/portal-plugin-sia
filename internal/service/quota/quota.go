package quota

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync/atomic"

	quotaCore "go.lumeweb.com/portal-plugin-quota/core"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	internalPkg "go.lumeweb.com/portal-plugin-sia/internal"
	siaDB "go.lumeweb.com/portal-plugin-sia/internal/db"
	quotaPkg "go.lumeweb.com/portal-plugin-sia/internal/quota"
	core "go.lumeweb.com/portal/core"
	db "go.lumeweb.com/portal/db"
	"go.opentelemetry.io/otel/attribute"
	"go.sia.tech/core/types"
	"go.sia.tech/indexd/accounts"
	"go.sia.tech/indexd/api/admin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var _ pluginCore.QuotaService = (*QuotaService)(nil)

var ErrAccountNotVerified = errors.New("user account not verified")

type QuotaService struct {
	*core.BaseComponent
	siaService          pluginCore.SiaService
	activeContractCount atomic.Uint64
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
				return ErrAccountNotVerified
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
						fundTargetBytes = internalPkg.CalculateFundTargetBytes(s.activeContractCount.Load())
					}
				}
				return nil
			})

			quotaReq := accounts.PutQuotaRequest{
				Description:     fmt.Sprintf("Portal user %d", userID),
				MaxPinnedData:   math.MaxInt64,
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
					s.Logger().Debug("enforce funding target: no sia account found", zap.Uint("userID", userID))
					return nil
				}
				return fmt.Errorf("failed to get sia account: %w", err)
			}

			s.Logger().Debug("enforce funding target: account state",
				zap.Uint("userID", userID),
				zap.Uint64("fundTargetBytes", account.FundTargetBytes),
				zap.String("quotaKey", account.QuotaKey),
			)

			if account.QuotaKey == "" {
				s.Logger().Debug("enforce funding target: QuotaKey empty, skipping", zap.Uint("userID", userID))
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
					s.Logger().Debug("enforce funding target: no limits resolved",
						zap.Uint("userID", userID),
						zap.Bool("limitsNil", limits == nil),
						zap.Error(err),
					)
					return nil
				}

				// No storage limit means no Sia access
				if !limits.HasStorageLimitConfig || limits.StorageLimitConfig == nil {
					s.Logger().Debug("enforce funding target: no storage limit config", zap.Uint("userID", userID))
					fundTargetBytes = 0
					overLimit = true
					return nil
				}

				// Use the cached active contract count (updated once per sync
				// tick by SyncFunding). Each contract's account gets its
				// proportional share of the per-interval bandwidth budget.
				numContracts := s.activeContractCount.Load()
				if numContracts == 0 {
					s.Logger().Debug("enforce funding target: no active contracts, skipping", zap.Uint("userID", userID))
					return nil
				}
				fundTargetBytes = internalPkg.CalculateFundTargetBytes(numContracts)
				s.Logger().Debug("enforce funding target: calculated fund target",
					zap.Uint("userID", userID),
					zap.Uint64("storageLimitBytes", limits.StorageLimitConfig.Bytes),
					zap.Uint64("numContracts", numContracts),
					zap.Uint64("fundTargetBytes", fundTargetBytes),
					zap.Uint64("accountFundTargetBytes", account.FundTargetBytes),
				)

				// Check storage quota — over-storage users must not receive funding.
				storageResult, err := quotaPkg.CheckStorageQuota(ctx, s.Context(), userID, 0)
				if err != nil {
					s.Logger().Warn("failed to check storage quota", zap.Uint("userID", userID), zap.Error(err))
				} else if storageResult != nil && !storageResult.Allowed {
					s.Logger().Warn("enforce funding target: storage quota exceeded",
						zap.Uint("userID", userID),
						zap.Uint64("currentUsage", storageResult.Details.CurrentUsage),
					)
					overLimit = true
				}

				// Predict the per-interval upload/download bytes based on
				// physical bandwidth ceiling and Sia erasure coding overhead.
				// This is independent of host/contract count — the pipe is
				// a hard physical ceiling.
				if fundTargetBytes > 0 {
					uploadBytes, downloadBytes := internalPkg.PredictedBandwidthBytes()

					if uploadBytes > 0 {
						uploadResult, err := quotaPkg.CheckUploadQuota(ctx, s.Context(), userID, uploadBytes)
						if err != nil {
							s.Logger().Warn("failed to check upload quota", zap.Uint("userID", userID), zap.Uint64("uploadBytes", uploadBytes), zap.Error(err))
						} else if uploadResult != nil && !uploadResult.Allowed {
							s.Logger().Warn("enforce funding target: upload quota exceeded",
								zap.Uint("userID", userID),
								zap.Uint64("uploadBytes", uploadBytes),
								zap.Uint64("currentUsage", uploadResult.Details.CurrentUsage),
							)
							overLimit = true
						}
					}

					if downloadBytes > 0 {
						downloadResult, err := quotaPkg.CheckDownloadQuota(ctx, s.Context(), userID, downloadBytes)
						if err != nil {
							s.Logger().Warn("failed to check download quota", zap.Uint("userID", userID), zap.Uint64("downloadBytes", downloadBytes), zap.Error(err))
						} else if downloadResult != nil && !downloadResult.Allowed {
							s.Logger().Warn("enforce funding target: download quota exceeded",
								zap.Uint("userID", userID),
								zap.Uint64("downloadBytes", downloadBytes),
								zap.Uint64("currentUsage", downloadResult.Details.CurrentUsage),
							)
							overLimit = true
						}
					}
				}

				return nil
			})

			if overLimit {
				s.Logger().Debug("enforce funding target: over limit, setting fundTargetBytes to 0", zap.Uint("userID", userID))
				fundTargetBytes = 0
			}

			// Skip update if unchanged
			if fundTargetBytes == account.FundTargetBytes {
				s.Logger().Debug("enforce funding target: skipping update, unchanged",
					zap.Uint("userID", userID),
					zap.Uint64("fundTargetBytes", fundTargetBytes),
				)
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

func (s *QuotaService) ConnectQuotaCheck(ctx context.Context, userID uint) (*pluginCore.ConnectQuotaResult, error) {
	result := &pluginCore.ConnectQuotaResult{
		HasQuota: true, // assume good until proven otherwise
	}

	// 1. Get the user's Sia account from DB
	account, err := s.siaService.GetAccount(ctx, userID)
	if err != nil {
		s.Logger().Error("connect quota check: failed to get sia account", zap.Uint("userID", userID), zap.Error(err))
		return nil, fmt.Errorf("failed to get sia account: %w", err)
	}

	s.Logger().Debug("connect quota check: account state",
		zap.Uint("userID", userID),
		zap.Uint64("fundTargetBytes", account.FundTargetBytes),
		zap.String("quotaKey", account.QuotaKey),
		zap.String("connectKey", account.ConnectKey),
	)

	// If FundTargetBytes is 0, the user has no storage quota plan
	if account.FundTargetBytes == 0 {
		s.Logger().Warn("connect quota check: blocking - FundTargetBytes is 0",
			zap.Uint("userID", userID),
			zap.String("quotaKey", account.QuotaKey),
		)
		result.HasQuota = false
		return result, nil
	}

	// 2. Calculate predicted bytes based on physical bandwidth ceiling.
	//    Upload uses 3x multiplier (Sia 10-of-30 erasure coding).
	//    Download uses 1x (only 10 shards needed to reconstruct).
	//    This is a hard physical ceiling — no user can exceed their pipe
	//    speed regardless of host or contract count.
	uploadBytes, downloadBytes := internalPkg.PredictedBandwidthBytes()

	s.Logger().Debug("connect quota check: predicted bandwidth bytes",
		zap.Uint("userID", userID),
		zap.Uint64("uploadBytes", uploadBytes),
		zap.Uint64("downloadBytes", downloadBytes),
	)

	// 3. Check quota reserves against predicted usage.
	uploadResult, err := quotaPkg.CheckUploadQuota(ctx, s.Context(), userID, uploadBytes, quotaCore.WithCreateReservation())
	if err != nil {
		s.Logger().Warn("failed to check upload quota", zap.Uint("userID", userID), zap.Uint64("uploadBytes", uploadBytes), zap.Error(err))
	} else if uploadResult != nil && !uploadResult.Allowed {
		s.Logger().Warn("connect quota check: upload quota exceeded",
			zap.Uint("userID", userID),
			zap.Uint64("uploadBytes", uploadBytes),
		)
		uploadResult.ReleaseReservation()
		result.HasQuota = false
		return result, nil
	}

	// Check download quota with predicted download bytes
	downloadResult, err := quotaPkg.CheckDownloadQuota(ctx, s.Context(), userID, downloadBytes, quotaCore.WithCreateReservation())
	if err != nil {
		s.Logger().Warn("failed to check download quota", zap.Uint("userID", userID), zap.Error(err))
		if uploadResult != nil {
			uploadResult.ReleaseReservation()
		}
		result.HasQuota = false
		return result, nil
	} else if downloadResult != nil && !downloadResult.Allowed {
		s.Logger().Warn("connect quota check: download quota exceeded",
			zap.Uint("userID", userID),
			zap.Uint64("downloadBytes", downloadBytes),
		)
		downloadResult.ReleaseReservation()
		if uploadResult != nil {
			uploadResult.ReleaseReservation()
		}
		result.HasQuota = false
		return result, nil
	}

	// Predictive pre-check only — release reservations since the result does not
	// carry them back to be consumed by the caller.
	quotaPkg.ReleaseReservations(uploadResult, downloadResult)

	return result, nil
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

			// Fetch and cache the active contract count once per sync tick.
			// EnforceFundingTarget uses this cached value instead of fetching
			// contracts per-user.
			activeContracts, err := s.siaService.AdminClient().Contracts(ctx, admin.WithRevisable(true))
			if err != nil {
				s.Logger().Warn("sync funding: failed to fetch active contracts", zap.Error(err))
			} else {
				s.activeContractCount.Store(uint64(len(activeContracts)))
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
			var totalProcessed, totalSkipped int64

			for hasMore {
				pageCtx, pageSpan := core.TraceMethod(ctx, "QuotaService.SyncFunding.fetchPage")

				events, err := s.siaService.AdminClient().FundingEvents(pageCtx, fundingCursor, pageSize)
				if err != nil {
					pageSpan.End()
					s.Logger().Error("failed to fetch funding events", zap.Error(err))
					return err
				}

				if len(events) == 0 {
					pageSpan.End()
					break
				}

				pageSpan.SetAttributes(
					attribute.Int64("events.count", int64(len(events))),
					attribute.Int64("funding_cursor.id", fundingCursor.ID),
				)
				pageSpan.End()

				// 3. Process each event — look up account on-demand
				for _, event := range events {
					var account siaDB.SiaAccount

					if event.FundType == accounts.FundingTypePool {
						// Pool funding events: AccountKey is the pool's public key,
						// not a user account key. Use QuotaName (resolved by indexd
						// via pool_id → pools → app_connect_keys → quotas) to find
						// the user's SiaAccount by QuotaKey.
						if event.QuotaName == nil || *event.QuotaName == "" {
							s.Logger().Debug("pool funding event has no quota name, skipping",
								zap.Int64("eventID", event.ID))
							totalSkipped++
							continue
						}
						acc, err := s.siaService.GetAccountByQuotaKey(ctx, *event.QuotaName)
						if err != nil {
							s.Logger().Debug("no sia account found for quota key",
								zap.String("quotaKey", *event.QuotaName),
								zap.Int64("eventID", event.ID))
							totalSkipped++
							continue
						}
						account = *acc
					} else {
						// Account funding events: look up via AccountKey → SiaAppAccount → SiaAccount
						appAccount, err := s.siaService.GetAppAccountByKey(ctx, types.PublicKey(event.AccountKey))
						if err != nil {
							totalSkipped++
							continue
						}

						if err := db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
							return tx.Where("id = ?", appAccount.SiaAccountID).First(&account)
						}); err != nil {
							totalSkipped++
							continue
						}
					}

					// Dedup: skip events we've already processed for this account
					if event.ID <= account.LastFundingEventID {
						totalSkipped++
						continue
					}

					// Update account cursor
					account.LastFundingEventID = event.ID
					account.LastFundingEventAt = &event.CreatedAt
					if err := db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
						return tx.Save(&account)
					}); err != nil {
						s.Logger().Error("failed to update account funding state", zap.Uint("userID", account.UserID), zap.Error(err))
						totalSkipped++
						continue
					}
					totalProcessed++

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

			span.SetAttributes(
				attribute.Int64("events.processed", totalProcessed),
				attribute.Int64("events.skipped", totalSkipped),
			)

			return nil
		},
	)
}
