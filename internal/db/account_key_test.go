package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.sia.tech/core/types"
)

func TestDBAccountKey_Value(t *testing.T) {
	sk := types.GeneratePrivateKey()
	pk := sk.PublicKey()
	key := DBAccountKeyFromPublicKey(pk)

	val, err := key.Value()
	require.NoError(t, err)

	b, ok := val.([]byte)
	require.True(t, ok, "Value() should return []byte")
	assert.Len(t, b, 32, "Value() should return 32 bytes")
	assert.Equal(t, pk[:], b, "Value() should return raw key bytes")
}

func TestDBAccountKey_Scan(t *testing.T) {
	sk := types.GeneratePrivateKey()
	pk := sk.PublicKey()

	var key DBAccountKey
	err := key.Scan(pk[:])
	require.NoError(t, err)
	assert.Equal(t, DBAccountKey(pk), key, "Scan() should reconstruct the key")
}

func TestDBAccountKey_Scan_InvalidType(t *testing.T) {
	var key DBAccountKey
	err := key.Scan("not-bytes")
	assert.Error(t, err, "Scan() should reject non-[]byte values")
}

func TestDBAccountKey_Scan_InvalidLength(t *testing.T) {
	var key DBAccountKey
	err := key.Scan([]byte{1, 2, 3})
	assert.Error(t, err, "Scan() should reject wrong-length bytes")
}

func TestDBAccountKey_RoundTrip(t *testing.T) {
	sk := types.GeneratePrivateKey()
	pk := sk.PublicKey()

	// Value() → Scan() round trip
	key := DBAccountKeyFromPublicKey(pk)
	val, err := key.Value()
	require.NoError(t, err)

	var key2 DBAccountKey
	err = key2.Scan(val)
	require.NoError(t, err)
	assert.Equal(t, key, key2, "Value/Scan round trip should be identity")
}

func TestDBAccountKey_PublicKey(t *testing.T) {
	sk := types.GeneratePrivateKey()
	pk := sk.PublicKey()
	key := DBAccountKeyFromPublicKey(pk)
	assert.Equal(t, pk, key.PublicKey(), "PublicKey() should return the original key")
}

func TestDBAccountKey_GormDataType(t *testing.T) {
	var key DBAccountKey
	assert.Equal(t, "bytes", key.GormDataType())
}
