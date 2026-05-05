package api

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal-plugin-sia/internal"
)

const (
	MetricPinSlab     = "pin_slab_total"
	MetricPinObject   = "pin_object_total"
	MetricUnpinSlab   = "unpin_slab_total"
	MetricUnpinObject = "unpin_object_total"
	MetricProxy       = "proxy_total"

	MetricPinSlabDuration     = "pin_slab_duration_seconds"
	MetricPinObjectDuration   = "pin_object_duration_seconds"
	MetricUnpinSlabDuration   = "unpin_slab_duration_seconds"
	MetricUnpinObjectDuration = "unpin_object_duration_seconds"
	MetricProxyDuration       = "proxy_duration_seconds"
)

const (
	LabelStatusError   = "error"
	LabelStatusSuccess = "success"
)

var (
	PinSlabTotal     *prometheus.CounterVec
	PinObjectTotal   *prometheus.CounterVec
	UnpinSlabTotal   *prometheus.CounterVec
	UnpinObjectTotal *prometheus.CounterVec
	ProxyTotal        *prometheus.CounterVec

	PinSlabDuration     *prometheus.HistogramVec
	PinObjectDuration   *prometheus.HistogramVec
	UnpinSlabDuration   *prometheus.HistogramVec
	UnpinObjectDuration *prometheus.HistogramVec
	ProxyDuration       *prometheus.HistogramVec
)

func init() {
	PinSlabTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricPinSlab,
			Subsystem: internal.ProtocolName,
			Help:      "Total number of pin slab operations",
		},
		[]string{"status"},
	)

	PinObjectTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricPinObject,
			Subsystem: internal.ProtocolName,
			Help:      "Total number of pin object operations",
		},
		[]string{"status"},
	)

	UnpinSlabTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricUnpinSlab,
			Subsystem: internal.ProtocolName,
			Help:      "Total number of unpin slab operations",
		},
		[]string{"status"},
	)

	UnpinObjectTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricUnpinObject,
			Subsystem: internal.ProtocolName,
			Help:      "Total number of unpin object operations",
		},
		[]string{"status"},
	)

	ProxyTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricProxy,
			Subsystem: internal.ProtocolName,
			Help:      "Total number of proxy operations",
		},
		[]string{"status"},
	)

	PinSlabDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricPinSlabDuration,
			Subsystem: internal.ProtocolName,
			Help:      "Duration of pin slab operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{},
	)

	PinObjectDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricPinObjectDuration,
			Subsystem: internal.ProtocolName,
			Help:      "Duration of pin object operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{},
	)

	UnpinSlabDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricUnpinSlabDuration,
			Subsystem: internal.ProtocolName,
			Help:      "Duration of unpin slab operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{},
	)

	UnpinObjectDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricUnpinObjectDuration,
			Subsystem: internal.ProtocolName,
			Help:      "Duration of unpin object operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{},
	)

	ProxyDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricProxyDuration,
			Subsystem: internal.ProtocolName,
			Help:      "Duration of proxy operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{},
	)
}

func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		PinSlabTotal,
		PinObjectTotal,
		UnpinSlabTotal,
		UnpinObjectTotal,
		ProxyTotal,
		PinSlabDuration,
		PinObjectDuration,
		UnpinSlabDuration,
		UnpinObjectDuration,
		ProxyDuration,
	}
}
