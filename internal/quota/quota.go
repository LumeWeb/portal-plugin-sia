package quota

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	quotaCore "go.lumeweb.com/portal-plugin-quota/core"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	core "go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/event"
)

const (
	CheckTypeUpload   = "upload"
	CheckTypeStorage  = "storage"
	CheckTypeDownload = "download"
)

const (
	checkFailedTemplate     = "%s usage check failed: %w"
	storageUsageErrTemplate = "%w (current: %d bytes, requested: %d bytes, maximum: %d bytes)"
	uploadUsageErrTemplate  = "%w (current: %d bytes, requested: %d bytes, maximum: %d bytes)"
	downloadUsageErrTemplate = "%w (current: %d bytes, requested: %d bytes, maximum: %d bytes)"
	defaultUsageErrTemplate  = "%s usage over capacity: current %d bytes + requested %d bytes would exceed threshold of %d bytes"
)

// WithQuotaService executes a function with quota service if available.
// If the quota service is not registered, fn is not called and nil is returned.
func WithQuotaService(cctx context.Context, ctx core.Context, fn func(quotaCore.QuotaService, context.Context) error) error {
	return core.WithService[quotaCore.QuotaService](ctx, quotaCore.QUOTA_SERVICE, func(qs quotaCore.QuotaService) error {
		return fn(qs, cctx)
	})
}

// CheckStorageQuota checks storage quota if service is available.
// Returns nil result when quota service is not loaded (lenient).
func CheckStorageQuota(cctx context.Context, ctx core.Context, userID uint, requestedBytes uint64, opts ...quotaCore.CheckOption) (*quotaCore.QuotaCheckResult, error) {
	var result *quotaCore.QuotaCheckResult

	err := WithQuotaService(cctx, ctx, func(qs quotaCore.QuotaService, c context.Context) error {
		res, err := qs.CheckStorageQuota(c, userID, requestedBytes, opts...)
		if err != nil {
			return err
		}
		result = &res
		return nil
	})

	return result, err
}

// CheckDownloadQuota checks download quota if service is available.
// Returns nil result when quota service is not loaded (lenient).
func CheckDownloadQuota(cctx context.Context, ctx core.Context, userID uint, requestedBytes uint64, opts ...quotaCore.CheckOption) (*quotaCore.QuotaCheckResult, error) {
	var result *quotaCore.QuotaCheckResult

	err := WithQuotaService(cctx, ctx, func(qs quotaCore.QuotaService, c context.Context) error {
		res, err := qs.CheckDownloadQuota(c, userID, requestedBytes, opts...)
		if err != nil {
			return err
		}
		result = &res
		return nil
	})

	return result, err
}

// CheckUploadQuota checks upload quota if service is available.
// Returns nil result when quota service is not loaded (lenient).
func CheckUploadQuota(cctx context.Context, ctx core.Context, userID uint, requestedBytes uint64, opts ...quotaCore.CheckOption) (*quotaCore.QuotaCheckResult, error) {
	var result *quotaCore.QuotaCheckResult

	err := WithQuotaService(cctx, ctx, func(qs quotaCore.QuotaService, c context.Context) error {
		res, err := qs.CheckUploadQuota(c, userID, requestedBytes, opts...)
		if err != nil {
			return err
		}
		result = &res
		return nil
	})

	return result, err
}

// ValidateStorageQuota checks storage quota and returns an error if exceeded.
// When quota service is not available, the check is skipped (lenient).
func ValidateStorageQuota(cctx context.Context, ctx core.Context, userID uint, requestedBytes uint64) error {
	result, err := CheckStorageQuota(cctx, ctx, userID, requestedBytes)
	if err != nil {
		ctx.Logger().Warn("Storage quota check failed",
			zap.Uint("user_id", userID),
			zap.Uint64("requested_bytes", requestedBytes),
			zap.Error(err))
		return fmt.Errorf("quota check failed: %w", err)
	}
	if result != nil && !result.Allowed {
		ctx.Logger().Debug("Storage quota exceeded",
			zap.Uint("user_id", userID),
			zap.Uint64("requested_bytes", requestedBytes),
			zap.Uint64("current_usage", result.Details.CurrentUsage),
			zap.Any("limit", result.Details.Limit))
		return core.ErrStorageQuotaExceeded
	}
	return nil
}

// ValidateDownloadQuota checks download quota and returns an error if exceeded.
// When quota service is not available, the check is skipped (lenient).
func ValidateDownloadQuota(cctx context.Context, ctx core.Context, userID uint, requestedBytes uint64) error {
	result, err := CheckDownloadQuota(cctx, ctx, userID, requestedBytes)
	if err != nil {
		ctx.Logger().Warn("Download quota check failed",
			zap.Uint("user_id", userID),
			zap.Uint64("requested_bytes", requestedBytes),
			zap.Error(err))
		return fmt.Errorf("quota check failed: %w", err)
	}
	if result != nil && !result.Allowed {
		ctx.Logger().Debug("Download quota exceeded",
			zap.Uint("user_id", userID),
			zap.Uint64("requested_bytes", requestedBytes),
			zap.Uint64("current_usage", result.Details.CurrentUsage),
			zap.Any("limit", result.Details.Limit))
		return core.ErrDownloadQuotaExceeded
	}
	return nil
}

// ValidateUploadQuota checks upload quota and returns an error if exceeded.
// When quota service is not available, the check is skipped (lenient).
func ValidateUploadQuota(cctx context.Context, ctx core.Context, userID uint, requestedBytes uint64) error {
	result, err := CheckUploadQuota(cctx, ctx, userID, requestedBytes)
	if err != nil {
		ctx.Logger().Warn("Upload quota check failed",
			zap.Uint("user_id", userID),
			zap.Uint64("requested_bytes", requestedBytes),
			zap.Error(err))
		return fmt.Errorf("quota check failed: %w", err)
	}
	if result != nil && !result.Allowed {
		ctx.Logger().Debug("Upload quota exceeded",
			zap.Uint("user_id", userID),
			zap.Uint64("requested_bytes", requestedBytes),
			zap.Uint64("current_usage", result.Details.CurrentUsage),
			zap.Any("limit", result.Details.Limit))
		return core.ErrUploadQuotaExceeded
	}
	return nil
}

// CheckWithReservation performs a quota check with reservation and provides unified error handling.
// When quota service is not available, returns nil result and nil error (lenient).
func CheckWithReservation(cctx context.Context, ctx core.Context, checkType string, userID uint, requestedBytes uint64, checkFunc func(context.Context, core.Context, uint, uint64, ...quotaCore.CheckOption) (*quotaCore.QuotaCheckResult, error)) (*quotaCore.QuotaCheckResult, error) {
	checkResult, err := checkFunc(cctx, ctx, userID, requestedBytes, quotaCore.WithCreateReservation())
	if err != nil {
		ctx.Logger().Warn("Failed to check quota", zap.String("check_type", checkType), zap.Uint("user_id", userID), zap.Uint64("requested_bytes", requestedBytes), zap.Error(err))
		return nil, fmt.Errorf(checkFailedTemplate, checkType, err)
	}
	if checkResult != nil && !checkResult.Allowed {
		currentUsage := checkResult.Details.CurrentUsage
		usageLimit := uint64(0)
		if checkResult.Details.Limit != nil {
			usageLimit = *checkResult.Details.Limit
		}

		var usageErr error
		switch checkType {
		case CheckTypeUpload:
			usageErr = fmt.Errorf(uploadUsageErrTemplate, core.ErrUploadQuotaExceeded, currentUsage, requestedBytes, usageLimit)
		case CheckTypeStorage:
			usageErr = fmt.Errorf(storageUsageErrTemplate, core.ErrStorageQuotaExceeded, currentUsage, requestedBytes, usageLimit)
		case CheckTypeDownload:
			usageErr = fmt.Errorf(downloadUsageErrTemplate, core.ErrDownloadQuotaExceeded, currentUsage, requestedBytes, usageLimit)
		default:
			usageErr = fmt.Errorf(defaultUsageErrTemplate, checkType, currentUsage, requestedBytes, usageLimit)
		}

		ctx.Logger().Warn("Quota exceeded", zap.String("check_type", checkType), zap.Uint("user_id", userID), zap.Uint64("requested_bytes", requestedBytes), zap.Uint64("current_usage", currentUsage), zap.Uint64("usage_limit", usageLimit))
		checkResult.ReleaseReservation()
		return nil, usageErr
	}
	return checkResult, nil
}

// ReleaseReservations releases multiple quota reservations safely.
func ReleaseReservations(reservations ...*quotaCore.QuotaCheckResult) {
	for _, result := range reservations {
		if result != nil {
			result.ReleaseReservation()
		}
	}
}

func EmitStorageObjectPinned(cctx context.Context, ctx core.Context, pin *models.Pin, ip string, reservationID *string) {
	core.Fire(ctx, event.EVENT_STORAGE_OBJECT_PINNED, event.NewStorageObjectPinnedEvent(cctx, pin, ip, reservationID))
}

func EmitStorageObjectUnpinned(cctx context.Context, ctx core.Context, pin *models.Pin, ip string) {
	core.Fire(ctx, event.EVENT_STORAGE_OBJECT_UNPINNED, event.NewStorageObjectUnpinnedEvent(cctx, pin, ip))
}

func RecordUpload(cctx context.Context, ctx core.Context, userID uint, uploadID uint, bytes uint64, ip string) error {
	return WithQuotaService(cctx, ctx, func(qs quotaCore.QuotaService, c context.Context) error {
		return qs.RecordUpload(c, userID, uploadID, bytes, ip)
	})
}

func RecordDownload(cctx context.Context, ctx core.Context, userID uint, uploadID uint, bytes uint64, ip string) error {
	return WithQuotaService(cctx, ctx, func(qs quotaCore.QuotaService, c context.Context) error {
		return qs.RecordDownload(c, userID, uploadID, bytes, ip)
	})
}

func EnforceFundingTarget(cctx context.Context, ctx core.Context, userID uint) error {
	return core.WithService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE, func(qs pluginCore.QuotaService) error {
		return qs.EnforceFundingTarget(cctx, userID)
	})
}
