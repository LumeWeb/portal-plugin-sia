package api

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	coreMocks "go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/event"
	"go.sia.tech/indexd/slabs"
)

// TestPinObject_RecordsZeroStorage verifies that pinning an object records 0
// bytes for storage quota and does NOT emit a storage pin event. Objects are
// virtual: they reference slabs that hold the real data. Storage is tracked
// via the slab pin, not the object pin.
func TestPinObject_RecordsZeroStorage(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)

		var capturedUpload *models.Upload
		mockUploadSvc := core.GetService[*coreMocks.MockUploadService](ctx, core.UPLOAD_SERVICE)
		mockUploadSvc.EXPECT().SaveUpload(
			mock.Anything,
			mock.AnythingOfType("*models.Upload"),
		).Run(func(ctx context.Context, upload *models.Upload) {
			capturedUpload = upload
		}).Return(nil).Once()

		// Register an event listener to verify the storage event is NOT fired.
		var eventFired int32
		core.Listen(ctx, event.EVENT_STORAGE_OBJECT_PINNED, func(_ *core.CoreEvent[event.StorageObjectPinnedEvent]) error {
			atomic.StoreInt32(&eventFired, 1)
			return nil
		})

		sk, _ := helper.setupSignedAccount()

		req := newTestPinObjectRequest(t, sk)
		reqBody := mustMarshalJSON(t, req)
		resp := helper.makeSignedRequest(http.MethodPost, "/objects", sk, reqBody)

		assert.Equal(t, http.StatusNoContent, resp.Code)
		assert.NotNil(t, capturedUpload, "SaveUpload should have been called")
		assert.Equal(t, uint64(0), capturedUpload.Size,
			"object pin upload must record 0 bytes to avoid double-counting storage")
		assert.Equal(t, "application/x-sia-object", capturedUpload.MimeType)
		assert.Equal(t, int32(0), atomic.LoadInt32(&eventFired),
			"storage pin event must NOT be fired for object pins")
	}, TestOptions)
}

// TestPinSlab_RecordsNonZeroStorage verifies that pinning a slab records
// non-zero bytes for storage quota and DOES emit a storage pin event.
func TestPinSlab_RecordsNonZeroStorage(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)

		var capturedUpload *models.Upload
		mockUploadSvc := core.GetService[*coreMocks.MockUploadService](ctx, core.UPLOAD_SERVICE)
		mockUploadSvc.EXPECT().SaveUpload(
			mock.Anything,
			mock.AnythingOfType("*models.Upload"),
		).Run(func(ctx context.Context, upload *models.Upload) {
			capturedUpload = upload
		}).Return(nil).Once()

		// Register an event listener to verify the storage event IS fired.
		var eventFired int32
		core.Listen(ctx, event.EVENT_STORAGE_OBJECT_PINNED, func(_ *core.CoreEvent[event.StorageObjectPinnedEvent]) error {
			atomic.StoreInt32(&eventFired, 1)
			return nil
		})

		sk, _ := helper.setupSignedAccount()

		params := newTestSlabPinParams(t)
		reqBody := mustMarshalJSON(t, []slabs.SlabPinParams{params})
		resp := helper.makeSignedRequest(http.MethodPost, "/slabs", sk, reqBody)

		assert.Equal(t, http.StatusOK, resp.Code)
		assert.NotNil(t, capturedUpload, "SaveUpload should have been called")
		assert.Greater(t, capturedUpload.Size, uint64(0),
			"slab pin upload must record non-zero bytes (real storage)")
		assert.Equal(t, "application/x-sia-slab", capturedUpload.MimeType)
		assert.Equal(t, int32(1), atomic.LoadInt32(&eventFired),
			"storage pin event must be fired for slab pins")
	}, TestOptions)
}
