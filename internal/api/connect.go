package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
	mcontext "go.lumeweb.com/portal-middleware/context"
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

	siaService := a.siaSvc

	account, err := siaService.GetAccount(ctx, userID)
	if err != nil || len(account.ConnectKey) == 0 {
		if err := a.quotaSvc.ProvisionAccount(ctx, userID); err != nil {
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

	result, err := a.proxyToIndexd(c, http.MethodPost, targetURL, body, a.resolvePublicHost(), http.StatusNoContent)
	if err != nil {
		return err
	}

	if result.StatusCode == http.StatusNoContent {
		if reqBody.Approve {
			if err := siaService.StoreAuthRequest(ctx, requestID, userID); err != nil {
				a.Logger().Error("failed to store auth request mapping", zap.Error(err))
			}
		} else {
			if err := siaService.DeleteAuthRequest(ctx, requestID); err != nil {
				a.Logger().Error("failed to delete rejected auth request", zap.Error(err))
			}
		}
	}

	writeProxyResponse(c, result)
	return nil
}
