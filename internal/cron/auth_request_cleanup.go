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
	AuthRequestCleanupJobSourceID = "sia"
	AuthRequestCleanupJobType     = "plugin.sia.auth_request_cleanup"
)

type AuthRequestCleanupJob struct {
	*core.BaseCronJob
}

func NewAuthRequestCleanupJob() core.CronJob {
	job := &AuthRequestCleanupJob{}

	jobID := uuid.New()
	scheduleDef := core.NewCronScheduleDefinition(core.CronScheduleTypeCron).
		WithCronExpression(internal.AuthRequestCleanupCronSchedule)

	job.BaseCronJob = core.NewBaseCronJob(
		jobID,
		core.JobOriginPlugin,
		AuthRequestCleanupJobSourceID,
		"Sia Auth Request Cleanup",
		scheduleDef,
		nil,
		core.WithExplicitJobType(AuthRequestCleanupJobType),
	)

	return job
}

func (j *AuthRequestCleanupJob) Run(ctx core.Context, eventCtx context.Context) error {
	_, span := core.TraceMethod(eventCtx, "AuthRequestCleanupJob.Run")
	defer span.End()

	siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)
	if siaSvc == nil {
		return nil
	}

	if err := siaSvc.CleanupExpiredAuthRequests(eventCtx); err != nil {
		ctx.Logger().Error("failed to cleanup expired auth requests", zap.Error(err))
		return err
	}

	return nil
}
