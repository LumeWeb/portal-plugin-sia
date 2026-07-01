package sia

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"gorm.io/gorm"
)

// TestDeleteAppAccount_NotFound verifies that DeleteAppAccount returns
// gorm.ErrRecordNotFound when the public key does not match any app account
// for the given Sia account. This exercises the DB lookup query directly
// against a real SQLite database, ensuring the query references valid tables
// and columns.
func TestDeleteAppAccount_NotFound(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		pk := generateTestPublicKey()

		_, err = siaSvc.RegisterAppAccount(ctx, account.ID, pk)
		require.NoError(tb, err)

		// Look up a different key that was never registered
		otherKey := generateTestPublicKey()

		err = siaSvc.DeleteAppAccount(ctx, account.ID, otherKey)
		require.Error(tb, err)
		assert.ErrorIs(tb, err, gorm.ErrRecordNotFound)
	}, TestOptions)
}

// TestDeleteAppAccount_DBLookupSucceeds verifies that DeleteAppAccount finds
// the app account by its public key and proceeds past the DB lookup stage.
// Without an admin client configured (as in tests), the method returns an
// "admin client not configured" error — but this confirms the query executed
// successfully against the correct schema.
func TestDeleteAppAccount_DBLookupSucceeds(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		account, err := siaSvc.RegisterAccount(ctx, 1)
		require.NoError(tb, err)

		pk := generateTestPublicKey()

		_, err = siaSvc.RegisterAppAccount(ctx, account.ID, pk)
		require.NoError(tb, err)

		err = siaSvc.DeleteAppAccount(ctx, account.ID, pk)
		require.Error(tb, err)
		assert.Contains(tb, err.Error(), "admin client")
	}, TestOptions)
}
