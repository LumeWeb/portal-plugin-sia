package core

import (
	"time"

	"go.sia.tech/core/types"
)

// AppAccount represents an app account with both local DB and indexd data.
type AppAccount struct {
	PublicKey   types.PublicKey
	Name        string
	Description string
	LogoURL     string
	ServiceURL  string
	PinnedData  uint64
	LastUsed    time.Time
}
