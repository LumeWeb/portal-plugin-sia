package dto

import (
	"time"

	"go.lumeweb.com/httputil"
	"go.lumeweb.com/portal-plugin-sia/core"
)

var (
	_ httputil.DTOResponse[*core.AppsSummary] = (*AppsSummaryResponse)(nil)
)

// AppSummaryResponse describes a single app account in the summary response.
type AppSummaryResponse struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	LogoURL     string    `json:"logoURL"`
	ServiceURL  string    `json:"serviceURL"`
	PinnedData  uint64    `json:"pinnedData"`
	LastUsed    time.Time `json:"lastUsed"`
}

// AppsSummaryResponse is the response for GET /api/apps.
type AppsSummaryResponse struct {
	AppCount int                  `json:"appCount"`
	Apps     []AppSummaryResponse `json:"apps"`
}

func (r *AppsSummaryResponse) FromModel(model *core.AppsSummary) error {
	if model == nil {
		return nil
	}
	r.AppCount = model.AppCount
	r.Apps = make([]AppSummaryResponse, 0, len(model.Apps))
	for _, app := range model.Apps {
		r.Apps = append(r.Apps, AppSummaryResponse{
			Name:        app.Name,
			Description: app.Description,
			LogoURL:     app.LogoURL,
			ServiceURL:  app.ServiceURL,
			PinnedData:  app.PinnedData,
			LastUsed:    app.LastUsed,
		})
	}
	return nil
}
