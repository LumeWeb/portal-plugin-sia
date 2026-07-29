package sia

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal-plugin-sia/internal"
	siaDB "go.lumeweb.com/portal-plugin-sia/internal/db"
	core "go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	dbModels "go.lumeweb.com/portal/db/models"
)

// slabMetadataJSON builds the {"redundancy": {...}} JSON that indexd.go stores
// in Upload.Metadata via quotaCore.EncodeSlabMetadata.
func slabMetadataJSON(minShards, totalSectors, dataSize, totalSize uint64) datatypes.JSON {
	m := map[string]interface{}{
		"redundancy": map[string]interface{}{
			"minShards":    minShards,
			"totalSectors": totalSectors,
			"dataSize":     dataSize,
			"totalSize":    totalSize,
		},
	}
	b, _ := json.Marshal(m)
	return b
}

// randomHash generates a random 32-byte hex string for use as a Sia hash digest.
func randomHash() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// createSlabUpload creates a Upload row representing a Sia slab pin.
// Size = dataSize (before redundancy), Metadata contains SlabMetadata with totalSize.
func createSlabUpload(t *testing.T, ctx coreTesting.TestContext, dataSize, totalSize uint64) *dbModels.Upload {
	hashStr := randomHash()

	encoded, err := internal.NewSiaHash(hashStr)
	require.NoError(t, err)

	upload := &dbModels.Upload{
		Hash:     encoded.Multihash(),
		MimeType: pluginCore.MimeTypeSiaSlab,
		Protocol: internal.ProtocolName,
		Size:     dataSize,
		Metadata: slabMetadataJSON(3, 9, dataSize, totalSize),
	}
	err = ctx.DB().Create(upload).Error
	require.NoError(t, err)
	return upload
}

// createVirtualObjectUpload creates a Upload row representing a Sia virtual object pin.
// Size = 0 (no physical storage), Metadata still has SlabMetadata but TotalSize = DataSize.
func createVirtualObjectUpload(t *testing.T, ctx coreTesting.TestContext, dataSize uint64) *dbModels.Upload {
	hashStr := randomHash()
	encoded, err := internal.NewSiaHash(hashStr)
	require.NoError(t, err)

	upload := &dbModels.Upload{
		Hash:     encoded.Multihash(),
		MimeType: pluginCore.MimeTypeSiaObject,
		Protocol: internal.ProtocolName,
		Size:     0,
		Metadata: slabMetadataJSON(0, 1, dataSize, dataSize),
	}
	err = ctx.DB().Create(upload).Error
	require.NoError(t, err)
	return upload
}

// createSiaSlab creates a row in the sia_slabs table.
func createSiaSlab(t *testing.T, ctx coreTesting.TestContext, appAccountID uint, slabID string) *siaDB.SiaSlab {
	slab := &siaDB.SiaSlab{
		SiaAppAccountID: appAccountID,
		SlabID:          slabID,
	}
	err := ctx.DB().Create(slab).Error
	require.NoError(t, err)
	return slab
}

func TestStorageStats_Empty(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		statsCache.Store(nil)
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		stats, err := siaSvc.StorageStats(ctx)

		require.NoError(tb, err)
		require.NotNil(tb, stats)
		assert.Equal(tb, uint64(0), stats.ObjectCount)
		assert.Equal(tb, uint64(0), stats.StorageBytes)
		assert.Equal(tb, uint64(0), stats.PhysicalStorageBytes)
		assert.Equal(tb, uint64(0), stats.PhysicalUnitCount)
	}, TestOptions)
}

func TestStorageStats_OnlySlabs(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		statsCache.Store(nil)
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		// Create 3 slab uploads with known sizes.
		// Slab 1: dataSize=1000, totalSize=3000 (3x redundancy)
		createSlabUpload(t, ctx, 1000, 3000)
		// Slab 2: dataSize=2000, totalSize=6000
		createSlabUpload(t, ctx, 2000, 6000)
		// Slab 3: dataSize=500, totalSize=1500
		createSlabUpload(t, ctx, 500, 1500)

		// Create 2 sia_slabs rows.
		createSiaSlab(t, ctx, 1, randomHash())
		createSiaSlab(t, ctx, 1, randomHash())

		stats, err := siaSvc.StorageStats(ctx)

		require.NoError(tb, err)
		require.NotNil(tb, stats)
		assert.Equal(tb, uint64(3), stats.ObjectCount)
		assert.Equal(tb, uint64(3500), stats.StorageBytes)         // 1000+2000+500
		assert.Equal(tb, uint64(10500), stats.PhysicalStorageBytes) // 3000+6000+1500
		assert.Equal(tb, uint64(2), stats.PhysicalUnitCount)
	}, TestOptions)
}

func TestStorageStats_ExcludesVirtualObjects(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		statsCache.Store(nil)
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		// Create 2 slab uploads.
		createSlabUpload(t, ctx, 1000, 3000)
		createSlabUpload(t, ctx, 2000, 6000)

		// Create 5 virtual object uploads (should be excluded from all counts).
		for i := 0; i < 5; i++ {
			createVirtualObjectUpload(t, ctx, 500)
		}

		// Create 1 sia_slab row.
		createSiaSlab(t, ctx, 1, randomHash())

		stats, err := siaSvc.StorageStats(ctx)

		require.NoError(tb, err)
		require.NotNil(tb, stats)
		// ObjectCount should be 2 (slabs only), not 7 (slabs + virtual objects).
		assert.Equal(tb, uint64(2), stats.ObjectCount)
		assert.Equal(tb, uint64(3000), stats.StorageBytes)          // 1000+2000
		assert.Equal(tb, uint64(9000), stats.PhysicalStorageBytes)   // 3000+6000
		assert.Equal(tb, uint64(1), stats.PhysicalUnitCount)
	}, TestOptions)
}

func TestStorageStats_MixedSlabsAndObjects(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		statsCache.Store(nil)
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		// Create slabs and virtual objects in interleaved order.
		createSlabUpload(t, ctx, 1000, 3000)
		createVirtualObjectUpload(t, ctx, 1000)
		createSlabUpload(t, ctx, 4000, 12000)
		createVirtualObjectUpload(t, ctx, 4000)
		createSlabUpload(t, ctx, 100, 300)

		// Create 3 sia_slabs rows.
		createSiaSlab(t, ctx, 1, randomHash())
		createSiaSlab(t, ctx, 1, randomHash())
		createSiaSlab(t, ctx, 2, randomHash())

		stats, err := siaSvc.StorageStats(ctx)

		require.NoError(tb, err)
		require.NotNil(tb, stats)
		assert.Equal(tb, uint64(3), stats.ObjectCount)              // 3 slabs only
		assert.Equal(tb, uint64(5100), stats.StorageBytes)          // 1000+4000+100
		assert.Equal(tb, uint64(15300), stats.PhysicalStorageBytes)  // 3000+12000+300
		assert.Equal(tb, uint64(3), stats.PhysicalUnitCount)        // 3 sia_slabs rows
	}, TestOptions)
}

func TestStorageStats_SoftDeletedExcluded(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		statsCache.Store(nil)
		siaSvc := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

		// Create 2 active slab uploads.
		createSlabUpload(t, ctx, 1000, 3000)
		createSlabUpload(t, ctx, 2000, 6000)

		// Create 1 soft-deleted slab upload.
		deletedUpload := createSlabUpload(t, ctx, 500, 1500)
		err := ctx.DB().Delete(deletedUpload).Error
		require.NoError(tb, err)

		// Create 1 active and 1 soft-deleted sia_slab.
		createSiaSlab(t, ctx, 1, randomHash())
		deletedSlab := createSiaSlab(t, ctx, 1, randomHash())
		err = ctx.DB().Delete(deletedSlab).Error
		require.NoError(tb, err)

		stats, err := siaSvc.StorageStats(ctx)

		require.NoError(tb, err)
		require.NotNil(tb, stats)
		assert.Equal(tb, uint64(2), stats.ObjectCount)            // 2 active slabs
		assert.Equal(tb, uint64(3000), stats.StorageBytes)        // 1000+2000
		assert.Equal(tb, uint64(9000), stats.PhysicalStorageBytes) // 3000+6000
		assert.Equal(tb, uint64(1), stats.PhysicalUnitCount)       // 1 active sia_slab
	}, TestOptions)
}
