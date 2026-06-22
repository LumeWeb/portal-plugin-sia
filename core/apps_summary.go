package core

import (
	"time"
)

// AppSummary describes a single app account connected to a user's Sia account.
type AppSummary struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	LogoURL     string    `json:"logoURL"`
	ServiceURL  string    `json:"serviceURL"`
	PinnedData  uint64    `json:"pinnedData"`
	LastUsed    time.Time `json:"lastUsed"`
}

// AppsSummary is the aggregate response for the GET /api/apps endpoint.
type AppsSummary struct {
	AppCount int          `json:"appCount"`
	Apps     []AppSummary `json:"apps"`
}
