package internal

import (
	"encoding/hex"
	"fmt"

	"go.sia.tech/core/types"
)

// ParsePublicKey decodes a hex-encoded ed25519 public key string (64 hex chars / 32 bytes)
// into a types.PublicKey. Returns an error if the input is not valid hex or the wrong length.
func ParsePublicKey(s string) (types.PublicKey, error) {
	keyBytes, err := hex.DecodeString(s)
	if err != nil {
		return types.PublicKey{}, fmt.Errorf("invalid hex encoding for public key: %w", err)
	}
	if len(keyBytes) != len(types.PublicKey{}) {
		return types.PublicKey{}, fmt.Errorf("invalid public key length: expected %d bytes, got %d", len(types.PublicKey{}), len(keyBytes))
	}
	var pk types.PublicKey
	copy(pk[:], keyBytes)
	return pk, nil
}
