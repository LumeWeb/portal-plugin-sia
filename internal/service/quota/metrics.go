package quota

import (
	"github.com/prometheus/client_golang/prometheus"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
)

const (
	MetricProvisionAccount      = "provision_account_total"
	MetricSyncFunding           = "sync_funding_total"
	MetricEnforceFundingTarget  = "enforce_funding_target_total"

	MetricProvisionAccountDuration      = "provision_account_duration_seconds"
	MetricSyncFundingDuration           = "sync_funding_duration_seconds"
	MetricEnforceFundingTargetDuration  = "enforce_funding_target_duration_seconds"
)

const (
	LabelStatusError   = "error"
	LabelStatusSuccess = "success"
)

var (
	ProvisionAccountTotal     *prometheus.CounterVec
	SyncFundingTotal          *prometheus.CounterVec
	EnforceFundingTargetTotal *prometheus.CounterVec

	ProvisionAccountDuration     *prometheus.HistogramVec
	SyncFundingDuration          *prometheus.HistogramVec
	EnforceFundingTargetDuration *prometheus.HistogramVec
)

func init() {
	ProvisionAccountTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricProvisionAccount,
			Subsystem: pluginCore.QUOTA_SERVICE,
			Help:      "Total number of ProvisionAccount operations",
		},
		[]string{"status"},
	)

	SyncFundingTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricSyncFunding,
			Subsystem: pluginCore.QUOTA_SERVICE,
			Help:      "Total number of SyncFunding operations",
		},
		[]string{"status"},
	)

	EnforceFundingTargetTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricEnforceFundingTarget,
			Subsystem: pluginCore.QUOTA_SERVICE,
			Help:      "Total number of EnforceFundingTarget operations",
		},
		[]string{"status"},
	)

	ProvisionAccountDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricProvisionAccountDuration,
			Subsystem: pluginCore.QUOTA_SERVICE,
			Help:      "Duration of ProvisionAccount operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{},
	)

	SyncFundingDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricSyncFundingDuration,
			Subsystem: pluginCore.QUOTA_SERVICE,
			Help:      "Duration of SyncFunding operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{},
	)

	EnforceFundingTargetDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricEnforceFundingTargetDuration,
			Subsystem: pluginCore.QUOTA_SERVICE,
			Help:      "Duration of EnforceFundingTarget operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{},
	)
}

func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		ProvisionAccountTotal,
		SyncFundingTotal,
		EnforceFundingTargetTotal,
		ProvisionAccountDuration,
		SyncFundingDuration,
		EnforceFundingTargetDuration,
	}
}
