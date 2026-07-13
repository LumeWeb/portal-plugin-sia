package internal

import (
	"time"

	"github.com/docker/go-units"
)

const (
	ProtocolName        = "sia"
	ProtocolDisplayName = "Sia"
)

const (
	FundingSyncCronSchedule        = "*/5 * * * *"
	AuthRequestCleanupCronSchedule = "*/5 * * * *"
	AuthRequestTTL                 = 10 * time.Minute
)

// AccountFundInterval is the funding interval used by indexd's account
// manager maintenance loop. indexd refunds every 5 minutes.
const AccountFundInterval = 5 * time.Minute

// AccountFundIntervalScaleFactor is the number of funding intervals per hour.
const AccountFundIntervalScaleFactor = time.Hour / AccountFundInterval // 12

// MaxBandwidthBytesPerSec is the assumed maximum sustained bandwidth a user
// can achieve (125 MB/s = 1 Gbps). Covers residential gigabit fiber and
// standard VPS instances. Users would need 29-88 Gbps sustained to exceed
// quota in a single interval — practically impossible.
var MaxBandwidthBytesPerSec = mustParseSize("125MB")

func mustParseSize(size string) uint64 {
	v, err := units.FromHumanSize(size)
	if err != nil {
		panic(err)
	}
	return uint64(v)
}

// BandwidthBytesPerInterval is the raw bytes a user can transfer in one
// funding interval at MaxBandwidthBytesPerSec. This is the combined
// upload+download budget — actual redundancy overhead is accounted for at
// pin time via the quota plugin's CalculateScaledSize using slab metadata.
func BandwidthBytesPerInterval() uint64 {
	return MaxBandwidthBytesPerSec * uint64(AccountFundInterval/time.Second)
}

// CalculateFundTargetBytes returns the per-contract fund target for one
// interval: bandwidth * interval / numContracts. Each contract's account
// gets its proportional share of the per-interval bandwidth budget.
// Actual redundancy overhead is accounted for at pin time.
func CalculateFundTargetBytes(numContracts uint64) uint64 {
	if numContracts == 0 {
		return 0
	}
	return BandwidthBytesPerInterval() / numContracts
}

// PredictedBandwidthBytes computes the upload and download bytes a user
// could transfer in one funding interval, based on the physical bandwidth
// ceiling. Both values are equal — actual redundancy overhead is accounted
// for at pin time, not predicted ahead.
//
// This is a hard physical ceiling — no user can exceed their pipe speed
// regardless of host or contract count.
func PredictedBandwidthBytes() (uploadBytes, downloadBytes uint64) {
	bytes := BandwidthBytesPerInterval()
	uploadBytes = bytes
	downloadBytes = bytes
	return
}
