package api

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

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
		desc:    "Details of the authenticated account including storage usage and account status.",
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
		desc:    "Usable hosts on the Sia network, filterable by protocol.",
		tags:    []string{tagHosts},
		swagger: []router.SwaggerOption{
			router.WithQueryParam("limit", "Max results (1-500)", int(100)),
			router.WithQueryParam("offset", "Results to skip", int(0)),
			router.WithQueryParam("protocol", "Filter by protocol (siamux or quic)", ""),
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
		desc:    "Objects for the authenticated account with pagination.",
		tags:    []string{tagObjects},
		swagger: []router.SwaggerOption{
			router.WithQueryParam("limit", "Max results (1-500)", int(100)),
			router.WithQueryParam("key", "Pagination offset key", ""),
			router.WithQueryParam("after", "Pagination offset timestamp (RFC3339)", ""),
			router.WithSuccessResponse(http.StatusOK, "List of object events",
				router.WithJSONContent(dto.ObjectListResponse{}),
			),
		},
	},
	{
		method:  http.MethodGet,
		path:    "/objects/:key",
		summary: "Get object details",
		desc:    "Details of an object by its key.",
		tags:    []string{tagObjects},
		swagger: []router.SwaggerOption{
			router.WithPathParam("key", "Object key", ""),
			router.WithSuccessResponse(http.StatusOK, "Object details",
				router.WithJSONContent(slabs.SealedObject{}),
			),
		},
	},
	{
		method:  http.MethodGet,
		path:    "/objects/:key/shared",
		summary: "Get shared object",
		desc:    "A shared object accessible without account context.",
		tags:    []string{tagObjects},
		swagger: []router.SwaggerOption{
			router.WithPathParam("key", "Object key", ""),
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
		desc:    "Slab IDs pinned by the authenticated account with pagination.",
		tags:    []string{tagSlabs},
		swagger: []router.SwaggerOption{
			router.WithQueryParam("limit", "Max results (1-500)", int(100)),
			router.WithQueryParam("offset", "Results to skip", int(0)),
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
		desc:    "Details of a pinned slab including encryption key, minimum shards, and sector locations.",
		tags:    []string{tagSlabs},
		swagger: []router.SwaggerOption{
			router.WithPathParam("slabid", "Slab ID", ""),
			router.WithSuccessResponse(http.StatusOK, "Slab details",
				router.WithJSONContent(slabs.PinnedSlab{}),
			),
			router.WithErrorResponses(
				router.DefineSwaggerErrorResponse(http.StatusNotFound, "Slab not found"),
			),
		},
	},
}

type API struct {
	*core.BaseComponent
	protocolConfig *pluginConfig.ProtocolConfig
	proxy          http.Handler
	siaSvc         pluginCore.SiaService
	httpSvc        core.HTTPService
	pinSvc         core.PinService
	uploadSvc      core.UploadService
	quotaSvc       pluginCore.QuotaService
}

func NewAPI() (core.API, []core.ContextBuilderOption, error) {
	svc := &API{}
	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			svc.protocolConfig = core.GetProtocolConfig[*pluginConfig.ProtocolConfig](ctx, internal.ProtocolName)
			svc.siaSvc = core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)
			svc.httpSvc = core.GetService[core.HTTPService](ctx, core.HTTP_SERVICE)
			svc.pinSvc = core.GetService[core.PinService](ctx, core.PIN_SERVICE)
			svc.uploadSvc = core.GetService[core.UploadService](ctx, core.UPLOAD_SERVICE)
			svc.quotaSvc = core.GetService[pluginCore.QuotaService](ctx, pluginCore.QUOTA_SERVICE)

			target, err := url.Parse(svc.protocolConfig.AppURL)
			if err != nil {
				return fmt.Errorf("invalid app_url: %w", err)
			}
			publicHost := svc.httpSvc.APISubdomain(internal.ProtocolName, false)
			svc.proxy = &httputil.ReverseProxy{
				Director: func(req *http.Request) {
					req.URL.Scheme = target.Scheme
					req.URL.Host = target.Host
					req.Host = publicHost
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

func (a *API) resolveProxyURL() string {
	return a.protocolConfig.AppURL
}

// resolvePublicHost returns the hostname that indexd expects in the Host header.
// Indexd validates jc.Request.Host against its advertiseURL hostname, which in
// the deployed environment matches the portal's public subdomain (since all
// requests route through the portal). Manual proxy requests must set this Host
// header to pass indexd's hostname validation, even though they connect to the
// internal AppURL target.
func (a *API) resolvePublicHost() string {
	host := a.httpSvc.APISubdomain(a.ID(), false)
	return a.appendPort(host)
}

// resolvePublicURL returns the full public URL for this API, including scheme
// and port (for non-standard ports). This is used for URLs returned to clients
// that they must be able to reach directly (e.g. status URLs, redirects).
func (a *API) resolvePublicURL() string {
	url := a.httpSvc.APISubdomain(a.ID(), true)
	return a.appendPort(url)
}

// appendPort appends the public-facing port to a base URL if it is non-standard
// (not 80 or 443). It respects ExternalPort config when set.
func (a *API) appendPort(base string) string {
	port := uint16(a.Config().Config().Core.ExternalPort)
	if port == 0 {
		port = a.httpSvc.Port()
	}
	if port != 0 && port != 443 && port != 80 {
		return fmt.Sprintf("%s:%d", base, port)
	}
	return base
}

// copyProxyResponseHeaders copies response headers from the upstream response
// to the echo response, normalizing Content-Type to match what the indexd SDK
// expects. The SDK does an exact string match on Content-Type against the
// Accept header (e.g. "application/json"), but Go's http handlers append
// "; charset=utf-8" by default, causing the match to fail.
func copyProxyResponseHeaders(dst http.ResponseWriter, src *http.Response) {
	for key, values := range src.Header {
		for _, value := range values {
			dst.Header().Add(key, value)
		}
	}
	if ct := dst.Header().Get("Content-Type"); ct != "" {
		if before, _, found := strings.Cut(ct, ";"); found && strings.HasPrefix(before, "application/json") {
			dst.Header().Set("Content-Type", before)
		}
	}
}

// buildAppURL constructs a URL to the indexd app API with the given path,
// preserving signed URL query parameters from the original request.
func (a *API) buildAppURL(originalURL *url.URL, path string) (string, error) {
	base, err := url.Parse(a.resolveProxyURL())
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
	siaMw := SiaSignedURLMiddleware(a.siaSvc, a.resolvePublicHost())

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
				router.WithDescription("Authorization page for the user to approve or reject the connection request. Unauthenticated users are redirected to login."),
				router.WithTags(tagAuth),
				router.WithPathParam("requestID", "Request ID", ""),
				router.WithSuccessResponse(http.StatusOK, "HTML authorization page"),
				router.WithErrorResponses(
					router.DefineSwaggerErrorResponse(http.StatusNotFound, "Unknown request ID"),
				),
			),
		),
		router.NewRoute(http.MethodGet, "/auth/connect/:requestID/status", echo.WrapHandler(a.proxy),
			router.WithSwagger(
				router.WithSummary("Check connection request status"),
				router.WithDescription("Returns approval status and userSecret if approved."),
				router.WithTags(tagAuth),
				router.WithPathParam("requestID", "Request ID", ""),
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

	// Signed-only routes (no portal JWT auth required — indexd validates signed URL)
	signedRoutes := []router.RouteDefinition{
		router.NewRoute(http.MethodGet, "/auth/check", echo.WrapHandler(a.proxy),
			router.WithSwagger(
				router.WithSummary("Check application authentication"),
				router.WithDescription("Verifies the application is authenticated with a valid signed URL."),
				router.WithTags(tagAuth),
				router.WithSuccessResponse(http.StatusNoContent, "Application is authenticated"),
			),
		),
		router.NewRoute(http.MethodPost, "/auth/connect", a.HandlePOSTAuthConnectInit),
	}
	router.RegisterRoutes(r, accessSvc, a.Subdomain(), signedRoutes)

	// App management routes (JWT auth required) — under /api prefix
	appApi, err := r.Group("/api")
	if err != nil {
		return fmt.Errorf("failed to create app API group: %w", err)
	}
	appRoutes := buildAppsRoutes(a)
	router.RegisterRoutes(appApi, accessSvc, a.Subdomain(), appRoutes)

	return nil
}

func buildPinnedSlabRoute(a *API) router.RouteDefinition {
	return router.NewRoute(http.MethodPost, "/slabs", a.pinSlabHandler,
		router.WithAccess(core.ACCESS_USER_ROLE),
		router.WithSwagger(
			router.WithSummary("Pin a slab"),
			router.WithDescription("Pins a slab for storage on the Sia network."),
			router.WithTags(tagSlabs),
			router.WithRequestBody(slabs.SlabPinParams{}, "Slab pinning parameters", true),
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
			router.WithDescription("Unpins all slabs not referenced by any pinned object, freeing storage quota."),
			router.WithTags(tagSlabs),
			router.WithSuccessResponse(http.StatusOK, "Slabs pruned successfully"),
		),
	)
}

func buildDeleteSlabRoute(a *API) router.RouteDefinition {
	return router.NewRoute(http.MethodDelete, "/slabs/:id", a.unpinSlabHandler,
		router.WithAccess(core.ACCESS_USER_ROLE),
		router.WithSwagger(
			router.WithSummary("Unpin a slab"),
			router.WithDescription("Unpins a slab. If no objects reference it, it will be removed."),
			router.WithTags(tagSlabs),
			router.WithPathParam("id", "Slab ID", ""),
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
			router.WithDescription("Creates or updates an object referencing already-pinned slabs."),
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
			router.WithDescription("Deletes an object. Underlying slabs are not deleted and may be shared with other objects."),
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
				router.WithSummary("Approve or reject a connection request"),
				router.WithDescription("Approves or rejects an application connection request."),
				router.WithTags(tagAuth),
				router.WithPathParam("requestID", "Request ID", ""),
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
				router.WithDescription("Registers the application key after approval."),
				router.WithTags(tagAuth),
				router.WithPathParam("requestID", "Request ID", ""),
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
