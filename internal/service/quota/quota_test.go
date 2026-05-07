package quota

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	quotaCore "go.lumeweb.com/portal-plugin-quota/core"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal-plugin-sia/internal"
	"go.lumeweb.com/portal-plugin-sia/internal/config"
	siaDB "go.lumeweb.com/portal-plugin-sia/internal/db"
	"go.lumeweb.com/portal-plugin-sia/internal/db/migrations"
	"go.lumeweb.com/portal-plugin-sia/internal/service/sia"
	"go.lumeweb.com/portal-plugin-sia/internal/testing/mocks"
	"go.lumeweb.com/portal-plugin-sia/internal/testing/util"
	"go.sia.tech/indexd/accounts"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

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

func baseQuotaTestOptions() coreTesting.TestContextBuilderOption {
	return coreTesting.CombineOptions(
		util.GetProtocolMock(),
		coreTesting.WithProtocolConfig(internal.ProtocolName, &config.ProtocolConfig{
			AppURL: "http://localhost:8081",
		}),
		coreTesting.WithSQLitePluginMigrations("sia", migrations.GetSQLite()),
	)
}

func QuotaTestOptions() coreTesting.TestContextBuilderOption {
	return coreTesting.CombineOptions(
		baseQuotaTestOptions(),
		coreTesting.WithServiceFactory(pluginCore.SIA_SERVICE, sia.NewSiaService),
		coreTesting.WithServiceFactory(pluginCore.QUOTA_SERVICE, NewQuotaService),
	)
}

func quotaTestOptionsWithMockAdmin(t *testing.T) (coreTesting.TestContextBuilderOption, *mocks.MockAdminClient) {
	mockAdmin := mocks.NewMockAdminClient(t)
	return coreTesting.CombineOptions(
		baseQuotaTestOptions(),
		coreTesting.WithServiceFactory(pluginCore.SIA_SERVICE, func() (core.Service, []core.ContextBuilderOption, error) {
			return sia.NewSiaServiceWithAdminClient(mockAdmin)
		}),
		coreTesting.WithServiceFactory(pluginCore.QUOTA_SERVICE, NewQuotaService),
	), mockAdmin
}

func TestProvisionAccount_Success(t *testing.T) {
	opts, mockAdmin := quotaTestOptionsWithMockAdmin(t)

	mockAdmin.EXPECT().PutQuota(mock.Anything, "user-1", mock.AnythingOfType("accounts.PutQuotaRequest")).Return(nil)
	mockAdmin.EXPECT().AddAppConnectKey(mock.Anything, mock.AnythingOfType("accounts.AppConnectKeyRequest")).Return(accounts.ConnectKey{
		Key: "test-connect-key",
	}, nil)

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		mockUserSvc := core.GetService[*coreTesting.MockUserService](ctx, core.USER_SERVICE)
		mockUserSvc.EXPECT().IsAccountVerified(mock.Anything, uint(1)).Return(true, nil).Maybe()

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		err := quotaSvc.ProvisionAccount(context.Background(), 1)

		require.NoError(tb, err)
	}, opts)
}

func TestProvisionAccount_UnverifiedUser(t *testing.T) {
	opts, mockAdmin := quotaTestOptionsWithMockAdmin(t)
	mockAdmin.EXPECT().PutQuota(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		mockUserSvc := core.GetService[*coreTesting.MockUserService](ctx, core.USER_SERVICE)
		mockUserSvc.EXPECT().IsAccountVerified(mock.Anything, uint(1)).Return(false, nil).Maybe()

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		err := quotaSvc.ProvisionAccount(context.Background(), 1)

		assert.Error(tb, err)
		assert.Contains(tb, err.Error(), "not verified")
	}, opts)
}

func TestEnforceFundingTarget_NoAccount(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)

		err := quotaSvc.EnforceFundingTarget(context.Background(), 999)

		assert.NoError(tb, err)
	}, QuotaTestOptions())
}

func TestSyncFunding_NoAdminClient(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)

		err := quotaSvc.SyncFunding(context.Background())

		assert.NoError(tb, err)
	}, QuotaTestOptions())
}

func TestProvisionAccount_DatabaseIntegration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		logger := ctx.Logger()

		account := &siaDB.SiaAccount{
			UserID:     1,
			ConnectKey: "test-connect-key",
		}
		err := ctx.DB().Create(account).Error
		assert.NoError(tb, err)

		var retrieved siaDB.SiaAccount
		err = ctx.DB().Where("user_id = ?", 1).First(&retrieved).Error
		assert.NoError(tb, err)
		assert.Equal(tb, "test-connect-key", retrieved.ConnectKey)

		logger.Debug("Database integration test passed", zap.Uint("userID", retrieved.UserID))
	}, QuotaTestOptions())
}

func TestEnforceFundingTarget_WithAccountNoQuotaKey(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		account := &siaDB.SiaAccount{
			UserID:     100,
			ConnectKey: "test-key-100",
			QuotaKey:   "",
		}
		err := ctx.DB().Create(account).Error
		assert.NoError(tb, err)

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		err = quotaSvc.EnforceFundingTarget(context.Background(), 100)

		assert.NoError(tb, err)
	}, QuotaTestOptions())
}
