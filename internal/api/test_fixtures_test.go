package api

import (
	"crypto/ed25519"
	"encoding/json"
	"testing"

	"go.sia.tech/core/types"
	"go.sia.tech/indexd/slabs"
)

// newTestEncryptionKey returns a deterministic EncryptionKey with a recognizable
// byte pattern. The fill byte distinguishes different keys in test output.
func newTestEncryptionKey(fill byte) slabs.EncryptionKey {
	var key slabs.EncryptionKey
	for i := range key {
		key[i] = fill
	}
	return key
}

// newTestHash256 returns a deterministic types.Hash256 where the first byte is
// set to fill and remaining bytes are zero-padded. Useful for sector roots and
// object IDs.
func newTestHash256(fill byte) types.Hash256 {
	var h types.Hash256
	h[0] = fill
	return h
}

// newTestPublicKey returns a valid types.PublicKey derived from a seed, suitable
// for use as a host key in PinnedSector. The seed parameter produces different
// keys to avoid duplicate-host-key validation failures.
func newTestPublicKey(t testing.TB, seed byte) types.PublicKey {
	t.Helper()
	// Derive a deterministic ed25519 key from the seed so toPublicKey() works.
	seedBytes := make([]byte, ed25519.SeedSize)
	seedBytes[0] = seed
	sk := types.NewPrivateKeyFromSeed(seedBytes)
	return sk.PublicKey()
}

// newTestSlabPinParams builds a valid SlabPinParams that passes slabs.Validate().
// Uses minShards=2 with 3 sectors (redundancy ratio 1.5, the minimum accepted).
func newTestSlabPinParams(t testing.TB) slabs.SlabPinParams {
	t.Helper()
	const minShards = 3
	sectors := make([]slabs.PinnedSector, 5) // 5/3 ≈ 1.67 redundancy
	for i := range sectors {
		sectors[i] = slabs.PinnedSector{
			Root:    newTestHash256(byte(i + 1)),
			HostKey: newTestPublicKey(t, byte(i+1)),
		}
	}
	return slabs.SlabPinParams{
		EncryptionKey: newTestEncryptionKey(0xAA),
		MinShards:     minShards,
		Sectors:       sectors,
	}
}

// newTestPinObjectRequest builds a valid PinObjectRequest with proper
// cryptographic signatures for the given account's private key.
// The request references a single slab slice and is signed by sk.
func newTestPinObjectRequest(t testing.TB, sk types.PrivateKey) slabs.PinObjectRequest {
	t.Helper()
	pk := sk.PublicKey()

	slabParams := newTestSlabPinParams(t)
	slabSlice := slabParams.Slice(0, 4096)

	obj := slabs.SealedObject{
		EncryptedDataKey: make([]byte, 32),
		Slabs:            []slabs.SlabSlice{slabSlice},
	}
	for i := range obj.EncryptedDataKey {
		obj.EncryptedDataKey[i] = 0xBB
	}

	obj.Sign(sk)
	if err := obj.VerifySignatures(pk); err != nil {
		t.Fatalf("failed to verify test object signatures: %v", err)
	}

	return obj.PinRequest()
}

// mustMarshalJSON serializes v to JSON or fatals the test.
func mustMarshalJSON(t testing.TB, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	return data
}

// formatSlabID returns the string representation of a SlabID derived from the
// given SlabPinParams.
func formatSlabID(params slabs.SlabPinParams) string {
	return params.Digest().String()
}

// formatObjectID returns the string representation of the object ID for a
// PinObjectRequest.
func formatObjectID(req slabs.PinObjectRequest) string {
	return req.ID.String()
}
