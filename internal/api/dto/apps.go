package dto

import (
	"encoding/hex"
	"time"

	"go.lumeweb.com/httputil"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
)

var (
	_ httputil.DTOResponse[*pluginCore.AppAccount] = (*AppResponse)(nil)
)

// AppResponse describes a single app account in the list response.
type AppResponse struct {
	PublicKey   string    `json:"publicKey"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	LogoURL     string    `json:"logoURL"`
	ServiceURL  string    `json:"serviceURL"`
	PinnedData  uint64    `json:"pinnedData"`
	LastUsed    time.Time `json:"lastUsed"`
}

// FromModel converts a core.AppAccount to an AppResponse DTO.
func (r *AppResponse) FromModel(model *pluginCore.AppAccount) error {
	if model == nil {
		return nil
	}
	r.PublicKey = hex.EncodeToString(model.PublicKey[:])
	r.Name = model.Name
	r.Description = model.Description
	r.LogoURL = model.LogoURL
	r.ServiceURL = model.ServiceURL
	r.PinnedData = model.PinnedData
	r.LastUsed = model.LastUsed
	return nil
}

// AppListResponse is a swagger-only DTO that represents the paginated response
// for the GET /api/apps endpoint.
//
// This struct exists due to a TODO bug where queryutil.Response generics are not getting detected
// properly as an array type in the swagger documentation generation. By providing a concrete struct,
// we ensure the swagger docs correctly show the data field as an array of AppResponse items.
//
// Note: This struct is only used for swagger documentation, not for actual encoding.
type AppListResponse struct {
	Data  []AppResponse `json:"data"`
	Total int64         `json:"total"`
}
