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

type approvalRequest struct {
	Approve bool `json:"approve"`
}

func (a *API) HandlePOSTAuthConnect(c echo.Context) error {
	requestID := c.Param("requestID")
	if requestID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing request ID")
	}

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

	siaService := core.GetService[pluginCore.SiaService](a.Context(), pluginCore.SIA_SERVICE)

	account, err := siaService.GetAccount(ctx, userID)
	if err != nil || len(account.ConnectKey) == 0 {
		quotaSvc := core.GetService[pluginCore.QuotaService](a.Context(), pluginCore.QUOTA_SERVICE)
		if err := quotaSvc.ProvisionAccount(ctx, userID); err != nil {
			a.Logger().Error("failed to provision account", zap.Uint("userID", userID), zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to provision account")
		}
		account, err = siaService.GetAccount(ctx, userID)
		if err != nil {
			a.Logger().Error("failed to get account after provisioning", zap.Uint("userID", userID), zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "account not found")
		}
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
	proxyReq.Host = a.resolvePublicHost()
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
