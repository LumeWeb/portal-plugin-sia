package quota

import (
	"context"
	"errors"
	"testing"
	"time"

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
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.sia.tech/core/types"
	"go.sia.tech/indexd/accounts"
	"go.sia.tech/indexd/hosts"
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

func (m *mockConfigManager) GetUserQuotaConfig(ctx context.Context, userID uint) (*quotaCore.UserQuotaConfig, error) {
	return nil, nil
}

func (m *mockConfigManager) GetPolicyEnforcer(ctx context.Context, userID uint) (quotaCore.PolicyEnforcer, error) {
	return nil, nil
}

func (m *mockConfigManager) GetUserAllowanceGrants(ctx context.Context, userID uint) ([]*quotaCore.AllowanceGrant, error) {
	return nil, nil
}

func (m *mockConfigManager) GetUserAllowanceGrantsByType(ctx context.Context, userID uint, grantType quotaCore.GrantType) ([]*quotaCore.AllowanceGrant, error) {
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

func quotaTestOptionsWithMockAdminAndQuotaCore(t *testing.T) (coreTesting.TestContextBuilderOption, *mocks.MockAdminClient) {
	mockAdmin := mocks.NewMockAdminClient(t)
	return coreTesting.CombineOptions(
		baseQuotaTestOptions(),
		coreTesting.WithServiceFactory(pluginCore.SIA_SERVICE, func() (core.Service, []core.ContextBuilderOption, error) {
			return sia.NewSiaServiceWithAdminClient(mockAdmin)
		}),
		coreTesting.WithServiceFactory(pluginCore.QUOTA_SERVICE, NewQuotaService),
		coreTesting.WithMockServiceFactory(quotaCore.QUOTA_SERVICE, quotaCore.NewMockQuotaService, quotaCore.QuotaConfig{}),
	), mockAdmin
}

func quotaTestOptionsWithMockAdminAndQuotaCoreHandle(t *testing.T) (coreTesting.TestContextBuilderOption, *mocks.MockAdminClient, *quotaCore.MockQuotaService) {
	mockAdmin := mocks.NewMockAdminClient(t)
	mockQuotaSvc := quotaCore.NewMockQuotaService(t)
	return coreTesting.CombineOptions(
		baseQuotaTestOptions(),
		coreTesting.WithServiceFactory(pluginCore.SIA_SERVICE, func() (core.Service, []core.ContextBuilderOption, error) {
			return sia.NewSiaServiceWithAdminClient(mockAdmin)
		}),
		coreTesting.WithServiceFactory(pluginCore.QUOTA_SERVICE, NewQuotaService),
		coreTesting.WithServiceFactory(quotaCore.QUOTA_SERVICE, func() (core.Service, []core.ContextBuilderOption, error) {
			return mockQuotaSvc, nil, nil
		}),
	), mockAdmin, mockQuotaSvc
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
		assert.ErrorIs(tb, err, ErrAccountNotVerified)
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

func TestSyncFunding_PoolEventAttributed(t *testing.T) {
	opts, mockAdmin := quotaTestOptionsWithMockAdmin(t)

	quotaKey := "user-42"
	poolID := 42
	poolEvent := accounts.FundingEvent{
		ID:                     100,
		HostKey:                types.PublicKey{0x01},
		AmountSC:               types.NewCurrency64(1000),
		EstimatedUploadBytes:   5000,
		EstimatedDownloadBytes: 3000,
		FundType:               accounts.FundingTypePool,
		PoolID:                 &poolID,
		QuotaName:              &quotaKey,
		CreatedAt:              time.Now(),
	}

	mockAdmin.EXPECT().FundingEvents(mock.Anything, mock.Anything, mock.Anything).Return([]accounts.FundingEvent{poolEvent}, nil).Once()
	mockAdmin.EXPECT().FundingEvents(mock.Anything, mock.Anything, mock.Anything).Return([]accounts.FundingEvent{}, nil).Maybe()

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Ensure a funding cursor row exists
		cursor := &siaDB.FundingCursor{
			ID:                 1,
			LastFundingEventID: 0,
			LastFundingEventAt: time.Time{},
		}
		ctx.DB().Where("id = ?", 1).FirstOrCreate(cursor)

		// Create a SiaAccount with matching QuotaKey
		account := &siaDB.SiaAccount{
			UserID:   42,
			QuotaKey: quotaKey,
		}
		ctx.DB().Create(account)

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		err := quotaSvc.SyncFunding(context.Background())

		assert.NoError(tb, err)

		// Verify the global cursor advanced past the pool event
		var updatedCursor siaDB.FundingCursor
		err = ctx.DB().Where("id = ?", 1).First(&updatedCursor).Error
		require.NoError(tb, err)
		assert.Equal(tb, int64(100), updatedCursor.LastFundingEventID, "cursor should advance past pool event")

		// Verify the SiaAccount was updated with the event tracking
		var updatedAccount siaDB.SiaAccount
		err = ctx.DB().Where("quota_key = ?", quotaKey).First(&updatedAccount).Error
		require.NoError(tb, err)
		assert.Equal(tb, int64(100), updatedAccount.LastFundingEventID, "account should track the pool event ID")
		require.NotNil(tb, updatedAccount.LastFundingEventAt)
	}, opts)
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

func TestEnforceFundingTarget_AppliesCalculateFundTargetBytes(t *testing.T) {
	opts, mockAdmin, mockQuotaSvc := quotaTestOptionsWithMockAdminAndQuotaCoreHandle(t)

	userID := uint(200)
	quotaKey := "user-200"
	storageLimitBytes := uint64(120 * 1 << 30) // 120 GiB
	expectedFundTargetBytes := internal.CalculateFundTargetBytes(storageLimitBytes)

	// mock config manager returns limits with a known storage config
	mockCM := &mockConfigManager{}
	mockCM.On("ResolveEffectiveLimits", mock.Anything, userID).Return(&quotaCore.EffectiveLimits{
		HasStorageLimitConfig: true,
		StorageLimitConfig: &quotaCore.Limit{
			Bytes: storageLimitBytes,
		},
	}, nil)

	mockQuotaSvc.EXPECT().GetConfigManager().Return(mockCM).Maybe()
	mockQuotaSvc.EXPECT().CheckStorageQuota(mock.Anything, mock.Anything, mock.Anything).Return(quotaCore.QuotaCheckResult{Allowed: true}, nil).Maybe()
	mockQuotaSvc.EXPECT().CheckUploadQuota(mock.Anything, mock.Anything, mock.Anything).Return(quotaCore.QuotaCheckResult{Allowed: true}, nil).Maybe()
	mockQuotaSvc.EXPECT().CheckDownloadQuota(mock.Anything, mock.Anything, mock.Anything).Return(quotaCore.QuotaCheckResult{Allowed: true}, nil).Maybe()

	// UpdateFundTargetBytes reads existing quota first (PutQuota is full replace)
	mockAdmin.EXPECT().Quota(mock.Anything, quotaKey).Return(accounts.Quota{
		Key:         quotaKey,
		Description: "test quota",
	}, nil).Maybe()

	// capture the PutQuota request to verify FundTargetBytes
	mockAdmin.EXPECT().PutQuota(mock.Anything, quotaKey, mock.MatchedBy(func(req accounts.PutQuotaRequest) bool {
		return req.FundTargetBytes != nil && *req.FundTargetBytes == expectedFundTargetBytes
	})).Return(nil).Once()

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		account := &siaDB.SiaAccount{
			UserID:   userID,
			QuotaKey: quotaKey,
		}
		err := ctx.DB().Create(account).Error
		assert.NoError(tb, err)

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		err = quotaSvc.EnforceFundingTarget(context.Background(), userID)
		assert.NoError(tb, err)
	}, opts)
}

func TestConnectQuotaCheck_NoAccount(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)

		result, err := quotaSvc.ConnectQuotaCheck(context.Background(), 999)

		assert.Error(tb, err)
		assert.Contains(tb, err.Error(), "failed to get sia account")
		assert.Nil(tb, result)
	}, QuotaTestOptions())
}

func TestConnectQuotaCheck_FundTargetBytesZero(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		account := &siaDB.SiaAccount{
			UserID:          1,
			ConnectKey:      "test-key-zero",
			FundTargetBytes: 0,
		}
		err := ctx.DB().Create(account).Error
		require.NoError(tb, err)

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		result, err := quotaSvc.ConnectQuotaCheck(context.Background(), 1)

		require.NoError(tb, err)
		assert.False(tb, result.HasQuota)
		assert.True(tb, result.HasUsableHosts) // quota error takes priority over unchecked hosts
	}, QuotaTestOptions())
}

func TestConnectQuotaCheck_AdminClientNil(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		account := &siaDB.SiaAccount{
			UserID:          1,
			ConnectKey:      "test-key-adminnil",
			FundTargetBytes: 1 << 30, // 1 GiB
		}
		err := ctx.DB().Create(account).Error
		require.NoError(tb, err)

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		result, err := quotaSvc.ConnectQuotaCheck(context.Background(), 1)

		require.NoError(tb, err)
		assert.True(tb, result.HasQuota)
		assert.True(tb, result.HasUsableHosts)
	}, QuotaTestOptions())
}

func TestConnectQuotaCheck_HostsError(t *testing.T) {
	opts, mockAdmin := quotaTestOptionsWithMockAdmin(t)
	mockAdmin.EXPECT().Hosts(mock.Anything, mock.Anything).Return(nil, errors.New("host service unavailable")).Maybe()

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		account := &siaDB.SiaAccount{
			UserID:          1,
			ConnectKey:      "test-key-hostserr",
			FundTargetBytes: 1 << 30,
		}
		err := ctx.DB().Create(account).Error
		require.NoError(tb, err)

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		result, err := quotaSvc.ConnectQuotaCheck(context.Background(), 1)

		require.NoError(tb, err)
		assert.True(tb, result.HasQuota)
		assert.True(tb, result.HasUsableHosts)
	}, opts)
}

func TestConnectQuotaCheck_NoUsableHosts(t *testing.T) {
	opts, mockAdmin := quotaTestOptionsWithMockAdmin(t)
	mockAdmin.EXPECT().Hosts(mock.Anything, mock.Anything).Return([]hosts.Host{}, nil).Maybe()

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		account := &siaDB.SiaAccount{
			UserID:          1,
			ConnectKey:      "test-key-nohosts",
			FundTargetBytes: 1 << 30,
		}
		err := ctx.DB().Create(account).Error
		require.NoError(tb, err)

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		result, err := quotaSvc.ConnectQuotaCheck(context.Background(), 1)

		require.NoError(tb, err)
		assert.False(tb, result.HasQuota)
		assert.False(tb, result.HasUsableHosts)
	}, opts)
}

func TestConnectQuotaCheck_UploadQuotaExceeded(t *testing.T) {
	opts, mockAdmin := quotaTestOptionsWithMockAdminAndQuotaCore(t)
	mockAdmin.EXPECT().Hosts(mock.Anything, mock.Anything).Return([]hosts.Host{{}}, nil).Maybe()

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		account := &siaDB.SiaAccount{
			UserID:          1,
			ConnectKey:      "test-key-upload-exceeded",
			FundTargetBytes: 1 << 30,
		}
		err := ctx.DB().Create(account).Error
		require.NoError(tb, err)

		mockQS := core.GetService[*quotaCore.MockQuotaService](ctx, quotaCore.QUOTA_SERVICE)
		mockQS.EXPECT().CheckUploadQuota(mock.Anything, mock.AnythingOfType("uint"), mock.AnythingOfType("uint64")).Return(quotaCore.QuotaCheckResult{
			Allowed: false,
		}, nil).Maybe()
		mockQS.EXPECT().CheckDownloadQuota(mock.Anything, mock.AnythingOfType("uint"), mock.AnythingOfType("uint64")).Return(quotaCore.QuotaCheckResult{
			Allowed: true,
		}, nil).Maybe()

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		result, err := quotaSvc.ConnectQuotaCheck(context.Background(), 1)

		require.NoError(tb, err)
		assert.False(tb, result.HasQuota)
		assert.True(tb, result.HasUsableHosts)
	}, opts)
}

func TestConnectQuotaCheck_DownloadQuotaExceeded(t *testing.T) {
	opts, mockAdmin := quotaTestOptionsWithMockAdminAndQuotaCore(t)
	mockAdmin.EXPECT().Hosts(mock.Anything, mock.Anything).Return([]hosts.Host{{}}, nil).Maybe()

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		account := &siaDB.SiaAccount{
			UserID:          1,
			ConnectKey:      "test-key-download-exceeded",
			FundTargetBytes: 1 << 30,
		}
		err := ctx.DB().Create(account).Error
		require.NoError(tb, err)

		mockQS := core.GetService[*quotaCore.MockQuotaService](ctx, quotaCore.QUOTA_SERVICE)
		mockQS.EXPECT().CheckUploadQuota(mock.Anything, mock.AnythingOfType("uint"), mock.AnythingOfType("uint64")).Return(quotaCore.QuotaCheckResult{
			Allowed: true,
		}, nil).Maybe()
		mockQS.EXPECT().CheckDownloadQuota(mock.Anything, mock.AnythingOfType("uint"), mock.AnythingOfType("uint64")).Return(quotaCore.QuotaCheckResult{
			Allowed: false,
		}, nil).Maybe()

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		result, err := quotaSvc.ConnectQuotaCheck(context.Background(), 1)

		require.NoError(tb, err)
		assert.False(tb, result.HasQuota)
		assert.True(tb, result.HasUsableHosts)
	}, opts)
}

func TestConnectQuotaCheck_Success(t *testing.T) {
	opts, mockAdmin := quotaTestOptionsWithMockAdmin(t)
	mockAdmin.EXPECT().Hosts(mock.Anything, mock.Anything).Return([]hosts.Host{{}}, nil).Maybe()

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		account := &siaDB.SiaAccount{
			UserID:          1,
			ConnectKey:      "test-key-success",
			FundTargetBytes: 1 << 30,
		}
		err := ctx.DB().Create(account).Error
		require.NoError(tb, err)

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		result, err := quotaSvc.ConnectQuotaCheck(context.Background(), 1)

		require.NoError(tb, err)
		assert.True(tb, result.HasQuota)
		assert.True(tb, result.HasUsableHosts)
	}, opts)
}
