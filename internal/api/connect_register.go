package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
	indexdApp "go.sia.tech/indexd/api/app"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

// HandlePOSTAuthConnectRegister handles the application key registration.
// It proxies to indexd first (letting indexd validate the signed URL and
// register the app key), and only on success creates a SiaAppAccount record
// linking the app to the user who approved the connection.
func (a *API) HandlePOSTAuthConnectRegister(c echo.Context) error {
	requestID := c.Param("requestID")
	if requestID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing request ID")
	}

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		a.Logger().Error("failed to read request body", zap.Error(err))
		return echo.NewHTTPError(http.StatusBadRequest, "failed to read request body")
	}
	defer c.Request().Body.Close()

	ctx := c.Request().Context()

	// Build target URL preserving signed URL query params (sc, ss, sv)
	targetURL, err := a.buildAppURL(c.Request().URL, fmt.Sprintf("/auth/connect/%s/register", requestID))
	if err != nil {
		a.Logger().Error("failed to build proxy URL", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

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

	proxyReq.Header.Set("Content-Type", c.Request().Header.Get("Content-Type"))
	proxyReq.Header.Set("User-Agent", c.Request().Header.Get("User-Agent"))

	client := &http.Client{}
	resp, err := client.Do(proxyReq)
	if err != nil {
		a.Logger().Error("failed to proxy request to indexd", zap.Error(err))
		return echo.NewHTTPError(http.StatusBadGateway, "upstream error")
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		a.Logger().Error("failed to read upstream response", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read response")
	}

	// On success from indexd, create the app account record
	if resp.StatusCode == http.StatusOK {
		var reqBody indexdApp.RegisterAppKeyRequest
		if err := json.Unmarshal(body, &reqBody); err == nil {
			siaService := core.GetService[pluginCore.SiaService](a.Context(), pluginCore.SIA_SERVICE)

			authReq, err := siaService.GetAuthRequest(ctx, requestID)
			if err != nil {
				a.Logger().Error("failed to get auth request for app account registration",
					zap.String("requestID", requestID), zap.Error(err))
			} else {
				accountKey := reqBody.AppKey.String()
				if _, err := siaService.RegisterAppAccount(ctx, authReq.UserID, accountKey); err != nil {
					a.Logger().Error("failed to register app account",
						zap.Uint("userID", authReq.UserID),
						zap.String("accountKey", accountKey),
						zap.Error(err))
				}

				if err := siaService.DeleteAuthRequest(ctx, requestID); err != nil {
					a.Logger().Warn("failed to delete auth request", zap.String("requestID", requestID), zap.Error(err))
				}
			}
		}
	}

	for key, values := range resp.Header {
		for _, value := range values {
			c.Response().Writer.Header().Add(key, value)
		}
	}
	c.Response().Status = resp.StatusCode
	c.Response().Write(respBody)

	return nil
}
