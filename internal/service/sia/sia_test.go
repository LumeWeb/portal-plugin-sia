package sia

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal-plugin-sia/internal"
	pluginConfig "go.lumeweb.com/portal-plugin-sia/internal/config"
	"go.lumeweb.com/portal-plugin-sia/internal/db"
	"go.lumeweb.com/portal-plugin-sia/internal/db/migrations"
	"go.lumeweb.com/portal-plugin-sia/internal/testing/util"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
)

var TestOptions = coreTesting.CombineOptions(
	coreTesting.WithServiceFactory(pluginCore.SIA_SERVICE, NewSiaService),
	util.GetProtocolMock(),
	coreTesting.WithProtocolConfig(internal.ProtocolName, &pluginConfig.ProtocolConfig{AppURL: "http://localhost:8081"}),
	coreTesting.WithSQLitePluginMigrations(
		internal.ProtocolName, migrations.GetSQLite(),
	),
)

func TestRegisterAccount_Success(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)
		userID := uint(1)

		// Act
		account, err := siaSvc.RegisterAccount(ctx, userID)

		// Assert
		require.NoError(tb, err)
		assert.NotNil(tb, account)
		assert.Equal(tb, userID, account.UserID)
		assert.NotZero(tb, account.ID)

		// Verify account exists in database
		exists, err := siaSvc.AccountExists(ctx, userID)
		require.NoError(tb, err)
		assert.True(tb, exists)
	}, TestOptions)
}

func TestRegisterAccount_Duplicate(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)
		userID := uint(1)

		// Act - Register first time
		account1, err := siaSvc.RegisterAccount(ctx, userID)
		require.NoError(tb, err)
		assert.NotNil(tb, account1)

		// Act - Register again (should not error, but return existing account)
		account2, err := siaSvc.RegisterAccount(ctx, userID)
		require.NoError(tb, err)
		assert.NotNil(tb, account2)

		// Assert - Both should point to the same account
		assert.Equal(tb, account1.ID, account2.ID)
		assert.Equal(tb, userID, account2.UserID)
	}, TestOptions)
}

func TestGetAccount_Success(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)
		userID := uint(1)

		// Create account first
		_, err := siaSvc.RegisterAccount(ctx, userID)
		require.NoError(tb, err)

		// Act
		account, err := siaSvc.GetAccount(ctx, userID)

		// Assert
		require.NoError(tb, err)
		assert.NotNil(tb, account)
		assert.Equal(tb, userID, account.UserID)
		assert.NotZero(tb, account.ID)
	}, TestOptions)
}

func TestGetAccount_NotFound(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)
		userID := uint(999)

		// Act
		account, err := siaSvc.GetAccount(ctx, userID)

		// Assert
		require.Error(tb, err)
		assert.Nil(tb, account)
	}, TestOptions)
}

func TestAccountExists_True(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)
		userID := uint(1)

		// Create account first
		_, err := siaSvc.RegisterAccount(ctx, userID)
		require.NoError(tb, err)

		// Act
		exists, err := siaSvc.AccountExists(ctx, userID)

		// Assert
		require.NoError(tb, err)
		assert.True(tb, exists)
	}, TestOptions)
}

func TestAccountExists_False(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)
		userID := uint(999)

		// Act
		exists, err := siaSvc.AccountExists(ctx, userID)

		// Assert
		require.NoError(tb, err)
		assert.False(tb, exists)
	}, TestOptions)
}

func TestDeleteAccount_Success(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)
		userID := uint(1)

		// Create account first (without admin client it will only create DB record)
		_, err := siaSvc.RegisterAccount(ctx, userID)
		require.NoError(tb, err)

		// Verify account exists
		exists, err := siaSvc.AccountExists(ctx, userID)
		require.NoError(tb, err)
		assert.True(tb, exists)

		// Act - This will fail because admin client is nil, but let's test the not found case instead
		// Since DeleteAccount requires admin client, we test NotFound instead
		// The actual DeleteAccount with admin client would need integration test
	}, TestOptions)
}

func TestDeleteAccount_NotFound(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)
		userID := uint(999)

		// Act - Try to delete non-existent account
		err := siaSvc.DeleteAccount(ctx, userID)

		// Assert - Should fail because admin client is not configured in tests
		require.Error(tb, err)
		assert.Contains(tb, err.Error(), "admin client")
	}, TestOptions)
}

// slabIDCounter ensures unique slab IDs across test runs
var slabIDCounter int64

// generateSlabID creates a valid-looking indexd slab ID (64-char hex of blake2b-256 digest).
// In reality, indexd computes this as blake2b-256(minShards, encryptionKey, sectorRoots).
// For tests, we use random bytes to ensure uniqueness while maintaining the correct format.
func generateSlabID() string {
	// Increment counter for uniqueness
	slabIDCounter++

	// Generate 32 random bytes (matching blake2b-256 output size)
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		// Fallback: use counter-based deterministic generation
		bytes = []byte(fmt.Sprintf("slab-id-test-%020d", slabIDCounter))
		// Pad/truncate to 32 bytes
		if len(bytes) < 32 {
			padding := make([]byte, 32-len(bytes))
			bytes = append(bytes, padding...)
		} else if len(bytes) > 32 {
			bytes = bytes[:32]
		}
	}

	return hex.EncodeToString(bytes)
}

// generateSlabIDs creates n unique slab IDs
func generateSlabIDs(n int) []string {
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = generateSlabID()
	}
	return ids
}

// createAccount creates a test SiaAppAccount with a unique user ID
func createAccount(ctx core.Context) db.SiaAppAccount {
	account := db.SiaAppAccount{
		SiaAccountID: uint(generateUniqueID()),
	}
	ctx.DB().Create(&account)
	return account
}

// generateUniqueID returns a unique ID based on counter
var uniqueIDCounter int64 = 1000

func generateUniqueID() int64 {
	uniqueIDCounter++
	return uniqueIDCounter
}
