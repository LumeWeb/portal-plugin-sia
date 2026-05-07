package api

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/labstack/echo/v4"
	jwt "go.lumeweb.com/portal-middleware/auth/jwt"
	middleware "go.lumeweb.com/portal-middleware/middleware"
	"go.lumeweb.com/portal-plugin-sia/internal"
	"go.lumeweb.com/portal-plugin-sia/internal/api/dto"
	pluginConfig "go.lumeweb.com/portal-plugin-sia/internal/config"
	router "go.lumeweb.com/portal-router"
	"go.lumeweb.com/portal/config"
	core "go.lumeweb.com/portal/core"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"

	// Import indexd types for swagger documentation
	indexdApp "go.sia.tech/indexd/api/app"
	"go.sia.tech/indexd/hosts"
	"go.sia.tech/indexd/slabs"
)

var _ core.API = (*API)(nil)

// tag constants for OpenAPI route grouping
const (
	tagAccounts = "accounts"
	tagHosts    = "hosts"
	tagObjects  = "objects"
	tagSlabs    = "slabs"
	tagAuth     = "auth"
)

// routeDef defines a proxy route with its swagger metadata
type routeDef struct {
	method  string
	path    string
	summary string
	desc    string
	tags    []string
	swagger []router.SwaggerOption
}

// proxyRouteDefinitions defines read-only indexd proxy routes with their swagger metadata.
// Mutating routes (pin, unpin, prune) have dedicated intercept handlers.
var proxyRouteDefinitions = []routeDef{
	{
		method:  http.MethodGet,
		path:    "/account",
		summary: "Retrieve account details",
		desc:    "Retrieves details of the current authenticated account including used storage, remaining storage, and account status.",
		tags:    []string{tagAccounts},
		swagger: []router.SwaggerOption{
			router.WithSuccessResponse(http.StatusOK, "Account details",
				router.WithJSONContent(indexdApp.AccountResponse{}),
			),
		},
	},
	{
		method:  http.MethodGet,
		path:    "/hosts",
		summary: "List usable hosts",
		desc:    "Returns a list of usable hosts on the Sia network with their addresses and geographic information. Can be filtered by protocol.",
		tags:    []string{tagHosts},
		swagger: []router.SwaggerOption{
			router.WithQueryParam("limit", "Maximum number of hosts to return (1-500, default 100)", int(100)),
			router.WithQueryParam("offset", "Number of hosts to skip (default 0)", int(0)),
			router.WithQueryParam("protocol", "Filter hosts by protocol (siamux or quic)", ""),
			router.WithSuccessResponse(http.StatusOK, "List of usable hosts",
				router.WithJSONContent(struct {
					Hosts []hosts.HostInfo `json:"hosts"`
				}{}),
			),
		},
	},
	{
		method:  http.MethodGet,
		path:    "/objects",
		summary: "List all objects",
		desc:    "Lists all objects for the authenticated account with pagination. Returns object events including deleted objects.",
		tags:    []string{tagObjects},
		swagger: []router.SwaggerOption{
			router.WithQueryParam("limit", "Maximum number of objects to return (1-500, default 100)", int(100)),
			router.WithQueryParam("key", "Object key to use as pagination offset (256-bit hash)", ""),
			router.WithQueryParam("after", "Timestamp to use as pagination offset (RFC3339 format)", ""),
			router.WithSuccessResponse(http.StatusOK, "List of object events",
				router.WithJSONContent(dto.ObjectListResponse{}),
			),
		},
	},
	{
		method:  http.MethodGet,
		path:    "/objects/:key",
		summary: "Get object details",
		desc:    "Retrieves details of a specific object by its key. Returns the sealed object with encrypted data key and slab information.",
		tags:    []string{tagObjects},
		swagger: []router.SwaggerOption{
			router.WithPathParam("key", "Object key identifier (256-bit hash)", ""),
			router.WithSuccessResponse(http.StatusOK, "Object details",
				router.WithJSONContent(slabs.SealedObject{}),
			),
		},
	},
	{
		method:  http.MethodGet,
		path:    "/objects/:key/shared",
		summary: "Get shared object",
		desc:    "Retrieves a shared object that can be accessed without account context. Contains all information needed to retrieve and decrypt the object.",
		tags:    []string{tagObjects},
		swagger: []router.SwaggerOption{
			router.WithPathParam("key", "Object key identifier (256-bit hash)", ""),
			router.WithSuccessResponse(http.StatusOK, "Shared object",
				router.WithJSONContent(slabs.SharedObject{}),
			),
			router.WithErrorResponses(
				router.DefineSwaggerErrorResponses(
					router.DefineSwaggerErrorResponse(http.StatusNotFound, "Object not found"),
				),
			),
		},
	},
	{
		method:  http.MethodGet,
		path:    "/slabs",
		summary: "List all pinned slabs",
		desc:    "Lists all slab IDs pinned by the authenticated account with pagination.",
		tags:    []string{tagSlabs},
		swagger: []router.SwaggerOption{
			router.WithQueryParam("limit", "Maximum number of slab IDs to return (1-500, default 100)", int(100)),
			router.WithQueryParam("offset", "Number of slab IDs to skip (default 0)", int(0)),
			router.WithSuccessResponse(http.StatusOK, "List of pinned slabs",
				router.WithJSONContent(struct {
					Slabs []slabs.SlabID `json:"slabs"`
				}{}),
			),
		},
	},
	{
		method:  http.MethodGet,
		path:    "/slabs/:slabid",
		summary: "Get slab details",
		desc:    "Retrieves details of a specific pinned slab including encryption key, minimum shards, and sector locations.",
		tags:    []string{tagSlabs},
		swagger: []router.SwaggerOption{
			router.WithPathParam("slabid", "Slab ID (256-bit hash)", ""),
			router.WithSuccessResponse(http.StatusOK, "Slab details",
				router.WithJSONContent(slabs.PinnedSlab{}),
			),
			router.WithErrorResponses(
				router.DefineSwaggerErrorResponse(http.StatusNotFound, "Slab not found"),
			),
		},
	},

	{
		method:  http.MethodGet,
		path:    "/auth/check",
		summary: "Check application authentication",
		desc:    "Checks if the application is authenticated with a valid signature.",
		tags:    []string{tagAuth},
		swagger: []router.SwaggerOption{
			router.WithSuccessResponse(http.StatusNoContent, "Application is authenticated"),
		},
	},

}

type API struct {
	*core.BaseComponent
	protocolConfig *pluginConfig.ProtocolConfig
	proxy          http.Handler
	siaSvc         pluginCore.SiaService
}

func NewAPI() (core.API, []core.ContextBuilderOption, error) {
	svc := &API{}
	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			svc.protocolConfig = core.GetProtocolConfig[*pluginConfig.ProtocolConfig](ctx, internal.ProtocolName)
			svc.siaSvc = core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

			appURL := resolveAppURLFromCtx(ctx, svc.protocolConfig)
			target, err := url.Parse(appURL)
			if err != nil {
				return err
			}
			svc.proxy = &httputil.ReverseProxy{
				Director: func(req *http.Request) {
					req.URL.Scheme = target.Scheme
					req.URL.Host = target.Host
					req.Host = target.Host
				},
			}

			return nil
		}),
	)
	return svc, opts, nil
}

func (a *API) ID() string {
	return a.Name()
}

func (a *API) Name() string {
	return internal.ProtocolName
}

func (a *API) resolveAppURL() string {
	if a.protocolConfig.AppURL != "" {
		return a.protocolConfig.AppURL
	}
	httpSvc := core.GetService[core.HTTPService](a.Context(), core.HTTP_SERVICE)
	return httpSvc.APISubdomain(a.ID(), true)
}

func resolveAppURLFromCtx(ctx core.Context, protocolConfig *pluginConfig.ProtocolConfig) string {
	if protocolConfig.AppURL != "" {
		return protocolConfig.AppURL
	}
	httpSvc := core.GetService[core.HTTPService](ctx, core.HTTP_SERVICE)
	return httpSvc.APISubdomain(internal.ProtocolName, true)
}

// buildAppURL constructs a URL to the indexd app API with the given path,
// preserving signed URL query parameters from the original request.
func (a *API) buildAppURL(originalURL *url.URL, path string) (string, error) {
	base, err := url.Parse(a.resolveAppURL())
	if err != nil {
		return "", fmt.Errorf("invalid app URL: %w", err)
	}
	base.Path = path
	base.RawQuery = originalURL.RawQuery
	return base.String(), nil
}

func (a *API) Subdomain() string {
	return internal.ProtocolName
}

func (a *API) AuthTokenName() string {
	return core.AUTH_TOKEN_NAME
}

func (a *API) GetConfig() config.APIConfig {
	return &pluginConfig.APIConfig{}
}

func (a *API) OpenAPIInfo() router.APIInfoDefinition {
	return router.APIInfo().
		Title("Sia API").
		Version("0.1.0").
		Description("Sia storage API for uploading and managing data on the Sia network. Supports slab and object operations with quota tracking and account management.").
		License("MIT", "https://opensource.org/licenses/MIT")
}

func (a *API) Configure(r router.Router, accessSvc core.AccessService) error {
	httpSvc := core.GetService[core.HTTPService](a.Context(), core.HTTP_SERVICE)
	siaMw := SiaSignedURLMiddleware(a.Context(), httpSvc.APISubdomain(a.ID(), false))

	siaOpts := []router.RouteOption{
		router.WithMiddlewares(siaMw),
		router.WithCors(),
	}

	// Intercept routes for mutating operations (pin, unpin, prune) use Sia signed URL auth.
	// These handlers validate quota/existence, proxy to indexd, then record in our DB.
	interceptRoutes := []router.RouteDefinition{
		buildPinnedSlabRoute(a),
		buildDeleteSlabRoute(a),
		buildPruneSlabsRoute(a),
		buildPinObjectRoute(a),
		buildDeleteObjectRoute(a),
	}
	router.RegisterRoutes(r, accessSvc, a.Subdomain(), interceptRoutes, siaOpts...)

	// Read-only proxy routes use Sia signed URL auth
	proxyRoutes := buildProxyRoutes(a)
	router.RegisterRoutes(r, accessSvc, a.Subdomain(), proxyRoutes, siaOpts...)

	// Connect approval route (requires JWT auth only, no Sia signed URL)
	// Connect init is in signedRoutes below (anonymous, no auth required)
	// Connect register uses custom handler to create SiaAppAccount record
	connectRoutes := buildConnectRoutes(a)
	router.RegisterRoutes(r, accessSvc, a.Subdomain(), connectRoutes)

	// Connect register route (anonymous — indexd validates signed URL, creates SiaAppAccount on success)
	connectRegisterRoutes := buildConnectRegisterRoutes(a)
	router.RegisterRoutes(r, accessSvc, a.Subdomain(), connectRegisterRoutes)

	// Connect public routes (no auth — indexd validates signed URL for status, UI page handles auth internally)
	connectPublicRoutes := []router.RouteDefinition{
		router.NewRoute(http.MethodGet, "/auth/connect/:requestID", a.HandleGETAuthConnect,
			router.WithMiddlewares(middleware.AuthMiddleware(a.Context(),
				middleware.WithAuthPurpose(jwt.PurposeLogin, jwt.PurposeAPI),
				middleware.WithAuthEmptyAllowed(true),
			)),
			router.WithSwagger(
				router.WithSummary("Get connection request UI"),
				router.WithDescription("Returns an HTML page for the user to approve or reject the application connection request. Unauthenticated users are redirected to the portal login page."),
				router.WithTags(tagAuth),
				router.WithPathParam("requestID", "Connect request ID", ""),
				router.WithSuccessResponse(http.StatusOK, "HTML authorization page"),
				router.WithErrorResponses(
					router.DefineSwaggerErrorResponse(http.StatusNotFound, "Unknown request ID"),
				),
			),
		),
		router.NewRoute(http.MethodGet, "/auth/connect/:requestID/status", echo.WrapHandler(a.proxy),
			router.WithSwagger(
				router.WithSummary("Check connection request status"),
				router.WithDescription("Returns whether the user has approved or rejected the connection request. If approved, includes the userSecret used to derive the application key."),
				router.WithTags(tagAuth),
				router.WithPathParam("requestID", "Connect request ID", ""),
				router.WithSuccessResponse(http.StatusOK, "Connection request status",
					router.WithJSONContent(indexdApp.AuthConnectStatusResponse{}),
				),
				router.WithErrorResponses(
					router.DefineSwaggerErrorResponse(http.StatusNotFound, "Unknown request ID"),
				),
			),
		),
	}
	router.RegisterRoutes(r, accessSvc, a.Subdomain(), connectPublicRoutes)

	// Signed-only routes (no JWT auth required)
	signedRoutes := []router.RouteDefinition{
		router.NewRoute(http.MethodPost, "/auth/connect", a.HandlePOSTAuthConnectInit),
	}
	router.RegisterRoutes(r, accessSvc, a.Subdomain(), signedRoutes)

	return nil
}

func buildPinnedSlabRoute(a *API) router.RouteDefinition {
	return router.NewRoute(http.MethodPost, "/slabs", a.pinSlabHandler,
		router.WithAccess(core.ACCESS_USER_ROLE),
		router.WithSwagger(
			router.WithSummary("Pin a slab to the indexer"),
			router.WithDescription("Pins a slab to the indexer for storage on the Sia network. The slab must include encryption parameters and sector locations."),
			router.WithTags(tagSlabs),
			router.WithRequestBody(slabs.SlabPinParams{}, "Slab pinning parameters including encryption key, min shards, and sector locations", true),
			router.WithSuccessResponse(http.StatusCreated, "Slab pinned successfully",
				router.WithJSONContent(struct {
					SlabID slabs.SlabID `json:"slabID"`
				}{}),
			),
			router.WithErrorResponses(
				router.DefineSwaggerErrorResponse(http.StatusNotFound, "Slab not found"),
			),
		),
	)
}

func buildPruneSlabsRoute(a *API) router.RouteDefinition {
	return router.NewRoute(http.MethodPost, "/slabs/prune", a.pruneSlabsHandler,
		router.WithAccess(core.ACCESS_USER_ROLE),
		router.WithSwagger(
			router.WithSummary("Prune unused slabs"),
			router.WithDescription("Unpins all slabs not referenced by any object currently pinned by the user. This frees up storage quota."),
			router.WithTags(tagSlabs),
			router.WithSuccessResponse(http.StatusOK, "Slabs pruned successfully"),
		),
	)
}

func buildDeleteSlabRoute(a *API) router.RouteDefinition {
	return router.NewRoute(http.MethodDelete, "/slabs/:id", a.unpinSlabHandler,
		router.WithAccess(core.ACCESS_USER_ROLE),
		router.WithSwagger(
			router.WithSummary("Unpin a slab from the indexer"),
			router.WithDescription("Unpins a slab from the indexer. If the slab is no longer referenced by any objects, it will be removed."),
			router.WithTags(tagSlabs),
			router.WithPathParam("id", "Slab ID (256-bit hash)", ""),
			router.WithSuccessResponse(http.StatusNoContent, "Slab unpinned successfully"),
			router.WithErrorResponses(
				router.DefineSwaggerErrorResponse(http.StatusNotFound, "Slab not found"),
			),
		),
	)
}

func buildPinObjectRoute(a *API) router.RouteDefinition {
	return router.NewRoute(http.MethodPost, "/objects", a.pinObjectHandler,
		router.WithAccess(core.ACCESS_USER_ROLE),
		router.WithSwagger(
			router.WithSummary("Create or update an object"),
			router.WithDescription("Creates a new object or updates an existing one. The object must reference slabs that are already pinned. Objects are automatically signed to verify integrity."),
			router.WithTags(tagObjects),
			router.WithRequestBody(slabs.PinObjectRequest{}, "Object pinning request including encrypted data key, slabs, and signatures", true),
			router.WithSuccessResponse(http.StatusNoContent, "Object created or updated successfully"),
			router.WithErrorResponses(
				router.DefineSwaggerErrorResponse(http.StatusBadRequest, "Object has no slabs, metadata exceeds size limit, or object contains unpinned slab"),
			),
		),
	)
}

func buildDeleteObjectRoute(a *API) router.RouteDefinition {
	return router.NewRoute(http.MethodDelete, "/objects/:key", a.unpinObjectHandler,
		router.WithAccess(core.ACCESS_USER_ROLE),
		router.WithSwagger(
			router.WithSummary("Delete an object"),
			router.WithDescription("Deletes an object from the indexer. The underlying slabs are not deleted and may be shared with other objects."),
			router.WithTags(tagObjects),
			router.WithPathParam("key", "Object key identifier (256-bit hash)", ""),
			router.WithSuccessResponse(http.StatusNoContent, "Object deleted successfully"),
			router.WithErrorResponses(
				router.DefineSwaggerErrorResponse(http.StatusNotFound, "Object not found"),
			),
		),
	)
}

func buildProxyRoutes(a *API) []router.RouteDefinition {
	routes := make([]router.RouteDefinition, 0, len(proxyRouteDefinitions))

	for _, def := range proxyRouteDefinitions {
		opts := []router.RouteOption{
			router.WithAccess(core.ACCESS_USER_ROLE),
			router.WithSwagger(
				append([]router.SwaggerOption{
					router.WithSummary(def.summary),
					router.WithDescription(def.desc),
					router.WithTags(def.tags...),
				}, def.swagger...)...,
			),
		}
		routes = append(routes, router.NewRoute(def.method, def.path, a.proxyHandler, opts...))
	}

	return routes
}

func buildConnectRoutes(a *API) []router.RouteDefinition {
	return []router.RouteDefinition{
		// Approve/reject connect - requires JWT auth
		router.NewRoute(http.MethodPost, "/auth/connect/:requestID", a.HandlePOSTAuthConnect,
			router.WithAccess(core.ACCESS_USER_ROLE),
			router.WithMiddlewares(middleware.AuthMiddleware(a.Context(), middleware.WithAuthPurpose(jwt.PurposeLogin, jwt.PurposeAPI))),
			router.WithSwagger(
				router.WithSummary("Approve or reject an application connection request"),
				router.WithDescription("Approves or rejects an application connection request. Requires valid Portal JWT authentication. The connect key is injected server-side based on the authenticated user's account."),
				router.WithTags(tagAuth),
				router.WithPathParam("requestID", "Connect request ID", ""),
				router.WithRequestBody(indexdApp.ApproveAppRequest{}, "Approval request body", true),
				router.WithSuccessResponse(http.StatusNoContent, "Request processed successfully"),
				router.WithErrorResponses(
					router.DefineSwaggerErrorResponses(
						router.DefineSwaggerErrorResponse(http.StatusUnauthorized, "Invalid connect key"),
						router.DefineSwaggerErrorResponse(http.StatusForbidden, "Connect key exhausted"),
						router.DefineSwaggerErrorResponse(http.StatusNotFound, "Request invalid or expired"),
					),
				),
			),
		),
	}
}

func buildConnectRegisterRoutes(a *API) []router.RouteDefinition {
	return []router.RouteDefinition{
		// Register app key - creates SiaAppAccount record on success
		router.NewRoute(http.MethodPost, "/auth/connect/:requestID/register", a.HandlePOSTAuthConnectRegister,
			router.WithAccess(core.ACCESS_USER_ROLE),
			router.WithSwagger(
				router.WithSummary("Finalize application registration"),
				router.WithDescription("Registers the application key after approval. The request must be signed with the ephemeral key from the initial connect request. Creates a SiaAppAccount record linking the app to the approving user."),
				router.WithTags(tagAuth),
				router.WithPathParam("requestID", "Connect request ID", ""),
				router.WithRequestBody(indexdApp.RegisterAppKeyRequest{}, "Application key registration request", true),
				router.WithSuccessResponse(http.StatusOK, "Application registered successfully"),
				router.WithErrorResponses(
					router.DefineSwaggerErrorResponses(
						router.DefineSwaggerErrorResponse(http.StatusUnauthorized, "Signed with different ephemeral key or invalid proof-of-possession"),
						router.DefineSwaggerErrorResponse(http.StatusForbidden, "Connect key exhausted or request not approved"),
						router.DefineSwaggerErrorResponse(http.StatusNotFound, "Unknown request ID"),
						router.DefineSwaggerErrorResponse(http.StatusGone, "Request expired"),
					),
				),
			),
		),
	}
}
