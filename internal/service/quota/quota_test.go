package quota

import (
	"context"
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
	"go.sia.tech/indexd/contracts"
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
	mockAdmin.EXPECT().Contracts(mock.Anything, mock.Anything).Return([]contracts.Contract{{}}, nil).Maybe()

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
	expectedFundTargetBytes := internal.CalculateFundTargetBytes(1, 1)

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
		// Simulate SyncFunding having cached 1 active contract
		quotaSvc.(*QuotaService).activeContractCount.Store(1)
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
	}, QuotaTestOptions())
}

func TestConnectQuotaCheck_UploadQuotaExceeded(t *testing.T) {
	opts, _ := quotaTestOptionsWithMockAdminAndQuotaCore(t)

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		account := &siaDB.SiaAccount{
			UserID:          1,
			ConnectKey:      "test-key-upload-exceeded",
			FundTargetBytes: 1 << 30,
		}
		err := ctx.DB().Create(account).Error
		require.NoError(tb, err)

		mockQS := core.GetService[*quotaCore.MockQuotaService](ctx, quotaCore.QUOTA_SERVICE)
		mockQS.EXPECT().CheckUploadQuota(mock.Anything, mock.AnythingOfType("uint"), mock.AnythingOfType("uint64"), mock.Anything).Return(quotaCore.QuotaCheckResult{
			Allowed: false,
		}, nil).Maybe()
		mockQS.EXPECT().CheckDownloadQuota(mock.Anything, mock.AnythingOfType("uint"), mock.AnythingOfType("uint64"), mock.Anything).Return(quotaCore.QuotaCheckResult{
			Allowed: true,
		}, nil).Maybe()

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		result, err := quotaSvc.ConnectQuotaCheck(context.Background(), 1)

		require.NoError(tb, err)
		assert.False(tb, result.HasQuota)
	}, opts)
}

func TestConnectQuotaCheck_DownloadQuotaExceeded(t *testing.T) {
	opts, _ := quotaTestOptionsWithMockAdminAndQuotaCore(t)

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		account := &siaDB.SiaAccount{
			UserID:          1,
			ConnectKey:      "test-key-download-exceeded",
			FundTargetBytes: 1 << 30,
		}
		err := ctx.DB().Create(account).Error
		require.NoError(tb, err)

		mockQS := core.GetService[*quotaCore.MockQuotaService](ctx, quotaCore.QUOTA_SERVICE)
		mockQS.EXPECT().CheckUploadQuota(mock.Anything, mock.AnythingOfType("uint"), mock.AnythingOfType("uint64"), mock.Anything).Return(quotaCore.QuotaCheckResult{
			Allowed: true,
		}, nil).Maybe()
		mockQS.EXPECT().CheckDownloadQuota(mock.Anything, mock.AnythingOfType("uint"), mock.AnythingOfType("uint64"), mock.Anything).Return(quotaCore.QuotaCheckResult{
			Allowed: false,
		}, nil).Maybe()

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		result, err := quotaSvc.ConnectQuotaCheck(context.Background(), 1)

		require.NoError(tb, err)
		assert.False(tb, result.HasQuota)
	}, opts)
}

func TestConnectQuotaCheck_Success(t *testing.T) {
	opts, _ := quotaTestOptionsWithMockAdmin(t)

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
	}, opts)
}

func TestEnforceFundingTarget_WithAppCount_Multiplier(t *testing.T) {
	opts, mockAdmin, mockQuotaSvc := quotaTestOptionsWithMockAdminAndQuotaCoreHandle(t)

	userID := uint(300)
	quotaKey := "user-300"
	storageLimitBytes := uint64(120 * 1 << 30) // 120 GiB

	// With 3 apps, FundTargetBytes should be 3x the single-app value.
	numApps := 3
	expectedFundTargetBytes := internal.CalculateFundTargetBytes(1, uint64(numApps))

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

	mockAdmin.EXPECT().Quota(mock.Anything, quotaKey).Return(accounts.Quota{
		Key:         quotaKey,
		Description: "test quota",
	}, nil).Maybe()

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
		quotaSvc.(*QuotaService).activeContractCount.Store(1)
		err = quotaSvc.EnforceFundingTargetWithAppCount(context.Background(), userID, numApps)
		assert.NoError(tb, err)
	}, opts)
}

func TestCountAppAccountsBySiaAccount(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Create two sia accounts
		account1 := &siaDB.SiaAccount{UserID: 1, QuotaKey: "key-1"}
		account2 := &siaDB.SiaAccount{UserID: 2, QuotaKey: "key-2"}
		require.NoError(tb, ctx.DB().Create(account1).Error)
		require.NoError(tb, ctx.DB().Create(account2).Error)

		// Create app accounts: 2 for account1, 1 for account2
		key1 := siaDB.DBAccountKeyFromPublicKey(types.GeneratePrivateKey().PublicKey())
		key2 := siaDB.DBAccountKeyFromPublicKey(types.GeneratePrivateKey().PublicKey())
		key3 := siaDB.DBAccountKeyFromPublicKey(types.GeneratePrivateKey().PublicKey())

		require.NoError(tb, ctx.DB().Create(&siaDB.SiaAppAccount{
			SiaAccountID: account1.ID,
			AccountKey:   key1,
		}).Error)
		require.NoError(tb, ctx.DB().Create(&siaDB.SiaAppAccount{
			SiaAccountID: account1.ID,
			AccountKey:   key2,
		}).Error)
		require.NoError(tb, ctx.DB().Create(&siaDB.SiaAppAccount{
			SiaAccountID: account2.ID,
			AccountKey:   key3,
		}).Error)

		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)
		counts, err := siaSvc.CountAppAccountsBySiaAccount(context.Background())
		require.NoError(tb, err)

		assert.Equal(tb, 2, counts[account1.ID])
		assert.Equal(tb, 1, counts[account2.ID])
	}, QuotaTestOptions())
}

func TestConnectQuotaCheck_PredictionScalesWithAppCount(t *testing.T) {
	opts, _ := quotaTestOptionsWithMockAdminAndQuotaCore(t)

	userID := uint(1)
	connectKey := "test-key-multi-app"

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		account := &siaDB.SiaAccount{
			UserID:          userID,
			ConnectKey:      connectKey,
			FundTargetBytes: 1 << 30,
		}
		err := ctx.DB().Create(account).Error
		require.NoError(tb, err)

		// Register 2 existing app accounts
		key1 := siaDB.DBAccountKeyFromPublicKey(types.GeneratePrivateKey().PublicKey())
		key2 := siaDB.DBAccountKeyFromPublicKey(types.GeneratePrivateKey().PublicKey())
		require.NoError(tb, ctx.DB().Create(&siaDB.SiaAppAccount{
			SiaAccountID: account.ID,
			AccountKey:   key1,
		}).Error)
		require.NoError(tb, ctx.DB().Create(&siaDB.SiaAppAccount{
			SiaAccountID: account.ID,
			AccountKey:   key2,
		}).Error)

		// With 2 existing apps + 1 being connected = 3 apps.
		// Upload prediction should be 3x single-app bandwidth.
		expectedUploadBytes, _ := internal.PredictedBandwidthBytes(3)

		mockQS := core.GetService[*quotaCore.MockQuotaService](ctx, quotaCore.QUOTA_SERVICE)
		mockQS.EXPECT().CheckUploadQuota(mock.Anything, userID, expectedUploadBytes, mock.Anything).Return(quotaCore.QuotaCheckResult{
			Allowed: true,
		}, nil).Maybe()
		mockQS.EXPECT().CheckDownloadQuota(mock.Anything, userID, mock.AnythingOfType("uint64"), mock.Anything).Return(quotaCore.QuotaCheckResult{
			Allowed: true,
		}, nil).Maybe()

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		result, err := quotaSvc.ConnectQuotaCheck(context.Background(), userID)

		require.NoError(tb, err)
		assert.True(tb, result.HasQuota)
	}, opts)
}

// Regression: EnforceFundingTargetWithAppCount(0) should default to numApps=1
// WITHOUT calling ListAppAccounts (batch succeeded, 0 apps found).
func TestEnforceFundingTarget_AppCountZero_DefaultsTo1(t *testing.T) {
	opts, mockAdmin, mockQuotaSvc := quotaTestOptionsWithMockAdminAndQuotaCoreHandle(t)

	userID := uint(400)
	quotaKey := "user-400"
	storageLimitBytes := uint64(120 * 1 << 30)

	// With 0 apps → defaults to 1 → same as single-app calculation.
	expectedFundTargetBytes := internal.CalculateFundTargetBytes(1, 1)

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

	mockAdmin.EXPECT().Quota(mock.Anything, quotaKey).Return(accounts.Quota{
		Key:         quotaKey,
		Description: "test quota",
	}, nil).Maybe()

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
		quotaSvc.(*QuotaService).activeContractCount.Store(1)

		// appCount=0 means batch found 0 apps. Should default to 1 internally
		// without calling ListAppAccounts.
		err = quotaSvc.EnforceFundingTargetWithAppCount(context.Background(), userID, 0)
		assert.NoError(tb, err)
	}, opts)
}

// Regression: EnforceFundingTarget (one-off caller) passes -1 to trigger
// per-account ListAppAccounts fallback. With 2 existing apps, FundTargetBytes
// should reflect numApps=2.
func TestEnforceFundingTarget_OneOffCall_FetchesAppCountViaDB(t *testing.T) {
	opts, mockAdmin, mockQuotaSvc := quotaTestOptionsWithMockAdminAndQuotaCoreHandle(t)

	userID := uint(500)
	quotaKey := "user-500"
	storageLimitBytes := uint64(120 * 1 << 30)

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

	mockAdmin.EXPECT().Quota(mock.Anything, quotaKey).Return(accounts.Quota{
		Key:         quotaKey,
		Description: "test quota",
	}, nil).Maybe()

	// 2 app accounts → expect FundTargetBytes for numApps=2
	expectedFundTargetBytes := internal.CalculateFundTargetBytes(1, 2)

	mockAdmin.EXPECT().PutQuota(mock.Anything, quotaKey, mock.MatchedBy(func(req accounts.PutQuotaRequest) bool {
		return req.FundTargetBytes != nil && *req.FundTargetBytes == expectedFundTargetBytes
	})).Return(nil).Once()

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		account := &siaDB.SiaAccount{
			UserID:   userID,
			QuotaKey: quotaKey,
		}
		err := ctx.DB().Create(account).Error
		require.NoError(tb, err)

		// Create 2 app accounts for this sia account
		key1 := siaDB.DBAccountKeyFromPublicKey(types.GeneratePrivateKey().PublicKey())
		key2 := siaDB.DBAccountKeyFromPublicKey(types.GeneratePrivateKey().PublicKey())
		require.NoError(tb, ctx.DB().Create(&siaDB.SiaAppAccount{
			SiaAccountID: account.ID,
			AccountKey:   key1,
		}).Error)
		require.NoError(tb, ctx.DB().Create(&siaDB.SiaAppAccount{
			SiaAccountID: account.ID,
			AccountKey:   key2,
		}).Error)

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		quotaSvc.(*QuotaService).activeContractCount.Store(1)

		// EnforceFundingTarget (not WithAppCount) should pass -1 internally
		// and trigger the per-account ListAppAccounts DB lookup.
		err = quotaSvc.EnforceFundingTarget(context.Background(), userID)
		assert.NoError(tb, err)
	}, opts)
}

// Regression: ConnectQuotaCheck with 0 existing apps should count numApps=1
// (just the connecting app).
func TestConnectQuotaCheck_ZeroApps_CountsConnectingApp(t *testing.T) {
	opts, _ := quotaTestOptionsWithMockAdminAndQuotaCore(t)

	userID := uint(1)
	connectKey := "test-key-zero-apps"

	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		account := &siaDB.SiaAccount{
			UserID:          userID,
			ConnectKey:      connectKey,
			FundTargetBytes: 1 << 30,
		}
		err := ctx.DB().Create(account).Error
		require.NoError(tb, err)

		// No app accounts registered — just the connecting app.
		// numApps should be 1, upload prediction = single-app bandwidth.
		expectedUploadBytes, _ := internal.PredictedBandwidthBytes(1)

		mockQS := core.GetService[*quotaCore.MockQuotaService](ctx, quotaCore.QUOTA_SERVICE)
		mockQS.EXPECT().CheckUploadQuota(mock.Anything, userID, expectedUploadBytes, mock.Anything).Return(quotaCore.QuotaCheckResult{
			Allowed: true,
		}, nil).Maybe()
		mockQS.EXPECT().CheckDownloadQuota(mock.Anything, userID, mock.AnythingOfType("uint64"), mock.Anything).Return(quotaCore.QuotaCheckResult{
			Allowed: true,
		}, nil).Maybe()

		quotaSvc := core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)
		result, err := quotaSvc.ConnectQuotaCheck(context.Background(), userID)

		require.NoError(tb, err)
		assert.True(tb, result.HasQuota)
	}, opts)
}

// Regression: CalculateFundTargetBytes edge cases.
func TestCalculateFundTargetBytes_EdgeCases(t *testing.T) {
	// Zero contracts → 0
	assert.Equal(t, uint64(0), internal.CalculateFundTargetBytes(0, 5))
	// Zero apps → 0
	assert.Equal(t, uint64(0), internal.CalculateFundTargetBytes(10, 0))
	// Both zero → 0
	assert.Equal(t, uint64(0), internal.CalculateFundTargetBytes(0, 0))
	// Normal: 1 contract, 1 app → full bandwidth budget
	expected := internal.BandwidthBytesPerInterval()
	assert.Equal(t, expected, internal.CalculateFundTargetBytes(1, 1))
	// 2 apps, 1 contract → 2x
	assert.Equal(t, expected*2, internal.CalculateFundTargetBytes(1, 2))
	// 2 apps, 2 contracts → same as 1 app, 1 contract
	assert.Equal(t, expected, internal.CalculateFundTargetBytes(2, 2))
}

// Regression: PredictedBandwidthBytes edge cases.
func TestPredictedBandwidthBytes_EdgeCases(t *testing.T) {
	// Zero apps → (0, 0)
	upload, download := internal.PredictedBandwidthBytes(0)
	assert.Equal(t, uint64(0), upload)
	assert.Equal(t, uint64(0), download)
	// 1 app → full bandwidth per direction
	expected := internal.BandwidthBytesPerInterval()
	upload, download = internal.PredictedBandwidthBytes(1)
	assert.Equal(t, expected, upload)
	assert.Equal(t, expected, download)
	// 3 apps → 3x
	upload, download = internal.PredictedBandwidthBytes(3)
	assert.Equal(t, expected*3, upload)
	assert.Equal(t, expected*3, download)
}

// Regression: CountAppAccountsBySiaAccount with no app accounts returns empty map.
func TestCountAppAccountsBySiaAccount_NoAppAccounts(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		account := &siaDB.SiaAccount{UserID: 1, QuotaKey: "key-1"}
		require.NoError(tb, ctx.DB().Create(account).Error)

		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)
		counts, err := siaSvc.CountAppAccountsBySiaAccount(context.Background())
		require.NoError(tb, err)
		assert.NotNil(tb, counts)
		_, exists := counts[account.ID]
		assert.False(tb, exists, "account with no apps should not be in map")
	}, QuotaTestOptions())
}
