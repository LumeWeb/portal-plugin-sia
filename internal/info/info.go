package info

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal-plugin-sia/build"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal-plugin-sia/internal"
	"go.lumeweb.com/portal-plugin-sia/internal/api"
	"go.lumeweb.com/portal-plugin-sia/internal/cron"
	"go.lumeweb.com/portal-plugin-sia/internal/db"
	"go.lumeweb.com/portal-plugin-sia/internal/protocol"
	quotaService "go.lumeweb.com/portal-plugin-sia/internal/service/quota"
	siaService "go.lumeweb.com/portal-plugin-sia/internal/service/sia"
	core "go.lumeweb.com/portal/core"
)

func GetCollectors() []prometheus.Collector {
	var collectors []prometheus.Collector

	collectors = append(collectors, siaService.GetCollectors()...)
	collectors = append(collectors, quotaService.GetCollectors()...)
	collectors = append(collectors, api.GetCollectors()...)

	return collectors
}

func GetPluginInfo() core.PluginInfo {
	return core.PluginInfo{
		ID:       internal.ProtocolName,
		Version:  build.GetInfo(),
		API:      api.NewAPI,
		Protocol: protocol.NewProtocol,
		Services: func() ([]core.ServiceInfo, error) {
			return []core.ServiceInfo{
				{
					ID:      pluginCore.SIA_SERVICE,
					Factory: siaService.NewSiaService,
				},
				{
					ID:      pluginCore.QUOTA_SERVICE,
					Factory: quotaService.NewQuotaService,
				},
			}, nil
		},
		Models: []any{
			&db.SiaAccount{},
			&db.SiaSlab{},
			&db.SiaObject{},
			&db.SiaObjectSlab{},
			&db.SiaAppAccount{},
			&db.AuthRequest{},
			&db.FundingCursor{},
		},
		Metrics: GetCollectors(),
		CronJobs: []core.PluginCronJob{
			{
				Name: "sia.funding_sync",
				Factory: func() (core.CronJob, error) {
					return cron.NewSyncFundingJob(), nil
				},
				Schedule: core.NewCronScheduleDefinition(core.CronScheduleTypeCron).WithCronExpression(internal.FundingSyncCronSchedule),
			},
		},
	}
}
