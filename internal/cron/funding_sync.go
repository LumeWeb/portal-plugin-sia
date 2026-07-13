package cron

import (
	"context"

	"github.com/google/uuid"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal-plugin-sia/internal"
	core "go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

const (
	FundingSyncJobSourceID = "sia"
	FundingSyncJobType     = "plugin.sia.funding_sync"
)

type SyncFundingJob struct {
	*core.BaseCronJob
}

func NewSyncFundingJob() core.CronJob {
	job := &SyncFundingJob{}

	jobID := uuid.New()
	scheduleDef := core.NewCronScheduleDefinition(core.CronScheduleTypeCron).
		WithCronExpression(internal.FundingSyncCronSchedule)

	job.BaseCronJob = core.NewBaseCronJob(
		jobID,
		core.JobOriginPlugin,
		FundingSyncJobSourceID,
		"Sia Funding Sync",
		scheduleDef,
		nil,
		core.WithExplicitJobType(FundingSyncJobType),
	)

	return job
}

func (j *SyncFundingJob) Run(ctx core.Context, eventCtx context.Context) error {
	_, span := core.TraceMethod(eventCtx, "SyncFundingJob.Run")
	defer span.End()

	logger := ctx.Logger()

	quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
	if quotaSvc == nil {
		return nil
	}

	if err := quotaSvc.SyncFunding(eventCtx); err != nil {
		return err
	}

	siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)
	if siaSvc == nil {
		return nil
	}

	accounts, err := siaSvc.ListProvisionedAccounts(eventCtx)
	if err != nil {
		return err
	}

	// Batch-fetch app counts to avoid N+1 per-account DB lookups.
	// On success, absent keys mean 0 apps (enforceFundingTarget defaults to 1).
	// On failure, pass -1 to trigger per-account ListAppAccounts fallback.
	appCounts, err := siaSvc.CountAppAccountsBySiaAccount(eventCtx)
	if err != nil {
		logger.Warn("failed to batch-fetch app counts, falling back to per-account lookup", zap.Error(err))
		appCounts = nil
	}

	for _, account := range accounts {
		var numApps int
		if appCounts != nil {
			numApps = appCounts[account.ID] // 0 is valid → defaults to 1 inside
		} else {
			numApps = -1 // sentinel: batch unavailable, trigger per-account fallback
		}
		if err := quotaSvc.EnforceFundingTargetWithAppCount(eventCtx, account.UserID, numApps); err != nil {
			logger.Error("failed to enforce funding target", zap.Uint("userID", account.UserID), zap.Error(err))
			continue
		}
	}

	return nil
}
