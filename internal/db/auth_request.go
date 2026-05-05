package db

import (
	"time"

	"gorm.io/gorm"
)

const authRequestExpiry = 10 * time.Minute

// AuthRequest stores the mapping between an indexd auth request ID and the portal user
// who initiated the request. This allows the approval endpoint to look up the user's
// ConnectKey without requiring JWT authentication—the requestID itself authenticates.
type AuthRequest struct {
	gorm.Model
	RequestID string
	UserID    uint
}

// TableName returns the table name for this model
func (AuthRequest) TableName() string {
	return "sia_auth_requests"
}

// IsExpired checks if the auth request has expired (10 minutes after creation)
func (ar *AuthRequest) IsExpired() bool {
	return time.Now().After(ar.CreatedAt.Add(authRequestExpiry))
}
