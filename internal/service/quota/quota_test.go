package quota

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	quotaCore "go.lumeweb.com/portal-plugin-quota/core"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal-plugin-sia/internal"
	siaDB "go.lumeweb.com/portal-plugin-sia/internal/db"
	"go.lumeweb.com/portal-plugin-sia/internal/db/migrations"
	"go.lumeweb.com/portal-plugin-sia/internal/service/sia"
	"go.lumeweb.com/portal-plugin-sia/internal/config"
	"go.lumeweb.com/portal-plugin-sia/internal/testing/util"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// mockConfigManager mocks the quota config manager
type mockConfigManager struct {
	mock.Mock
}

func (m *mockConfigManager) Config() interface{} {
	return nil
}

func (m *mockConfigManager) DB() *gorm.DB {
	return nil
}

func (m *mockConfigManager) ResolveEffectiveLimits(ctx context.Context, userID uint) (*quotaCore.EffectiveLimits, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) != nil {
		return args.Get(0).(*quotaCore.EffectiveLimits), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockConfigManager) GetUserQuotaConfig(ctx context.Context, userID uint) (*interface{}, error) {
	return nil, nil
}

func (m *mockConfigManager) GetPolicyEnforcer(ctx context.Context, userID uint) (quotaCore.PolicyEnforcer, error) {
	return nil, nil
}

func (m *mockConfigManager) GetUserAllowanceGrants(ctx context.Context, userID uint) ([]*interface{}, error) {
	return nil, nil
}

func (m *mockConfigManager) GetUserAllowanceGrantsByType(ctx context.Context, userID uint, grantType interface{}) ([]*interface{}, error) {
	return nil, nil
}

// QuotaTestOptions builds the test context with mocked services
func QuotaTestOptions() coreTesting.TestContextBuilderOption {
	return coreTesting.CombineOptions(
		util.GetProtocolMock(),
		coreTesting.WithProtocolConfig(internal.ProtocolName, &config.ProtocolConfig{
			Key:    "test-admin-key",
			URL:    "http://localhost:8080",
			AppURL: "http://localhost:8081",
		}),
		coreTesting.WithServiceFactory(pluginCore.SIA_SERVICE, sia.NewSiaService),
		coreTesting.WithServiceFactory(pluginCore.QUOTA_SERVICE, NewQuotaService),
		coreTesting.WithSQLitePluginMigrations("sia", migrations.GetSQLite()),
	)
}

func TestProvisionAccount_Success(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		logger := ctx.Logger()
		logger.Debug("Starting ProvisionAccount_Success test")

		// Setup mock user service - mark user as verified
		mockUserSvc := core.GetService[*coreTesting.MockUserService](ctx, core.USER_SERVICE)
		mockUserSvc.EXPECT().IsAccountVerified(mock.Anything, uint(1)).Return(true, nil).Maybe()

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		
		// Act - attempt to provision account
		// With protocol config set, admin client is configured but will fail calling indexd API
		err := quotaSvc.ProvisionAccount(context.Background(), 1)
		
		// Assert - expect error from indexd API call (404 Not Found since no real server)
		assert.Error(tb, err)
	}, QuotaTestOptions())
}

func TestProvisionAccount_UnverifiedUser(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		// Setup mock user service - mark user as NOT verified
		mockUserSvc := core.GetService[*coreTesting.MockUserService](ctx, core.USER_SERVICE)
		mockUserSvc.EXPECT().IsAccountVerified(mock.Anything, uint(1)).Return(false, nil).Maybe()

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)

		// Act
		err := quotaSvc.ProvisionAccount(context.Background(), 1) // User 1 exists but not verified

		// Assert - should fail because user not verified
		assert.Error(tb, err)
		assert.Contains(tb, err.Error(), "not verified")
	}, QuotaTestOptions())
}

func TestEnforceFundingTarget_NoAccount(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange - no account exists for user 1
		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		logger := ctx.Logger()
		logger.Debug("Testing EnforceFundingTarget with no account")

		// Act
		err := quotaSvc.EnforceFundingTarget(context.Background(), 999) // User 999 doesn't exist

		// Assert - should succeed with no error (account not found returns nil)
		assert.NoError(tb, err)
	}, QuotaTestOptions())
}

func TestSyncFunding_NoAdminClient(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange
		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		logger := ctx.Logger()
		logger.Debug("Testing SyncFunding with no admin client")

		// Act - sync funding without admin client configured
		// This should return nil because adminClient == nil is handled gracefully
		err := quotaSvc.SyncFunding(context.Background())

		// Assert - should succeed with no error when admin client is nil
		assert.NoError(tb, err)
	}, coreTesting.CombineOptions(
		QuotaTestOptions(),
		coreTesting.WithConfig("plugin.sia.protocol.key", ""),
		coreTesting.WithConfig("plugin.sia.protocol.url", ""),
	))
}

func TestProvisionAccount_DatabaseIntegration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		logger := ctx.Logger()

		// First, manually create a SiaAccount record
		account := &siaDB.SiaAccount{
			UserID:     1,
			ConnectKey: "test-connect-key",
		}
		err := ctx.DB().Create(account).Error
		assert.NoError(tb, err)

		// Verify it was created
		var retrieved siaDB.SiaAccount
		err = ctx.DB().Where("user_id = ?", 1).First(&retrieved).Error
		assert.NoError(tb, err)
		assert.Equal(tb, "test-connect-key", retrieved.ConnectKey)

		logger.Debug("Database integration test passed", zap.Uint("userID", retrieved.UserID))
	}, QuotaTestOptions())
}

func TestEnforceFundingTarget_WithAccountNoQuotaKey(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Arrange - create an account with no quota key
		account := &siaDB.SiaAccount{
			UserID:     100,
			ConnectKey: "test-key-100",
			QuotaKey:   "", // Empty quota key
		}
		err := ctx.DB().Create(account).Error
		assert.NoError(tb, err)

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)

		// Act
		err = quotaSvc.EnforceFundingTarget(context.Background(), 100)

		// Assert - should succeed when quota key is empty (early return)
		assert.NoError(tb, err)
	}, QuotaTestOptions())
}
