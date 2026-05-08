package sia

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
)

func TestRegisterSlab_Success(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, generateTestPublicKey())
		require.NoError(tb, err)

		err = siaSvc.RegisterSlab(ctx, appAccount.ID, "abc123def456")
		require.NoError(tb, err)

		slabs, err := siaSvc.ListSlabsByAppAccount(ctx, appAccount.ID)
		require.NoError(tb, err)
		assert.Len(tb, slabs, 1)
		assert.Equal(tb, "abc123def456", slabs[0].SlabID)
		assert.Equal(tb, appAccount.ID, slabs[0].SiaAppAccountID)
	}, TestOptions)
}

func TestRegisterSlab_Idempotent(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, generateTestPublicKey())
		require.NoError(tb, err)

		err = siaSvc.RegisterSlab(ctx, appAccount.ID, "abc123def456")
		require.NoError(tb, err)

		err = siaSvc.RegisterSlab(ctx, appAccount.ID, "abc123def456")
		require.NoError(tb, err)

		slabs, err := siaSvc.ListSlabsByAppAccount(ctx, appAccount.ID)
		require.NoError(tb, err)
		assert.Len(tb, slabs, 1)
	}, TestOptions)
}

func TestRegisterSlab_DifferentSlabs(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, generateTestPublicKey())
		require.NoError(tb, err)

		err = siaSvc.RegisterSlab(ctx, appAccount.ID, "slab1")
		require.NoError(tb, err)

		err = siaSvc.RegisterSlab(ctx, appAccount.ID, "slab2")
		require.NoError(tb, err)

		slabs, err := siaSvc.ListSlabsByAppAccount(ctx, appAccount.ID)
		require.NoError(tb, err)
		assert.Len(tb, slabs, 2)
	}, TestOptions)
}

func TestDeleteSlab_Success(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, generateTestPublicKey())
		require.NoError(tb, err)

		err = siaSvc.RegisterSlab(ctx, appAccount.ID, "abc123def456")
		require.NoError(tb, err)

		err = siaSvc.DeleteSlab(ctx, appAccount.ID, "abc123def456")
		require.NoError(tb, err)

		slabs, err := siaSvc.ListSlabsByAppAccount(ctx, appAccount.ID)
		require.NoError(tb, err)
		assert.Len(tb, slabs, 0)
	}, TestOptions)
}

func TestDeleteSlab_NotFound(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, generateTestPublicKey())
		require.NoError(tb, err)

		err = siaSvc.DeleteSlab(ctx, appAccount.ID, "nonexistent")
		require.NoError(tb, err)
	}, TestOptions)
}

func TestDeleteSlab_OnlyDeletesTarget(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, generateTestPublicKey())
		require.NoError(tb, err)

		err = siaSvc.RegisterSlab(ctx, appAccount.ID, "slab1")
		require.NoError(tb, err)
		err = siaSvc.RegisterSlab(ctx, appAccount.ID, "slab2")
		require.NoError(tb, err)

		err = siaSvc.DeleteSlab(ctx, appAccount.ID, "slab1")
		require.NoError(tb, err)

		slabs, err := siaSvc.ListSlabsByAppAccount(ctx, appAccount.ID)
		require.NoError(tb, err)
		assert.Len(tb, slabs, 1)
		assert.Equal(tb, "slab2", slabs[0].SlabID)
	}, TestOptions)
}

func TestListSlabsByAppAccount_Empty(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, generateTestPublicKey())
		require.NoError(tb, err)

		slabs, err := siaSvc.ListSlabsByAppAccount(ctx, appAccount.ID)
		require.NoError(tb, err)
		assert.Len(tb, slabs, 0)
	}, TestOptions)
}

func TestListSlabsByAppAccount_IsolatedByAccount(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount1, err := siaSvc.RegisterAppAccount(ctx, account.ID, generateTestPublicKey())
		require.NoError(tb, err)
		appAccount2, err := siaSvc.RegisterAppAccount(ctx, account.ID, generateTestPublicKey())
		require.NoError(tb, err)

		err = siaSvc.RegisterSlab(ctx, appAccount1.ID, "shared-slab")
		require.NoError(tb, err)
		err = siaSvc.RegisterSlab(ctx, appAccount2.ID, "shared-slab")
		require.NoError(tb, err)

		slabs1, err := siaSvc.ListSlabsByAppAccount(ctx, appAccount1.ID)
		require.NoError(tb, err)
		assert.Len(tb, slabs1, 1)

		slabs2, err := siaSvc.ListSlabsByAppAccount(ctx, appAccount2.ID)
		require.NoError(tb, err)
		assert.Len(tb, slabs2, 1)
	}, TestOptions)
}

func TestDeleteSlabsByAppAccount_Success(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount, err := siaSvc.RegisterAppAccount(ctx, account.ID, generateTestPublicKey())
		require.NoError(tb, err)

		err = siaSvc.RegisterSlab(ctx, appAccount.ID, "slab1")
		require.NoError(tb, err)
		err = siaSvc.RegisterSlab(ctx, appAccount.ID, "slab2")
		require.NoError(tb, err)

		err = siaSvc.DeleteSlabsByAppAccount(ctx, appAccount.ID)
		require.NoError(tb, err)

		slabs, err := siaSvc.ListSlabsByAppAccount(ctx, appAccount.ID)
		require.NoError(tb, err)
		assert.Len(tb, slabs, 0)
	}, TestOptions)
}

func TestDeleteSlabsByAppAccount_DoesNotAffectOtherAccounts(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		appAccount1, err := siaSvc.RegisterAppAccount(ctx, account.ID, generateTestPublicKey())
		require.NoError(tb, err)
		appAccount2, err := siaSvc.RegisterAppAccount(ctx, account.ID, generateTestPublicKey())
		require.NoError(tb, err)

		err = siaSvc.RegisterSlab(ctx, appAccount1.ID, "slab1")
		require.NoError(tb, err)
		err = siaSvc.RegisterSlab(ctx, appAccount2.ID, "slab2")
		require.NoError(tb, err)

		err = siaSvc.DeleteSlabsByAppAccount(ctx, appAccount1.ID)
		require.NoError(tb, err)

		slabs2, err := siaSvc.ListSlabsByAppAccount(ctx, appAccount2.ID)
		require.NoError(tb, err)
		assert.Len(tb, slabs2, 1)
		assert.Equal(tb, "slab2", slabs2[0].SlabID)
	}, TestOptions)
}
