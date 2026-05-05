package internal

import (
	"encoding/hex"
	"fmt"

	mh "github.com/multiformats/go-multihash"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	core "go.lumeweb.com/portal/core"
)

// NewSiaHash creates a StorageHash from a hex-encoded BLAKE2b-256 digest string
// as returned by indexd (e.g. SlabID.String(), ObjectID.String()).
// The input is the raw 64-character hex of an already-computed blake2b-256 hash.
func NewSiaHash(s string) (core.StorageHash, error) {
	hashDigest, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid hex-encoded hash: %w", err)
	}
	encoded, err := mh.Encode(hashDigest, pluginCore.HashTypeBLAKE2b256)
	if err != nil {
		return nil, fmt.Errorf("multihash encode failed: %w", err)
	}
	return StorageHashFromMultihash(encoded), nil
}

// StorageHashFromMultihash creates a StorageHash from an existing multihash byte slice.
func StorageHashFromMultihash(data []byte) core.StorageHash {
	return core.NewStorageHashFromMultihash(data, pluginCore.HashTypeBLAKE2b256, nil)
}
