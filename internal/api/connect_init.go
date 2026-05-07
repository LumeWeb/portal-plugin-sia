package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"

	"github.com/labstack/echo/v4"
	indexdApp "go.sia.tech/indexd/api/app"

	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

// HandlePOSTAuthConnectInit handles the initial connect request.
// It requires only Sia signed URL auth (no JWT required).
// It intercepts the response from indexd to rewrite URLs to point to the portal.
func (a *API) HandlePOSTAuthConnectInit(c echo.Context) error {
	ctx := c.Request().Context()

	// Read request body
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		a.Logger().Error("failed to read request body", zap.Error(err))
		return echo.NewHTTPError(http.StatusBadRequest, "failed to read request body")
	}
	defer c.Request().Body.Close()

	// Build target URL for indexd (internal, no /api prefix), preserving signed URL query params
	targetURL, err := a.buildAppURL(c.Request().URL, "/auth/connect")
	if err != nil {
		a.Logger().Error("failed to build proxy URL", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	// Create proxy request
	proxyReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		targetURL,
		bytes.NewReader(body),
	)
	if err != nil {
		a.Logger().Error("failed to create proxy request", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	// Copy headers
	proxyReq.Header.Set("Content-Type", c.Request().Header.Get("Content-Type"))
	proxyReq.Header.Set("User-Agent", c.Request().Header.Get("User-Agent"))

	// Execute request
	client := &http.Client{}
	resp, err := client.Do(proxyReq)
	if err != nil {
		a.Logger().Error("failed to proxy request to indexd", zap.Error(err))
		return echo.NewHTTPError(http.StatusBadGateway, "upstream error")
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		a.Logger().Error("failed to read upstream response", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read response")
	}

	// If not OK, return as-is
	if resp.StatusCode != http.StatusOK {
		for key, values := range resp.Header {
			for _, value := range values {
				c.Response().Writer.Header().Add(key, value)
			}
		}
		c.Response().Status = resp.StatusCode
		c.Response().Write(respBody)
		return nil
	}

	// Parse the response
	var registerResp indexdApp.RegisterAppResponse
	if err := json.Unmarshal(respBody, &registerResp); err != nil {
		a.Logger().Error("failed to unmarshal RegisterAppResponse", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "invalid upstream response")
	}

	// Extract requestID from ResponseURL (e.g., "http://indexd/auth/connect/abc123")
	requestID := path.Base(registerResp.ResponseURL)
	if requestID == "" || requestID == "/" {
		a.Logger().Error("failed to extract requestID from ResponseURL", zap.String("responseURL", registerResp.ResponseURL))
		return echo.NewHTTPError(http.StatusInternalServerError, "invalid upstream response")
	}

	// Rewrite URLs to point to portal
	// Original: <indexd.AdvertiseURL>/auth/connect/<requestID>
	// Target:   <portal.URL>/auth/connect/<requestID>
	httpSvc := core.GetService[core.HTTPService](a.Context(), core.HTTP_SERVICE)
	portalBaseURL := httpSvc.APISubdomain(a.ID(), true)

	registerResp.ResponseURL = fmt.Sprintf("%s/auth/connect/%s", portalBaseURL, requestID)
	registerResp.StatusURL = fmt.Sprintf("%s/auth/connect/%s/status", portalBaseURL, requestID)
	registerResp.RegisterURL = fmt.Sprintf("%s/auth/connect/%s/register", portalBaseURL, requestID)

	// Return rewritten response
	c.Response().Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Response().Status = resp.StatusCode
	return json.NewEncoder(c.Response()).Encode(registerResp)
}
