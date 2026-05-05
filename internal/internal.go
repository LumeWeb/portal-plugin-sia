package internal

import "time"

const (
	ProtocolName        = "sia"
	ProtocolDisplayName = "Sia"
)

const (
	FundingSyncCronSchedule = "0 * * * *"
)

// AccountFundInterval is the funding interval used by indexd.
const AccountFundInterval = 5 * time.Minute

// AccountFundIntervalScaleFactor is the number of funding intervals per hour.
const AccountFundIntervalScaleFactor = time.Hour / AccountFundInterval // 12

// CalculateFundTargetBytes divides storage limit by the number of 5-minute
// intervals in an hour to get the per-interval fund target.
func CalculateFundTargetBytes(storageLimitBytes uint64) uint64 {
	if storageLimitBytes == 0 {
		return 0
	}
	return storageLimitBytes / uint64(AccountFundIntervalScaleFactor)
}
