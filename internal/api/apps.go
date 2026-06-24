package api

import (
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	mcontext "go.lumeweb.com/portal-middleware/context"
	router "go.lumeweb.com/portal-router"
	core "go.lumeweb.com/portal/core"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal-plugin-sia/internal"
	"go.lumeweb.com/portal-plugin-sia/internal/api/dto"
	"go.lumeweb.com/queryutil"
	queryutilHttp "go.lumeweb.com/queryutil/http"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// listAppsHandler returns a paginated, filtered, sorted list of apps
// connected to the user's Sia account. The current user's ID is injected
// as a filter so users can only see their own apps.
func (a *API) listAppsHandler(c echo.Context) error {
	userID, err := mcontext.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "login required")
	}

	return queryutilHttp.ProcessListRequest(
		c.Response(),
		c.Request(),
		"apps",
		func(filters []queryutil.CrudFilter, sorts []queryutil.Sort, pagination queryutil.Pagination) ([]pluginCore.AppAccount, int64, error) {
			return a.siaSvc.ListApps(c.Request().Context(), userID, filters, sorts, pagination)
		},
		func(app pluginCore.AppAccount) dto.AppResponse {
			return dto.AppResponse{
				PublicKey:   hex.EncodeToString(app.PublicKey[:]),
				Name:        app.Name,
				Description: app.Description,
				LogoURL:     app.LogoURL,
				ServiceURL:  app.ServiceURL,
				PinnedData:  app.PinnedData,
				LastUsed:    app.LastUsed,
			}
		},
	)
}

// deleteAppHandler deletes a single app account via the admin client.
// The :pubkey parameter is the hex-encoded ed25519 public key of the app account.
func (a *API) deleteAppHandler(c echo.Context) error {
	pubkeyHex := c.Param("pubkey")
	if pubkeyHex == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing public key")
	}

	pubkey, err := internal.ParsePublicKey(pubkeyHex)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid public key")
	}

	userID, err := mcontext.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "login required")
	}

	// Get the user's Sia account
	account, err := a.siaSvc.GetAccount(c.Request().Context(), userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "sia account not found")
		}
		a.Logger().Error("failed to get sia account", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get account")
	}

	// Delete the app account (verifies ownership, calls admin, removes DB record)
	if err := a.siaSvc.DeleteAppAccount(c.Request().Context(), account.ID, pubkey); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "app account not found")
		}
		a.Logger().Error("failed to delete app account", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete app account")
	}

	return c.NoContent(http.StatusNoContent)
}

// pruneAccountHandler prunes all unreferenced slabs across all of the user's app accounts.
func (a *API) pruneAccountHandler(c echo.Context) error {
	userID, err := mcontext.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "login required")
	}

	if err := a.siaSvc.PruneAccount(c.Request().Context(), userID); err != nil {
		a.Logger().Error("failed to prune account", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to prune account")
	}

	return c.NoContent(http.StatusNoContent)
}

// tagApps is the OpenAPI tag for app management routes.
const tagApps = "Apps"

// buildAppsRoutes returns JWT-authenticated routes for app management.
func buildAppsRoutes(a *API) []router.RouteDefinition {
	authMW := a.jwtAuthMW()

	return []router.RouteDefinition{
		// GET /apps — list all apps (queryutil list endpoint)
		router.NewRoute(http.MethodGet, "/apps", a.listAppsHandler,
			router.WithAccess(core.ACCESS_USER_ROLE),
			authMW,
			router.WithSwagger(
				router.WithSummary("List connected apps"),
				router.WithDescription("Returns a paginated, filtered, sorted list of apps connected to the user's Sia account."),
				router.WithTags(tagApps),
				router.WithSuccessResponse(http.StatusOK, "App list",
					router.WithJSONContent(dto.AppListResponse{}),
				),
				router.WithErrorResponses(
					router.DefineSwaggerErrorResponse(http.StatusUnauthorized, "Login required"),
				),
			),
		),

		// DELETE /apps/:pubkey — delete a single app account by its ed25519 public key
		router.NewRoute(http.MethodDelete, "/apps/:pubkey", a.deleteAppHandler,
			router.WithAccess(core.ACCESS_USER_ROLE),
			authMW,
			router.WithSwagger(
				router.WithSummary("Delete a connected app"),
				router.WithDescription("Deletes an app account by its hex-encoded ed25519 public key via the admin side. Removes the account from the indexer and the local database."),
				router.WithTags(tagApps),
				router.WithPathParam("pubkey", "Hex-encoded ed25519 public key of the app account", ""),
				router.WithSuccessResponse(http.StatusNoContent, "App deleted successfully"),
				router.WithErrorResponses(
					router.DefineSwaggerErrorResponses(
						router.DefineSwaggerErrorResponse(http.StatusBadRequest, "Invalid or missing public key"),
						router.DefineSwaggerErrorResponse(http.StatusNotFound, "App account not found"),
						router.DefineSwaggerErrorResponse(http.StatusUnauthorized, "Login required"),
					),
				),
			),
		),

		// POST /prune — prune unreferenced slabs across all app accounts
		router.NewRoute(http.MethodPost, "/prune", a.pruneAccountHandler,
			router.WithAccess(core.ACCESS_USER_ROLE),
			authMW,
			router.WithSwagger(
				router.WithSummary("Prune unused slabs"),
				router.WithDescription("Prunes all pinned slabs across all of the user's app accounts that are not currently referenced by any object. This is a disk defrag operation."),
				router.WithTags(tagApps),
				router.WithSuccessResponse(http.StatusNoContent, "Prune completed successfully"),
				router.WithErrorResponses(
					router.DefineSwaggerErrorResponse(http.StatusUnauthorized, "Login required"),
				),
			),
		),
	}
}
