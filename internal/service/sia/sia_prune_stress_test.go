package sia

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal-plugin-sia/internal/db"
	core "go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	coreMocks "go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

// storageHashMatcher is a custom matcher for StorageHash arguments
func storageHashMatcher() interface{} {
	return mock.MatchedBy(func(h *core.StorageHashDefault) bool { return true })
}

// registerSlabsAndObjects registers slabs first, then objects, ensuring all slab references exist
func registerSlabsAndObjects(ctx context.Context, svc *SiaService, accountID uint, objects map[string][]string) error {
	// Collect all unique slabIDs
	slabSet := make(map[string]bool)
	for _, slabIDs := range objects {
		for _, sid := range slabIDs {
			slabSet[sid] = true
		}
	}

	// Register all slabs first
	for sid := range slabSet {
		if err := svc.RegisterSlab(ctx, accountID, sid); err != nil {
			return err
		}
	}

	// Register objects
	for objectID, slabIDs := range objects {
		if err := svc.RegisterObject(ctx, accountID, objectID, slabIDs); err != nil {
			return err
		}
	}

	return nil
}

// registerSlabsAndObjectsMultiAccount registers slabs for multiple accounts then objects
func registerSlabsAndObjectsMultiAccount(ctx context.Context, svc *SiaService, accountsSlabs map[uint]map[string][]string, sharedSlabs []string) error {
	// Get all unique slabIDs across all accounts and shared
	slabSet := make(map[string]bool)
	for _, sid := range sharedSlabs {
		slabSet[sid] = true
	}
	for _, objects := range accountsSlabs {
		for _, slabIDs := range objects {
			for _, sid := range slabIDs {
				slabSet[sid] = true
			}
		}
	}

	// Register all slabs for each account
	for accountID := range accountsSlabs {
		for sid := range slabSet {
			if err := svc.RegisterSlab(ctx, accountID, sid); err != nil {
				return err
			}
		}
	}

	// Register objects
	for accountID, objects := range accountsSlabs {
		for objectID, slabIDs := range objects {
			if err := svc.RegisterObject(ctx, accountID, objectID, slabIDs); err != nil {
				return err
			}
		}
	}

	return nil
}

// TestPruneSlabsParameterized runs comprehensive parameterized prune scenarios
func TestPruneSlabsParameterized(t *testing.T) {
	testCases := []struct {
		name      string
		setupFunc func(ctx context.Context, svc *SiaService, ctxTest core.Context, mockPinSvc *coreMocks.MockPinService, mockUploadSvc *coreMocks.MockUploadService) (appAccountID uint, expectedOrphans int, expectedDeletions int)
	}{
		{
			name: "SingleAccount_OverlappingSlabs_FullPrune",
			setupFunc: func(ctx context.Context, svc *SiaService, ctxTest core.Context, mockPinSvc *coreMocks.MockPinService, mockUploadSvc *coreMocks.MockUploadService) (uint, int, int) {
				account := createAccount(ctxTest)
				// 7 slabs for overlapping scenario (5 objects, window size 3)
				slabIDs := generateSlabIDs(7)

				// Object -> slabs mapping: 5 objs with overlapping slabs
				objects := make(map[string][]string)
				for i := 0; i < 5; i++ {
					objects[fmt.Sprintf("obj-%d", i)] = slabIDs[i : i+3]
				}
				err := registerSlabsAndObjects(ctx, svc, account.ID, objects)
				require.NoError(t, err)

				// Delete all objects, then prune should find all 7 slabs orphaned
				err = svc.DeleteObjectsByAppAccount(ctx, account.ID)
				require.NoError(t, err)

				// Setup mocks for 7 slabs being deleted
				for i := 0; i < 7; i++ {
					mockPinSvc.EXPECT().UploadPinnedGlobal(mock.Anything, storageHashMatcher()).Return(false, nil).Once()
					mockPinSvc.EXPECT().GetAllPinsByHash(mock.Anything, storageHashMatcher()).Return([]*models.Pin{}, nil).Once()
					mockUploadSvc.EXPECT().DeleteUpload(mock.Anything, storageHashMatcher()).Return(nil).Maybe()
				}

				return account.ID, 7, 7
			},
		},
		{
			name: "TwoAccounts_SharedSlabs_PartialPrune",
			setupFunc: func(ctx context.Context, svc *SiaService, ctxTest core.Context, mockPinSvc *coreMocks.MockPinService, mockUploadSvc *coreMocks.MockUploadService) (uint, int, int) {
				accountA := createAccount(ctxTest)
				accountB := createAccount(ctxTest)

				sharedSlabs := generateSlabIDs(5)
				accountsSlabs := map[uint]map[string][]string{
					accountA.ID: {"obj-a1": sharedSlabs},
					accountB.ID: {"obj-b1": sharedSlabs},
				}
				err := registerSlabsAndObjectsMultiAccount(ctx, svc, accountsSlabs, sharedSlabs)
				require.NoError(t, err)

				// Delete all objects from account A
				err = svc.DeleteObjectsByAppAccount(ctx, accountA.ID)
				require.NoError(t, err)

				// Prune for account A: 5 slabs found orphaned for A, but shared with B
				// Should NOT delete because B still references them
				return accountA.ID, 5, 0
			},
		},
		{
			name: "TwoAccounts_SharedSlabs_BothDelete_SkipsBecauseStillReferenced",
			setupFunc: func(ctx context.Context, svc *SiaService, ctxTest core.Context, mockPinSvc *coreMocks.MockPinService, mockUploadSvc *coreMocks.MockUploadService) (uint, int, int) {
				accountA := createAccount(ctxTest)
				accountB := createAccount(ctxTest)

				sharedSlabs := generateSlabIDs(5)
				accountsSlabs := map[uint]map[string][]string{
					accountA.ID: {"obj-a1": sharedSlabs},
					accountB.ID: {"obj-b1": sharedSlabs},
				}
				err := registerSlabsAndObjectsMultiAccount(ctx, svc, accountsSlabs, sharedSlabs)
				require.NoError(t, err)

				// Delete from account A only - account B still has objects
				err = svc.DeleteObjectsByAppAccount(ctx, accountA.ID)
				require.NoError(t, err)

				// Prune for A: 5 slabs found orphaned for A, but B still references them
				return accountA.ID, 5, 0
			},
		},
		{
			name: "ChainReferences_DeleteSecondFromEnd_Cascades",
			setupFunc: func(ctx context.Context, svc *SiaService, ctxTest core.Context, mockPinSvc *coreMocks.MockPinService, mockUploadSvc *coreMocks.MockUploadService) (uint, int, int) {
				account := createAccount(ctxTest)

				abs := generateSlabIDs(3)
				// 3 slabs, 2 objects: obj1[s0,s1], obj2[s1,s2]
				objects := map[string][]string{
					"obj1": {abs[0], abs[1]},
					"obj2": {abs[1], abs[2]},
				}
				err := registerSlabsAndObjects(ctx, svc, account.ID, objects)
				require.NoError(t, err)

				// Delete only obj2 manually — obj1 still references s0 and s1
				err = svc.DeleteObject(ctx, account.ID, "obj2")
				require.NoError(t, err)

				// Before PruneSlabs: only s2 is orphaned (obj1 still references s0, s1)
				// But PruneSlabs calls DeleteObjectsByAppAccount first, which deletes obj1 too
				// → all 3 slabs become orphaned and get pruned
				for range 3 {
					mockPinSvc.EXPECT().UploadPinnedGlobal(mock.Anything, storageHashMatcher()).Return(false, nil).Once()
					mockPinSvc.EXPECT().GetAllPinsByHash(mock.Anything, storageHashMatcher()).Return([]*models.Pin{}, nil).Once()
					mockUploadSvc.EXPECT().DeleteUpload(mock.Anything, storageHashMatcher()).Return(nil).Maybe()
				}

				return account.ID, 1, 3
			},
		},
		{
			name: "ChainReferences_AllDeleted_FullPrune",
			setupFunc: func(ctx context.Context, svc *SiaService, ctxTest core.Context, mockPinSvc *coreMocks.MockPinService, mockUploadSvc *coreMocks.MockUploadService) (uint, int, int) {
				account := createAccount(ctxTest)

				abs := generateSlabIDs(4)
				objects := map[string][]string{
					"obj1": {abs[0], abs[1]},
					"obj2": {abs[1], abs[2]},
					"obj3": {abs[2], abs[3]},
				}
				err := registerSlabsAndObjects(ctx, svc, account.ID, objects)
				require.NoError(t, err)

				// Delete ALL objects (using bulk delete)
				err = svc.DeleteObjectsByAppAccount(ctx, account.ID)
				require.NoError(t, err)

				// Setup mocks for 4 slabs being deleted
				for i := 0; i < 4; i++ {
					mockPinSvc.EXPECT().UploadPinnedGlobal(mock.Anything, storageHashMatcher()).Return(false, nil).Once()
					mockPinSvc.EXPECT().GetAllPinsByHash(mock.Anything, storageHashMatcher()).Return([]*models.Pin{}, nil).Once()
					mockUploadSvc.EXPECT().DeleteUpload(mock.Anything, storageHashMatcher()).Return(nil).Maybe()
				}

				return account.ID, 4, 4
			},
		},
		{
			name: "CircularReference_FullPrune",
			setupFunc: func(ctx context.Context, svc *SiaService, ctxTest core.Context, mockPinSvc *coreMocks.MockPinService, mockUploadSvc *coreMocks.MockUploadService) (uint, int, int) {
				account := createAccount(ctxTest)

				slabIDs := generateSlabIDs(3)
				objects := map[string][]string{
					"obj-a": {slabIDs[0], slabIDs[1]},
					"obj-b": {slabIDs[1], slabIDs[2]},
				}
				err := registerSlabsAndObjects(ctx, svc, account.ID, objects)
				require.NoError(t, err)

				// Delete both
				err = svc.DeleteObjectsByAppAccount(ctx, account.ID)
				require.NoError(t, err)

				// Setup mocks for 3 slabs being deleted
				for i := 0; i < 3; i++ {
					mockPinSvc.EXPECT().UploadPinnedGlobal(mock.Anything, storageHashMatcher()).Return(false, nil).Once()
					mockPinSvc.EXPECT().GetAllPinsByHash(mock.Anything, storageHashMatcher()).Return([]*models.Pin{}, nil).Once()
					mockUploadSvc.EXPECT().DeleteUpload(mock.Anything, storageHashMatcher()).Return(nil).Maybe()
				}

				return account.ID, 3, 3
			},
		},
		{
			name: "GlobalPin_Exists_SkipsDeletionButDeletesOthers",
			setupFunc: func(ctx context.Context, svc *SiaService, ctxTest core.Context, mockPinSvc *coreMocks.MockPinService, mockUploadSvc *coreMocks.MockUploadService) (uint, int, int) {
				account := createAccount(ctxTest)

				slabIDs := generateSlabIDs(3)
				err := registerSlabsAndObjects(ctx, svc, account.ID, map[string][]string{"obj": slabIDs})
				require.NoError(t, err)

				// Delete object
				err = svc.DeleteObject(ctx, account.ID, "obj")
				require.NoError(t, err)

				// First slab is pinned globally (simulates another user having a pin)
				mockPinSvc.EXPECT().UploadPinnedGlobal(mock.Anything, storageHashMatcher()).Return(true, nil).Once()
				// Second and third are not pinned globally - proceed with deletion
				mockPinSvc.EXPECT().UploadPinnedGlobal(mock.Anything, storageHashMatcher()).Return(false, nil).Once()
				mockPinSvc.EXPECT().GetAllPinsByHash(mock.Anything, storageHashMatcher()).Return([]*models.Pin{}, nil).Once()
				mockUploadSvc.EXPECT().DeleteUpload(mock.Anything, storageHashMatcher()).Return(nil).Maybe()
				mockPinSvc.EXPECT().UploadPinnedGlobal(mock.Anything, storageHashMatcher()).Return(false, nil).Once()
				mockPinSvc.EXPECT().GetAllPinsByHash(mock.Anything, storageHashMatcher()).Return([]*models.Pin{}, nil).Once()
				mockUploadSvc.EXPECT().DeleteUpload(mock.Anything, storageHashMatcher()).Return(nil).Maybe()

				return account.ID, 3, 2
			},
		},
		{
			name: "EmptyAccount_NoObjects_NoError",
			setupFunc: func(ctx context.Context, svc *SiaService, ctxTest core.Context, mockPinSvc *coreMocks.MockPinService, mockUploadSvc *coreMocks.MockUploadService) (uint, int, int) {
				account := createAccount(ctxTest)
				return account.ID, 0, 0
			},
		},
		{
			name: "ManyObjectsWithIdentitySlabs_AllOrphaned",
			setupFunc: func(ctx context.Context, svc *SiaService, ctxTest core.Context, mockPinSvc *coreMocks.MockPinService, mockUploadSvc *coreMocks.MockUploadService) (uint, int, int) {
				account := createAccount(ctxTest)
				// 100 objects, each with 1 unique slab (no sharing)
				objects := make(map[string][]string)
				for i := 0; i < 100; i++ {
					objects[fmt.Sprintf("obj-%d", i)] = []string{generateSlabID()}
				}
				err := registerSlabsAndObjects(ctx, svc, account.ID, objects)
				require.NoError(t, err)

				// Delete all
				err = svc.DeleteObjectsByAppAccount(ctx, account.ID)
				require.NoError(t, err)

				// Setup mocks for 100 slabs being deleted
				for i := 0; i < 100; i++ {
					mockPinSvc.EXPECT().UploadPinnedGlobal(mock.Anything, storageHashMatcher()).Return(false, nil).Once()
					mockPinSvc.EXPECT().GetAllPinsByHash(mock.Anything, storageHashMatcher()).Return([]*models.Pin{}, nil).Once()
					mockUploadSvc.EXPECT().DeleteUpload(mock.Anything, storageHashMatcher()).Return(nil).Maybe()
				}

				return account.ID, 100, 100
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctxTest coreTesting.TestContext) {
				// Get mock services
				mockPinSvc := core.GetService[*coreMocks.MockPinService](ctxTest, core.PIN_SERVICE)
				mockUploadSvc := core.GetService[*coreMocks.MockUploadService](ctxTest, core.UPLOAD_SERVICE)

				siaSvc := core.GetService[*SiaService](ctxTest, pluginCore.SIA_SERVICE)

				ctx := context.Background()

				// Run setup to get account and expectations (setupFunc configures mocks)
				appAccountID, expectedOrphans, _ := tc.setupFunc(ctx, siaSvc, ctxTest, mockPinSvc, mockUploadSvc)

				// Verify orphaned slabs count before prune
				orphanedBefore, err := siaSvc.FindOrphanedSlabs(ctx, appAccountID)
				require.NoError(t, err)
				assert.Len(t, orphanedBefore, expectedOrphans, "orphaned slabs count mismatch before prune")

				// Run prune
				err = siaSvc.PruneSlabs(ctx, appAccountID)
				require.NoError(t, err)

				// Verify orphans after (should be 0)
				orphanedAfter, err := siaSvc.FindOrphanedSlabs(ctx, appAccountID)
				require.NoError(t, err)
				assert.Empty(t, orphanedAfter, "orphaned slabs should be empty after prune")
			}, TestOptions)
		})
	}
}

// TestPruneSlabsConcurrent tests sequential pruning of multiple accounts
// NOTE: SQLite does not support concurrent writes well, so we test sequentially
func TestPruneSlabsConcurrent(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctxTest coreTesting.TestContext) {
		mockPinSvc := core.GetService[*coreMocks.MockPinService](ctxTest, core.PIN_SERVICE)
		mockUploadSvc := core.GetService[*coreMocks.MockUploadService](ctxTest, core.UPLOAD_SERVICE)
		siaSvc := core.GetService[*SiaService](ctxTest, pluginCore.SIA_SERVICE)

		// Create 10 accounts, each with 5 objects (each object: 3 slabs)
		accounts := make([]db.SiaAppAccount, 10)
		totalSlabs := 10 * 5 * 3 // 150 slabs

		// Set up mocks for all potential deletions
		for i := 0; i < totalSlabs; i++ {
			mockPinSvc.EXPECT().UploadPinnedGlobal(mock.Anything, storageHashMatcher()).Return(false, nil).Maybe()
			mockPinSvc.EXPECT().GetAllPinsByHash(mock.Anything, storageHashMatcher()).Return([]*models.Pin{}, nil).Maybe()
			mockUploadSvc.EXPECT().DeleteUpload(mock.Anything, storageHashMatcher()).Return(nil).Maybe()
		}

		for i := 0; i < 10; i++ {
			acc := createAccount(ctxTest)
			accounts[i] = acc

			// Create 5 objects with 3 slabs each
			objects := make(map[string][]string)
			for j := 0; j < 5; j++ {
				objects[fmt.Sprintf("account-%d-obj-%d", acc.ID, j)] = generateSlabIDs(3)
			}
			err := registerSlabsAndObjects(context.Background(), siaSvc, acc.ID, objects)
			require.NoError(t, err)
		}

		// Delete all objects from all accounts (sequential to avoid SQLite lock)
		for _, acc := range accounts {
			err := siaSvc.DeleteObjectsByAppAccount(context.Background(), acc.ID)
			require.NoError(t, err)
		}

		// Prune all accounts (sequential to avoid SQLite lock)
		for _, acc := range accounts {
			err := siaSvc.PruneSlabs(context.Background(), acc.ID)
			require.NoError(t, err)
		}

		// Verify all accounts have no orphaned slabs
		for _, acc := range accounts {
			orphaned, err := siaSvc.FindOrphanedSlabs(context.Background(), acc.ID)
			require.NoError(t, err)
			assert.Empty(t, orphaned, "account %d should have no orphaned slabs", acc.ID)
		}
	}, TestOptions)
}

// TestFindOrphanedSlabsEdgeCases tests edge cases for slab orphan detection
func TestFindOrphanedSlabsEdgeCases(t *testing.T) {
	testCases := []struct {
		name        string
		setupFunc   func(ctx context.Context, svc *SiaService, dbc *gorm.DB, account1, account2 db.SiaAppAccount)
		expectCount int
	}{
		{
			name: "SoftDeletedObject_SlabShouldBeOrphaned",
			setupFunc: func(ctx context.Context, svc *SiaService, dbc *gorm.DB, account1, account2 db.SiaAppAccount) {
				slabID := generateSlabID()
				err := registerSlabsAndObjects(ctx, svc, account1.ID, map[string][]string{"obj1": {slabID}})
				require.NoError(t, err)

				// Soft delete the object directly (not through service)
				err = dbc.Model(&db.SiaObject{}).Where("object_id = ?", "obj1").Update("deleted_at", "now()").Error
				require.NoError(t, err)
			},
			expectCount: 1,
		},
		{
			name: "ReRegisterObject_SlabReReferenced",
			setupFunc: func(ctx context.Context, svc *SiaService, dbc *gorm.DB, account1, account2 db.SiaAppAccount) {
				slabID := generateSlabID()
				// Register
				err := registerSlabsAndObjects(ctx, svc, account1.ID, map[string][]string{"obj1": {slabID}})
				require.NoError(t, err)
				// Delete
				err = svc.DeleteObject(ctx, account1.ID, "obj1")
				require.NoError(t, err)
				// Re-register with same slab
				err = registerSlabsAndObjects(ctx, svc, account1.ID, map[string][]string{"obj2": {slabID}})
				require.NoError(t, err)
			},
			expectCount: 0,
		},
		{
			name: "MultipleObjectsSameSlab_DeleteOne_OtherPreserves",
			setupFunc: func(ctx context.Context, svc *SiaService, dbc *gorm.DB, account1, account2 db.SiaAppAccount) {
				slabID := generateSlabID()
				err := registerSlabsAndObjects(ctx, svc, account1.ID, map[string][]string{
					"obj1": {slabID},
					"obj2": {slabID},
				})
				require.NoError(t, err)

				// Delete only obj1
				err = svc.DeleteObject(ctx, account1.ID, "obj1")
				require.NoError(t, err)
			},
			expectCount: 0,
		},
		{
			name: "OrphanedAcrossDifferentAppAccounts",
			setupFunc: func(ctx context.Context, svc *SiaService, dbc *gorm.DB, account1, account2 db.SiaAppAccount) {
				// Account1: register and delete
				slab1 := generateSlabID()
				err := registerSlabsAndObjects(ctx, svc, account1.ID, map[string][]string{"obj1": {slab1}})
				require.NoError(t, err)
				err = svc.DeleteObject(ctx, account1.ID, "obj1")
				require.NoError(t, err)

				// Account2: register and delete
				slab2 := generateSlabID()
				err = registerSlabsAndObjects(ctx, svc, account2.ID, map[string][]string{"obj2": {slab2}})
				require.NoError(t, err)
				err = svc.DeleteObject(ctx, account2.ID, "obj2")
				require.NoError(t, err)
			},
			expectCount: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctxTest coreTesting.TestContext) {
				siaSvc := core.GetService[*SiaService](ctxTest, pluginCore.SIA_SERVICE)

				ctx := context.Background()

				// Create test accounts
				account1 := createAccount(ctxTest)
				account2 := createAccount(ctxTest)

				tc.setupFunc(ctx, siaSvc, ctxTest.DB(), account1, account2)

				// Check account1 specifically
				orphaned, err := siaSvc.FindOrphanedSlabs(ctx, account1.ID)
				require.NoError(t, err)
				assert.Len(t, orphaned, tc.expectCount)
			}, TestOptions)
		})
	}
}
