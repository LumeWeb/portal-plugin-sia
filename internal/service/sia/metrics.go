package sia

import (
	"github.com/prometheus/client_golang/prometheus"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
)

const (
	MetricRegisterAccount          = "register_account_total"
	MetricGetAccount               = "get_account_total"
	MetricAccountExists            = "account_exists_total"
	MetricDeleteAccount            = "delete_account_total"
	MetricGetFundingCursor         = "get_funding_cursor_total"
	MetricUpdateFundingCursor      = "update_funding_cursor_total"
	MetricListProvisionedAccounts  = "list_provisioned_accounts_total"
	MetricUpdateFundTargetBytes    = "update_fund_target_bytes_total"

	MetricRegisterAccountDuration          = "register_account_duration_seconds"
	MetricGetAccountDuration               = "get_account_duration_seconds"
	MetricAccountExistsDuration            = "account_exists_duration_seconds"
	MetricDeleteAccountDuration            = "delete_account_duration_seconds"
	MetricGetFundingCursorDuration         = "get_funding_cursor_duration_seconds"
	MetricUpdateFundingCursorDuration      = "update_funding_cursor_duration_seconds"
	MetricListProvisionedAccountsDuration  = "list_provisioned_accounts_duration_seconds"
	MetricUpdateFundTargetBytesDuration    = "update_fund_target_bytes_duration_seconds"
)

const (
	LabelStatusError   = "error"
	LabelStatusSuccess = "success"
)

var (
	RegisterAccountTotal         *prometheus.CounterVec
	GetAccountTotal              *prometheus.CounterVec
	AccountExistsTotal           *prometheus.CounterVec
	DeleteAccountTotal           *prometheus.CounterVec
	GetFundingCursorTotal        *prometheus.CounterVec
	UpdateFundingCursorTotal     *prometheus.CounterVec
	ListProvisionedAccountsTotal *prometheus.CounterVec
	UpdateFundTargetBytesTotal   *prometheus.CounterVec

	RegisterAccountDuration         *prometheus.HistogramVec
	GetAccountDuration              *prometheus.HistogramVec
	AccountExistsDuration           *prometheus.HistogramVec
	DeleteAccountDuration            *prometheus.HistogramVec
	GetFundingCursorDuration        *prometheus.HistogramVec
	UpdateFundingCursorDuration     *prometheus.HistogramVec
	ListProvisionedAccountsDuration *prometheus.HistogramVec
	UpdateFundTargetBytesDuration   *prometheus.HistogramVec
)

func init() {
	RegisterAccountTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricRegisterAccount,
			Subsystem: pluginCore.SIA_SERVICE,
			Help:      "Total number of RegisterAccount operations",
		},
		[]string{"status"},
	)

	GetAccountTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricGetAccount,
			Subsystem: pluginCore.SIA_SERVICE,
			Help:      "Total number of GetAccount operations",
		},
		[]string{"status"},
	)

	AccountExistsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricAccountExists,
			Subsystem: pluginCore.SIA_SERVICE,
			Help:      "Total number of AccountExists operations",
		},
		[]string{"status"},
	)

	DeleteAccountTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricDeleteAccount,
			Subsystem: pluginCore.SIA_SERVICE,
			Help:      "Total number of DeleteAccount operations",
		},
		[]string{"status"},
	)

	GetFundingCursorTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricGetFundingCursor,
			Subsystem: pluginCore.SIA_SERVICE,
			Help:      "Total number of GetFundingCursor operations",
		},
		[]string{"status"},
	)

	UpdateFundingCursorTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricUpdateFundingCursor,
			Subsystem: pluginCore.SIA_SERVICE,
			Help:      "Total number of UpdateFundingCursor operations",
		},
		[]string{"status"},
	)

	ListProvisionedAccountsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricListProvisionedAccounts,
			Subsystem: pluginCore.SIA_SERVICE,
			Help:      "Total number of ListProvisionedAccounts operations",
		},
		[]string{"status"},
	)

	UpdateFundTargetBytesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricUpdateFundTargetBytes,
			Subsystem: pluginCore.SIA_SERVICE,
			Help:      "Total number of UpdateFundTargetBytes operations",
		},
		[]string{"status"},
	)

	RegisterAccountDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricRegisterAccountDuration,
			Subsystem: pluginCore.SIA_SERVICE,
			Help:      "Duration of RegisterAccount operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{},
	)

	GetAccountDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricGetAccountDuration,
			Subsystem: pluginCore.SIA_SERVICE,
			Help:      "Duration of GetAccount operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{},
	)

	AccountExistsDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricAccountExistsDuration,
			Subsystem: pluginCore.SIA_SERVICE,
			Help:      "Duration of AccountExists operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{},
	)

	DeleteAccountDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricDeleteAccountDuration,
			Subsystem: pluginCore.SIA_SERVICE,
			Help:      "Duration of DeleteAccount operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{},
	)

	GetFundingCursorDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricGetFundingCursorDuration,
			Subsystem: pluginCore.SIA_SERVICE,
			Help:      "Duration of GetFundingCursor operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{},
	)

	UpdateFundingCursorDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricUpdateFundingCursorDuration,
			Subsystem: pluginCore.SIA_SERVICE,
			Help:      "Duration of UpdateFundingCursor operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{},
	)

	ListProvisionedAccountsDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricListProvisionedAccountsDuration,
			Subsystem: pluginCore.SIA_SERVICE,
			Help:      "Duration of ListProvisionedAccounts operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{},
	)

	UpdateFundTargetBytesDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricUpdateFundTargetBytesDuration,
			Subsystem: pluginCore.SIA_SERVICE,
			Help:      "Duration of UpdateFundTargetBytes operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{},
	)
}

func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		RegisterAccountTotal,
		GetAccountTotal,
		AccountExistsTotal,
		DeleteAccountTotal,
		GetFundingCursorTotal,
		UpdateFundingCursorTotal,
		ListProvisionedAccountsTotal,
		UpdateFundTargetBytesTotal,
		RegisterAccountDuration,
		GetAccountDuration,
		AccountExistsDuration,
		DeleteAccountDuration,
		GetFundingCursorDuration,
		UpdateFundingCursorDuration,
		ListProvisionedAccountsDuration,
		UpdateFundTargetBytesDuration,
	}
}
