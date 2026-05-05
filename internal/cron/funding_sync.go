package cron

import (
	"context"

	"github.com/google/uuid"
	"go.lumeweb.com/portal-plugin-sia/internal"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
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

	for _, account := range accounts {
		if err := quotaSvc.EnforceFundingTarget(eventCtx, account.UserID); err != nil {
			logger.Error("failed to enforce funding target", zap.Uint("userID", account.UserID), zap.Error(err))
			continue
		}
	}

	return nil
}
