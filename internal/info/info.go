package info

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal-plugin-sia/build"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal-plugin-sia/internal"
	"go.lumeweb.com/portal-plugin-sia/internal/api"
	"go.lumeweb.com/portal-plugin-sia/internal/cron"
	"go.lumeweb.com/portal-plugin-sia/internal/db"
	"go.lumeweb.com/portal-plugin-sia/internal/db/migrations"
	"go.lumeweb.com/portal-plugin-sia/internal/protocol"
	quotaService "go.lumeweb.com/portal-plugin-sia/internal/service/quota"
	siaService "go.lumeweb.com/portal-plugin-sia/internal/service/sia"
	core "go.lumeweb.com/portal/core"
	portal_plugin_sia "go.lumeweb.com/web/go/portal-plugin-sia"
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
		Depends:  []string{"quota"},
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
		Migrations: core.DBMigration{
			core.DB_TYPE_SQLITE: migrations.GetSQLite(),
			core.DB_TYPE_MYSQL:  migrations.GetMySQL(),
		},
		Metrics: GetCollectors(),
		WebBundles: core.NewWebBundles(core.NewWebBundle(portal_plugin_sia.GetFS(), core.WithWebBundleTargetApps("dashboard"))),
		CronJobs: []core.PluginCronJob{
			{
				Name: "funding_sync",
				Factory: func() (core.CronJob, error) {
					return cron.NewSyncFundingJob(), nil
				},
				Schedule: core.NewCronScheduleDefinition(core.CronScheduleTypeCron).WithCronExpression(internal.FundingSyncCronSchedule),
			},
			{
				Name: "auth_request_cleanup",
				Factory: func() (core.CronJob, error) {
					return cron.NewAuthRequestCleanupJob(), nil
				},
				Schedule: core.NewCronScheduleDefinition(core.CronScheduleTypeCron).WithCronExpression(internal.AuthRequestCleanupCronSchedule),
			},
		},
	}
}
