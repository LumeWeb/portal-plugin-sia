package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"go.lumeweb.com/httputil"
	jwt "go.lumeweb.com/portal-middleware/auth/jwt"
	mcontext "go.lumeweb.com/portal-middleware/context"
	middleware "go.lumeweb.com/portal-middleware/middleware"
	router "go.lumeweb.com/portal-router"
	core "go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal-plugin-sia/internal"
	"go.lumeweb.com/portal-plugin-sia/internal/api/dto"
	"go.uber.org/zap"
)

// listAppsHandler returns a summary of all apps connected to the user's Sia account.
func (a *API) listAppsHandler(c echo.Context) error {
	ctx := httputil.Context(c)

	userID, err := mcontext.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "login required")
	}

	summary, err := a.siaSvc.GetAppsSummary(c.Request().Context(), userID)
	if err != nil {
		a.Logger().Error("failed to get apps summary", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get apps summary")
	}

	return httputil.EncodeResponse(ctx, summary, &dto.AppsSummaryResponse{})
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
		a.Logger().Error("failed to get sia account", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get account")
	}

	// Delete the app account (verifies ownership, calls admin, removes DB record)
	if err := a.siaSvc.DeleteAppAccount(c.Request().Context(), account.ID, pubkey); err != nil {
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
	authMW := router.WithMiddlewares(middleware.AuthMiddleware(a.Context(),
		middleware.WithAuthPurpose(jwt.PurposeLogin, jwt.PurposeAPI),
	))

	return []router.RouteDefinition{
		// GET /apps — list all apps with usage summary
		router.NewRoute(http.MethodGet, "/apps", a.listAppsHandler,
			router.WithAccess(core.ACCESS_USER_ROLE),
			authMW,
			router.WithSwagger(
				router.WithSummary("List connected apps"),
				router.WithDescription("Returns a summary of all apps connected to the user's Sia account, including per-app usage and aggregate quota."),
				router.WithTags(tagApps),
				router.WithSuccessResponse(http.StatusOK, "Apps summary",
					router.WithJSONContent(dto.AppsSummaryResponse{}),
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
