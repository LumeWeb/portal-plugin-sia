package sia

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	coreMocks "go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

func TestRegisterObject_Success(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, "test-key")
		require.NoError(tb, err)

		slab1 := generateSlabID()
		slab2 := generateSlabID()
		err = siaSvc.RegisterSlab(ctx, appAccount.ID, slab1)
		require.NoError(tb, err)
		err = siaSvc.RegisterSlab(ctx, appAccount.ID, slab2)
		require.NoError(tb, err)

		err = siaSvc.RegisterObject(ctx, appAccount.ID, "obj1", []string{slab1, slab2})
		require.NoError(tb, err)
	}, TestOptions)
}

func TestRegisterObject_Idempotent(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, "test-key")
		require.NoError(tb, err)

		slab1 := generateSlabID()
		err = siaSvc.RegisterSlab(ctx, appAccount.ID, slab1)
		require.NoError(tb, err)

		err = siaSvc.RegisterObject(ctx, appAccount.ID, "obj1", []string{slab1})
		require.NoError(tb, err)

		err = siaSvc.RegisterObject(ctx, appAccount.ID, "obj1", []string{slab1})
		require.NoError(tb, err)
	}, TestOptions)
}

func TestRegisterObject_SkipsUnknownSlabs(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, "test-key")
		require.NoError(tb, err)

		slab1 := generateSlabID()
		err = siaSvc.RegisterSlab(ctx, appAccount.ID, slab1)
		require.NoError(tb, err)

		err = siaSvc.RegisterObject(ctx, appAccount.ID, "obj1", []string{slab1, "unknown-slab"})
		require.NoError(tb, err)
	}, TestOptions)
}

func TestDeleteObject_Success(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, "test-key")
		require.NoError(tb, err)

		slab1 := generateSlabID()
		err = siaSvc.RegisterSlab(ctx, appAccount.ID, slab1)
		require.NoError(tb, err)

		err = siaSvc.RegisterObject(ctx, appAccount.ID, "obj1", []string{slab1})
		require.NoError(tb, err)

		err = siaSvc.DeleteObject(ctx, appAccount.ID, "obj1")
		require.NoError(tb, err)

		orphaned, err := siaSvc.FindOrphanedSlabs(ctx, appAccount.ID)
		require.NoError(tb, err)
		assert.Contains(tb, orphaned, slab1)
	}, TestOptions)
}

func TestDeleteObject_NotFound(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, "test-key")
		require.NoError(tb, err)

		err = siaSvc.DeleteObject(ctx, appAccount.ID, "nonexistent")
		require.NoError(tb, err)
	}, TestOptions)
}

func TestDeleteObject_OnlyDeletesTarget(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, "test-key")
		require.NoError(tb, err)

		slab1 := generateSlabID()
		slab2 := generateSlabID()
		err = siaSvc.RegisterSlab(ctx, appAccount.ID, slab1)
		require.NoError(tb, err)
		err = siaSvc.RegisterSlab(ctx, appAccount.ID, slab2)
		require.NoError(tb, err)

		err = siaSvc.RegisterObject(ctx, appAccount.ID, "obj1", []string{slab1})
		require.NoError(tb, err)
		err = siaSvc.RegisterObject(ctx, appAccount.ID, "obj2", []string{slab2})
		require.NoError(tb, err)

		err = siaSvc.DeleteObject(ctx, appAccount.ID, "obj1")
		require.NoError(tb, err)

		orphaned, err := siaSvc.FindOrphanedSlabs(ctx, appAccount.ID)
		require.NoError(tb, err)
		assert.Contains(tb, orphaned, slab1)
		assert.NotContains(tb, orphaned, slab2)
	}, TestOptions)
}

func TestDeleteObjectsByAppAccount_Success(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, "test-key")
		require.NoError(tb, err)

		slab1 := generateSlabID()
		slab2 := generateSlabID()
		err = siaSvc.RegisterSlab(ctx, appAccount.ID, slab1)
		require.NoError(tb, err)
		err = siaSvc.RegisterSlab(ctx, appAccount.ID, slab2)
		require.NoError(tb, err)

		err = siaSvc.RegisterObject(ctx, appAccount.ID, "obj1", []string{slab1})
		require.NoError(tb, err)
		err = siaSvc.RegisterObject(ctx, appAccount.ID, "obj2", []string{slab2})
		require.NoError(tb, err)

		err = siaSvc.DeleteObjectsByAppAccount(ctx, appAccount.ID)
		require.NoError(tb, err)

		orphaned, err := siaSvc.FindOrphanedSlabs(ctx, appAccount.ID)
		require.NoError(tb, err)
		assert.Len(tb, orphaned, 2)
	}, TestOptions)
}

func TestDeleteObjectsByAppAccount_DoesNotAffectOtherAccounts(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount1, err := siaSvc.RegisterAppAccount(ctx, account.ID, "key1")
		require.NoError(tb, err)
		appAccount2, err := siaSvc.RegisterAppAccount(ctx, account.ID, "key2")
		require.NoError(tb, err)

		slab1 := generateSlabID()
		slab2 := generateSlabID()
		err = siaSvc.RegisterSlab(ctx, appAccount1.ID, slab1)
		require.NoError(tb, err)
		err = siaSvc.RegisterSlab(ctx, appAccount2.ID, slab2)
		require.NoError(tb, err)

		err = siaSvc.RegisterObject(ctx, appAccount1.ID, "obj1", []string{slab1})
		require.NoError(tb, err)
		err = siaSvc.RegisterObject(ctx, appAccount2.ID, "obj2", []string{slab2})
		require.NoError(tb, err)

		err = siaSvc.DeleteObjectsByAppAccount(ctx, appAccount1.ID)
		require.NoError(tb, err)

		orphaned2, err := siaSvc.FindOrphanedSlabs(ctx, appAccount2.ID)
		require.NoError(tb, err)
		assert.Len(tb, orphaned2, 0)
	}, TestOptions)
}

func TestFindOrphanedSlabs_None(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, "test-key")
		require.NoError(tb, err)

		slab1 := generateSlabID()
		err = siaSvc.RegisterSlab(ctx, appAccount.ID, slab1)
		require.NoError(tb, err)
		err = siaSvc.RegisterObject(ctx, appAccount.ID, "obj1", []string{slab1})
		require.NoError(tb, err)

		orphaned, err := siaSvc.FindOrphanedSlabs(ctx, appAccount.ID)
		require.NoError(tb, err)
		assert.Len(tb, orphaned, 0)
	}, TestOptions)
}

func TestFindOrphanedSlabs_AfterObjectDeleted(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, "test-key")
		require.NoError(tb, err)

		slab1 := generateSlabID()
		err = siaSvc.RegisterSlab(ctx, appAccount.ID, slab1)
		require.NoError(tb, err)
		err = siaSvc.RegisterObject(ctx, appAccount.ID, "obj1", []string{slab1})
		require.NoError(tb, err)

		err = siaSvc.DeleteObject(ctx, appAccount.ID, "obj1")
		require.NoError(tb, err)

		orphaned, err := siaSvc.FindOrphanedSlabs(ctx, appAccount.ID)
		require.NoError(tb, err)
		assert.Contains(tb, orphaned, slab1)
	}, TestOptions)
}

func TestFindOrphanedSlabs_SharedSlabNotOrphanedIfOtherObjectRefs(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, "test-key")
		require.NoError(tb, err)

		slab1 := generateSlabID()
		err = siaSvc.RegisterSlab(ctx, appAccount.ID, slab1)
		require.NoError(tb, err)

		err = siaSvc.RegisterObject(ctx, appAccount.ID, "obj1", []string{slab1})
		require.NoError(tb, err)
		err = siaSvc.RegisterObject(ctx, appAccount.ID, "obj2", []string{slab1})
		require.NoError(tb, err)

		err = siaSvc.DeleteObject(ctx, appAccount.ID, "obj1")
		require.NoError(tb, err)

		orphaned, err := siaSvc.FindOrphanedSlabs(ctx, appAccount.ID)
		require.NoError(tb, err)
		assert.NotContains(tb, orphaned, slab1)
	}, TestOptions)
}

func TestPruneSlabs_FullyOrphaned_DeletesPinAndUpload(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, "test-key")
		require.NoError(tb, err)

		slab1 := generateSlabID()
		err = siaSvc.RegisterSlab(ctx, appAccount.ID, slab1)
		require.NoError(tb, err)
		err = siaSvc.RegisterObject(ctx, appAccount.ID, "obj1", []string{slab1})
		require.NoError(tb, err)

		mockPinSvc := core.GetService[*coreMocks.MockPinService](ctx, core.PIN_SERVICE)
		mockUploadSvc := core.GetService[*coreMocks.MockUploadService](ctx, core.UPLOAD_SERVICE)

		mockPinSvc.EXPECT().UploadPinnedGlobal(mock.Anything, storageHashMatcher()).Return(false, nil)
		mockPinSvc.EXPECT().GetAllPinsByHash(mock.Anything, storageHashMatcher()).Return([]*models.Pin{
			{Model: gorm.Model{ID: 1}, UserID: 1, UploadID: 1},
		}, nil)
		mockPinSvc.EXPECT().DeletePin(mock.Anything, mock.AnythingOfType("uint")).Return(nil)
		mockUploadSvc.EXPECT().DeleteUpload(mock.Anything, storageHashMatcher()).Return(nil)

		err = siaSvc.PruneSlabs(ctx, appAccount.ID)
		require.NoError(tb, err)

		slabs, err := siaSvc.ListSlabsByAppAccount(ctx, appAccount.ID)
		require.NoError(tb, err)
		assert.Len(tb, slabs, 0)
	}, TestOptions)
}

func TestPruneSlabs_StillPinnedGlobally_SkipsDeletion(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, "test-key")
		require.NoError(tb, err)

		slab1 := generateSlabID()
		err = siaSvc.RegisterSlab(ctx, appAccount.ID, slab1)
		require.NoError(tb, err)
		err = siaSvc.RegisterObject(ctx, appAccount.ID, "obj1", []string{slab1})
		require.NoError(tb, err)

		mockPinSvc := core.GetService[*coreMocks.MockPinService](ctx, core.PIN_SERVICE)
		mockPinSvc.EXPECT().UploadPinnedGlobal(mock.Anything, storageHashMatcher()).Return(true, nil)

		err = siaSvc.PruneSlabs(ctx, appAccount.ID)
		require.NoError(tb, err)

		slabs, err := siaSvc.ListSlabsByAppAccount(ctx, appAccount.ID)
		require.NoError(tb, err)
		assert.Len(tb, slabs, 0)
	}, TestOptions)
}

func TestPruneSlabs_SharedAcrossAppAccounts_SkipsDeletion(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount1, err := siaSvc.RegisterAppAccount(ctx, account.ID, "key1")
		require.NoError(tb, err)
		appAccount2, err := siaSvc.RegisterAppAccount(ctx, account.ID, "key2")
		require.NoError(tb, err)

		slab1 := generateSlabID()
		err = siaSvc.RegisterSlab(ctx, appAccount1.ID, slab1)
		require.NoError(tb, err)
		err = siaSvc.RegisterSlab(ctx, appAccount2.ID, slab1)
		require.NoError(tb, err)

		err = siaSvc.RegisterObject(ctx, appAccount1.ID, "obj1", []string{slab1})
		require.NoError(tb, err)
		err = siaSvc.RegisterObject(ctx, appAccount2.ID, "obj2", []string{slab1})
		require.NoError(tb, err)

		err = siaSvc.PruneSlabs(ctx, appAccount1.ID)
		require.NoError(tb, err)

		slabs2, err := siaSvc.ListSlabsByAppAccount(ctx, appAccount2.ID)
		require.NoError(tb, err)
		assert.Len(tb, slabs2, 1)
		assert.Equal(tb, slab1, slabs2[0].SlabID)
	}, TestOptions)
}

func TestPruneSlabs_NoOrphans_NoError(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, "test-key")
		require.NoError(tb, err)

		slab1 := generateSlabID()
		err = siaSvc.RegisterSlab(ctx, appAccount.ID, slab1)
		require.NoError(tb, err)
		err = siaSvc.RegisterObject(ctx, appAccount.ID, "obj1", []string{slab1})
		require.NoError(tb, err)

		mockPinSvc := core.GetService[*coreMocks.MockPinService](ctx, core.PIN_SERVICE)
		mockUploadSvc := core.GetService[*coreMocks.MockUploadService](ctx, core.UPLOAD_SERVICE)
		// Slab becomes orphaned after object deleted; check shows not pinned globally
		mockPinSvc.EXPECT().UploadPinnedGlobal(mock.Anything, storageHashMatcher()).Return(false, nil)
		// No pins exist for this hash
		mockPinSvc.EXPECT().GetAllPinsByHash(mock.Anything, storageHashMatcher()).Return([]*models.Pin{}, nil)
		// Delete the upload
		mockUploadSvc.EXPECT().DeleteUpload(mock.Anything, storageHashMatcher()).Return(nil)

		err = siaSvc.PruneSlabs(ctx, appAccount.ID)
		require.NoError(tb, err)

		slabs, err := siaSvc.ListSlabsByAppAccount(ctx, appAccount.ID)
		require.NoError(tb, err)
		assert.Len(tb, slabs, 0)
	}, TestOptions)
}
