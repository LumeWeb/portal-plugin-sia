package internal

import "time"

const (
	ProtocolName        = "sia"
	ProtocolDisplayName = "Sia"
)

const (
	FundingSyncCronSchedule        = "*/5 * * * *"
	AuthRequestCleanupCronSchedule = "*/5 * * * *"
	AuthRequestTTL                 = 10 * time.Minute
)

// AccountFundInterval is the funding interval used by indexd's contract
// manager maintenance loop.
const AccountFundInterval = 2 * time.Minute

// AccountFundIntervalScaleFactor is the number of funding intervals per hour.
const AccountFundIntervalScaleFactor = time.Hour / AccountFundInterval // 30

// CalculateFundTargetBytes divides storage limit by the number of funding
// intervals in an hour to get the per-interval fund target.
func CalculateFundTargetBytes(storageLimitBytes uint64) uint64 {
	if storageLimitBytes == 0 {
		return 0
	}
	return storageLimitBytes / uint64(AccountFundIntervalScaleFactor)
}
