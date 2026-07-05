package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"

	"github.com/labstack/echo/v4"
	indexdApp "go.sia.tech/indexd/api/app"
	"go.uber.org/zap"
)

// HandlePOSTAuthConnectInit handles the initial connect request.
// It requires only Sia signed URL auth (no JWT required).
// It intercepts the response from indexd to rewrite URLs to point to the portal.
func (a *API) HandlePOSTAuthConnectInit(c echo.Context) error {
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		a.Logger().Error("failed to read request body", zap.Error(err))
		return echo.NewHTTPError(http.StatusBadRequest, "failed to read request body")
	}
	defer c.Request().Body.Close()

	targetURL, err := a.buildAppURL(c.Request().URL, "/auth/connect")
	if err != nil {
		a.Logger().Error("failed to build proxy URL", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	result, err := a.proxyToIndexd(c, http.MethodPost, targetURL, body, a.resolvePublicHost(), http.StatusOK, "", "")
	if err != nil {
		return err
	}

	if result.StatusCode != http.StatusOK {
		writeProxyResponse(c, result)
		return nil
	}

	var registerResp indexdApp.RegisterAppResponse
	if err := json.Unmarshal(result.Body, &registerResp); err != nil {
		a.Logger().Error("failed to unmarshal RegisterAppResponse", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "invalid upstream response")
	}

	requestID := path.Base(registerResp.ResponseURL)
	if requestID == "" || requestID == "/" {
		a.Logger().Error("failed to extract requestID from ResponseURL", zap.String("responseURL", registerResp.ResponseURL))
		return echo.NewHTTPError(http.StatusInternalServerError, "invalid upstream response")
	}

	// Rewrite URLs to point to portal
	portalBaseURL := a.resolvePublicURL()
	registerResp.ResponseURL = fmt.Sprintf("%s/auth/connect/%s", portalBaseURL, requestID)
	registerResp.StatusURL = fmt.Sprintf("%s/auth/connect/%s/status", portalBaseURL, requestID)
	registerResp.RegisterURL = fmt.Sprintf("%s/auth/connect/%s/register", portalBaseURL, requestID)

	c.Response().Header().Set("Content-Type", "application/json")
	c.Response().Status = result.StatusCode
	return json.NewEncoder(c.Response()).Encode(registerResp)
}
