package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
	mcontext "go.lumeweb.com/portal-middleware/context"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

// approvalRequest represents the body of the connect approval request.
type approvalRequest struct {
	Approve bool `json:"approve"`
}

// HandlePOSTAuthConnect handles the approval/rejection of connect requests.
// It requires JWT authentication to determine the user, gets the connect key from the database,
// and proxies the request to indexd with Basic Auth injected.
//
// @Param requestID path string required "Request ID"
func (a *API) HandlePOSTAuthConnect(c echo.Context) error {
	requestID := c.Param("requestID")
	if requestID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing request ID")
	}

	// Get userID from JWT token
	userID, err := mcontext.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "login required")
	}

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		a.Logger().Error("failed to read request body", zap.Error(err))
		return echo.NewHTTPError(http.StatusBadRequest, "failed to read request body")
	}
	defer c.Request().Body.Close()

	var reqBody approvalRequest
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	ctx := c.Request().Context()

	// Get SiaService
	siaService := core.GetService[pluginCore.SiaService](a.Context(), pluginCore.SIA_SERVICE)

	// Use GetAccount method with userID from JWT
	account, err := siaService.GetAccount(ctx, userID)
	if err != nil {
		a.Logger().Error("failed to find SiaAccount", zap.Uint("userID", userID), zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "account not found")
	}

	targetURL, err := a.buildAppURL(c.Request().URL, fmt.Sprintf("/auth/connect/%s", requestID))
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

	if len(account.ConnectKey) == 0 {
		return echo.NewHTTPError(http.StatusInternalServerError, "connect key not available")
	}
	proxyReq.SetBasicAuth("", account.ConnectKey)

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

	// Store requestID→userID mapping if approval was successful
	// This allows the registration step to link the app to this user
	if resp.StatusCode == http.StatusNoContent && reqBody.Approve {
		if err := siaService.StoreAuthRequest(ctx, requestID, userID); err != nil {
			a.Logger().Error("failed to store auth request mapping", zap.Error(err))
		}
	} else if resp.StatusCode == http.StatusOK && !reqBody.Approve {
		if err := siaService.DeleteAuthRequest(ctx, requestID); err != nil {
			a.Logger().Error("failed to delete rejected auth request", zap.Error(err))
		}
	}

	copyProxyResponseHeaders(c.Response().Writer, resp)
	c.Response().Status = resp.StatusCode
	c.Response().Write(respBody)

	return nil
}
