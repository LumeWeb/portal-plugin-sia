package db

import (
	"database/sql/driver"
	"fmt"

	"go.sia.tech/core/types"
)

// DBAccountKey wraps types.PublicKey for GORM database storage as raw bytes
// (BLOB/bytea), matching indexd's storage format.
type DBAccountKey types.PublicKey

// Value implements driver.Valuer — stores the public key as raw 32 bytes.
func (k DBAccountKey) Value() (driver.Value, error) {
	return k[:], nil
}

// Scan implements sql.Scanner — reads raw 32 bytes from the database.
func (k *DBAccountKey) Scan(src any) error {
	b, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into DBAccountKey", src)
	}
	if len(b) != len(k) {
		return fmt.Errorf("invalid DBAccountKey length: got %d, want %d", len(b), len(k))
	}
	copy(k[:], b)
	return nil
}

// GormDataType returns the GORM schema type for this field.
func (DBAccountKey) GormDataType() string {
	return "bytes"
}

// PublicKey returns the underlying types.PublicKey.
func (k DBAccountKey) PublicKey() types.PublicKey {
	return types.PublicKey(k)
}

// DBAccountKeyFromPublicKey creates a DBAccountKey from a types.PublicKey.
func DBAccountKeyFromPublicKey(pk types.PublicKey) DBAccountKey {
	return DBAccountKey(pk)
}
