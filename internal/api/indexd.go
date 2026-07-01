package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	mcontext "go.lumeweb.com/portal-middleware/context"
	quotaCore "go.lumeweb.com/portal-plugin-quota/core"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal-plugin-sia/internal"
	"go.lumeweb.com/portal-plugin-sia/internal/quota"
	core "go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"go.sia.tech/core/rhp/v4"
	"go.sia.tech/core/types"
	"go.sia.tech/indexd/slabs"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

type pinResponseParser func(*responseRecorder) (pinResult, error)

type pinResult struct {
	storageHash core.StorageHash
	metadata    quotaCore.SlabMetadata
}

// alreadyPinned checks if the given hash is already pinned for the user,
// returning true if so. Used to skip duplicate recording on re-pin.
func (a *API) alreadyPinned(ctx context.Context, hash core.StorageHash, userID uint) bool {
	pinned, err := a.pinSvc.UploadPinnedByUser(ctx, hash, userID)
	if err != nil {
		a.Logger().Error("failed to check pin existence", zap.Error(err))
		return false
	}
	return pinned
}

func (a *API) pinAndRecord(c echo.Context, dataSize uint64, mimeType string, knownHash core.StorageHash, parseResponse pinResponseParser) error {
	userID, err := mcontext.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	ctx := c.Request().Context()

	if knownHash != nil && a.alreadyPinned(ctx, knownHash, userID) {
		a.proxyHandler(c)
		return nil
	}

	// Objects are virtual (dataSize=0): skip quota check and event emission.
	// Storage is tracked via the slab pin, not the object pin.
	var result *quotaCore.QuotaCheckResult
	var reservationID *string
	if dataSize > 0 {
		result, err = quota.CheckWithReservation(ctx, a.Context(), quota.CheckTypeStorage, userID, dataSize, quota.CheckStorageQuota)
		if err != nil {
			return echo.NewHTTPError(http.StatusPaymentRequired, err.Error())
		}

		if result != nil && result.Reservation != nil {
			rid := result.Reservation.UUID()
			reservationID = &rid
		}
	}

	recorder := newResponseRecorder(c.Response().Writer)
	c.Response().Writer = recorder

	a.proxyHandler(c)

	if recorder.statusCode >= 400 {
		quota.ReleaseReservations(result)
		return nil
	}

	pr, err := parseResponse(recorder)
	if err != nil {
		quota.ReleaseReservations(result)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if a.alreadyPinned(ctx, pr.storageHash, userID) {
		quota.ReleaseReservations(result)
		return nil
	}

	metadataJSON, err := quotaCore.EncodeSlabMetadata(datatypes.JSON{}, pr.metadata)
	if err != nil {
		a.Logger().Error("failed to encode slab metadata", zap.Error(err))
		quota.ReleaseReservations(result)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to encode metadata")
	}

	upload := &models.Upload{
		UserID:     userID,
		Hash:       pr.storageHash.Multihash(),
		CIDType:    pr.storageHash.CIDType(),
		MimeType:   mimeType,
		Protocol:   internal.ProtocolName,
		Size:       dataSize,
		Metadata:   metadataJSON,
		UploaderIP: c.RealIP(),
	}

	if err := a.uploadSvc.SaveUpload(ctx, upload); err != nil {
		a.Logger().Error("failed to save upload", zap.Error(err))
		quota.ReleaseReservations(result)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to record upload")
	}

	pinSvc := a.pinSvc
	pin := &models.Pin{
		UserID:   userID,
		UploadID: upload.ID,
	}
	createdPin, err := pinSvc.CreatePin(ctx, pin, nil)
	if err != nil {
		a.Logger().Error("failed to create pin", zap.Error(err))
	} else {
		// Skip storage event for virtual object pins (dataSize=0).
		// Storage is tracked via the slab pin, not the object pin.
		if dataSize > 0 {
			quota.EmitStorageObjectPinned(ctx, a.Context(), createdPin, c.RealIP(), reservationID)
		}
		if err := quota.EnforceFundingTarget(core.DetachContext(ctx), a.Context(), userID); err != nil {
			a.Logger().Error("failed to enforce funding target", zap.Error(err))
		}
	}

	return nil
}

func (a *API) pinSlabHandler(c echo.Context) error {
	ctx, span := core.TraceMethod(c.Request().Context(), "API.pinSlabHandler")
	defer span.End()
	c.SetRequest(c.Request().WithContext(ctx))

	timer := prometheus.NewTimer(PinSlabDuration.WithLabelValues())
	defer timer.ObserveDuration()

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		PinSlabTotal.WithLabelValues(LabelStatusError).Inc()
		return echo.NewHTTPError(http.StatusBadRequest, "failed to read request body")
	}

	var params []slabs.SlabPinParams
	if err := json.Unmarshal(body, &params); err != nil {
		PinSlabTotal.WithLabelValues(LabelStatusError).Inc()
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	var totalDataSize, totalTotalSize uint64
	var totalMinShards uint64
	var totalSectors uint64

	for _, sp := range params {
		dataSize := uint64(sp.MinShards) * uint64(rhp.SectorSize)
		sectorCount := len(sp.Sectors)
		totalDataSize += dataSize
		totalTotalSize += uint64(sectorCount) * uint64(rhp.SectorSize)
		totalMinShards += uint64(sp.MinShards)
		totalSectors += uint64(sectorCount)
	}

	c.Request().Body = io.NopCloser(bytes.NewReader(body))

	err = a.pinAndRecord(c, totalDataSize, pluginCore.MimeTypeSiaSlab, nil, func(recorder *responseRecorder) (pinResult, error) {
		var slabIDs []slabs.SlabID
		if err := json.Unmarshal(recorder.body.Bytes(), &slabIDs); err != nil || len(slabIDs) == 0 {
			return pinResult{}, fmt.Errorf("failed to parse slab ID from response")
		}

		if appAccountID, ok := getSiaAppAccountID(c); ok {
			for _, sid := range slabIDs {
				if err := a.siaSvc.RegisterSlab(ctx, appAccountID, sid.String()); err != nil {
					a.Logger().Error("failed to register slab", zap.String("slabID", sid.String()), zap.Error(err))
				}
			}
		}

		storageHash, err := internal.NewSiaHash(slabIDs[0].String())
		if err != nil {
			return pinResult{}, fmt.Errorf("invalid slab ID: %w", err)
		}

		return pinResult{
			storageHash: storageHash,
			metadata: quotaCore.SlabMetadata{
				MinShards:    totalMinShards,
				TotalSectors: totalSectors,
				DataSize:     totalDataSize,
				TotalSize:    totalTotalSize,
			},
		}, nil
	})

	if err != nil {
		PinSlabTotal.WithLabelValues(LabelStatusError).Inc()
		return err
	}

	PinSlabTotal.WithLabelValues(LabelStatusSuccess).Inc()
	return nil
}

func (a *API) pinObjectHandler(c echo.Context) error {
	ctx, span := core.TraceMethod(c.Request().Context(), "API.pinObjectHandler")
	defer span.End()
	c.SetRequest(c.Request().WithContext(ctx))

	timer := prometheus.NewTimer(PinObjectDuration.WithLabelValues())
	defer timer.ObserveDuration()

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		PinObjectTotal.WithLabelValues(LabelStatusError).Inc()
		return echo.NewHTTPError(http.StatusBadRequest, "failed to read request body")
	}

	var req slabs.PinObjectRequest
	if err := json.Unmarshal(body, &req); err != nil {
		PinObjectTotal.WithLabelValues(LabelStatusError).Inc()
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	c.Request().Body = io.NopCloser(bytes.NewReader(body))

	objectHash, hashErr := internal.NewSiaHash(req.ID.String())
	if hashErr != nil {
		return hashErr
	}

	// Objects are virtual: they reference slabs that hold the real data.
	// Storage quota is tracked via the slab pin, not the object pin.
	// Record 0 bytes to avoid double-counting.
	err = a.pinAndRecord(c, 0, pluginCore.MimeTypeSiaObject, objectHash, func(recorder *responseRecorder) (pinResult, error) {
		// Register object and its slab relationships after successful proxy
		if appAccountID, ok := getSiaAppAccountID(c); ok {
			slabIDs := make([]string, 0, len(req.Slabs))
			for _, s := range req.Slabs {
				slabIDs = append(slabIDs, s.ID.String())
			}
			if err := a.siaSvc.RegisterObject(ctx, appAccountID, req.ID.String(), slabIDs); err != nil {
				a.Logger().Error("failed to register object",
					zap.String("objectID", req.ID.String()),
					zap.Error(err))
			}
		}

		objectStorageHash, innerErr := internal.NewSiaHash(req.ID.String())
		if innerErr != nil {
			return pinResult{}, fmt.Errorf("invalid object ID: %w", innerErr)
		}

		totalDataSize := uint64(0)
		for _, slab := range req.Slabs {
			totalDataSize += uint64(slab.Length)
		}

		return pinResult{
			storageHash: objectStorageHash,
			metadata: quotaCore.SlabMetadata{
				MinShards:    0,
				TotalSectors: uint64(len(req.Slabs)),
				DataSize:     totalDataSize,
				TotalSize:    totalDataSize,
			},
		}, nil
	})

	if err != nil {
		PinObjectTotal.WithLabelValues(LabelStatusError).Inc()
		return err
	}

	PinObjectTotal.WithLabelValues(LabelStatusSuccess).Inc()
	return nil
}

func (a *API) withRecordingProxy(c echo.Context, onSuccess func(userID uint)) {
	recorder := newResponseRecorder(c.Response().Writer)
	c.Response().Writer = recorder

	a.proxyHandler(c)

	if recorder.statusCode >= 400 {
		return
	}

	userID, err := mcontext.GetUserID(c)
	if err != nil {
		return
	}

	onSuccess(userID)
}

func (a *API) pruneSlabsHandler(c echo.Context) error {
	ctx, span := core.TraceMethod(c.Request().Context(), "API.pruneSlabsHandler")
	defer span.End()
	c.SetRequest(c.Request().WithContext(ctx))

	recorder := newResponseRecorder(c.Response().Writer)
	c.Response().Writer = recorder

	a.proxyHandler(c)

	// Only proceed with local cleanup if indexd prune succeeded
	if recorder.statusCode >= 400 {
		return nil
	}

	// Delegate to SiaService for full prune logic (testable service layer)
	if appAccountID, ok := getSiaAppAccountID(c); ok {
		if err := a.siaSvc.PruneSlabs(ctx, appAccountID); err != nil {
			a.Logger().Error("failed to prune slabs",
				zap.Uint("appAccountID", appAccountID),
				zap.Error(err))
		}
	}

	return nil
}

func (a *API) unpinSlabHandler(c echo.Context) error {
	ctx, span := core.TraceMethod(c.Request().Context(), "API.unpinSlabHandler")
	defer span.End()
	c.SetRequest(c.Request().WithContext(ctx))

	timer := prometheus.NewTimer(UnpinSlabDuration.WithLabelValues())
	defer timer.ObserveDuration()

	ctx2 := c.Request().Context()
	a.withRecordingProxy(c, func(userID uint) {
		slabIDStr := c.Param("id")
		var slabID slabs.SlabID
		if err := slabID.UnmarshalText([]byte(slabIDStr)); err != nil {
			a.deletePinAndUpload(ctx2, slabIDStr, userID, c)
			return
		}

		if appAccountID, ok := getSiaAppAccountID(c); ok {
			if err := a.siaSvc.DeleteSlab(ctx2, appAccountID, slabID.String()); err != nil {
				a.Logger().Error("failed to delete slab record", zap.String("slabID", slabID.String()), zap.Error(err))
			}
		}

		a.deletePinAndUpload(ctx2, slabID.String(), userID, c)
		UnpinSlabTotal.WithLabelValues(LabelStatusSuccess).Inc()
	})

	return nil
}

func (a *API) unpinObjectHandler(c echo.Context) error {
	ctx, span := core.TraceMethod(c.Request().Context(), "API.unpinObjectHandler")
	defer span.End()
	c.SetRequest(c.Request().WithContext(ctx))

	timer := prometheus.NewTimer(UnpinObjectDuration.WithLabelValues())
	defer timer.ObserveDuration()

	ctx2 := c.Request().Context()
	a.withRecordingProxy(c, func(userID uint) {
		objKey := c.Param("key")
		var objID types.Hash256
		if err := objID.UnmarshalText([]byte(objKey)); err != nil {
			a.deletePinAndUpload(ctx2, objKey, userID, c)
			return
		}

		// Delete object record before pin/upload cleanup
		if appAccountID, ok := getSiaAppAccountID(c); ok {
			if err := a.siaSvc.DeleteObject(ctx2, appAccountID, objID.String()); err != nil {
				a.Logger().Error("failed to delete object record",
					zap.String("objectID", objID.String()),
					zap.Error(err))
			}
		}

		a.deletePinAndUpload(ctx2, objID.String(), userID, c)
		UnpinObjectTotal.WithLabelValues(LabelStatusSuccess).Inc()
	})

	return nil
}

func (a *API) deletePinAndUpload(ctx context.Context, hashStr string, userID uint, c echo.Context) error {
	storageHash, err := internal.NewSiaHash(hashStr)
	if err != nil {
		return fmt.Errorf("invalid hash: %w", err)
	}

	pin, err := a.pinSvc.GetPinByHash(ctx, storageHash, userID)
	if err != nil {
		a.Logger().Error("failed to get pin for deletion",
			zap.String("hash", hashStr),
			zap.Error(err))
		return nil
	}
	if pin == nil {
		a.Logger().Debug("pin not found for deletion, already removed",
			zap.String("hash", hashStr))
		return nil
	}

	upload, err := a.uploadSvc.GetUploadByID(ctx, pin.UploadID)
	if err != nil {
		a.Logger().Error("failed to get upload for pin deletion",
			zap.String("hash", hashStr),
			zap.Error(err))
		return nil
	}
	if upload == nil {
		a.Logger().Debug("upload not found for pin deletion, already removed",
			zap.String("hash", hashStr))
		return nil
	}

	if err := a.pinSvc.DeletePin(ctx, pin.ID); err != nil {
		a.Logger().Error("failed to delete pin", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to clean up records")
	}

	if err := a.uploadSvc.DeleteUpload(ctx, internal.StorageHashFromMultihash(upload.Hash)); err != nil {
		a.Logger().Error("failed to delete upload", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to clean up records")
	}

	// Skip storage event for virtual object pins (Size=0).
	// Legacy object pins may have Size>0 and need quota release.
	if upload.Size > 0 {
		quota.EmitStorageObjectUnpinned(c.Request().Context(), a.Context(), pin, c.RealIP())
	}
	if err := quota.EnforceFundingTarget(core.DetachContext(ctx), a.Context(), userID); err != nil {
		a.Logger().Error("failed to enforce funding target", zap.Error(err))
	}
	return nil
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}
