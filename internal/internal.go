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

// AccountFundInterval is the funding interval used by indexd's account
// manager maintenance loop. indexd refunds every 5 minutes.
const AccountFundInterval = 5 * time.Minute

// AccountFundIntervalScaleFactor is the number of funding intervals per hour.
const AccountFundIntervalScaleFactor = time.Hour / AccountFundInterval // 12

// CalculateFundTargetBytes divides storage limit by the number of funding
// intervals in an hour to get the per-interval fund target.
func CalculateFundTargetBytes(storageLimitBytes uint64) uint64 {
	if storageLimitBytes == 0 {
		return 0
	}
	return storageLimitBytes / uint64(AccountFundIntervalScaleFactor)
}

// PredictedQuotaBytes computes the upload and download bytes to reserve for a
// given total predicted usage. Upload and download are split evenly since
// indexd funds both directions from the same FundTargetBytes budget.
func PredictedQuotaBytes(totalBytes uint64) (uploadBytes, downloadBytes uint64) {
	uploadBytes = totalBytes / 2
	downloadBytes = totalBytes / 2
	return
}
