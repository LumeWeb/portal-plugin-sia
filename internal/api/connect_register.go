package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
	indexdApp "go.sia.tech/indexd/api/app"
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

	targetURL, err := a.buildAppURL(c.Request().URL, fmt.Sprintf("/auth/connect/%s/register", requestID))
	if err != nil {
		a.Logger().Error("failed to build proxy URL", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	result, err := a.proxyToIndexd(c, http.MethodPost, targetURL, body, a.resolvePublicHost(), http.StatusNoContent)
	if err != nil {
		return err
	}

	// On success from indexd, create the app account record
	if result.StatusCode == http.StatusNoContent {
		var reqBody indexdApp.RegisterAppKeyRequest
		if err := json.Unmarshal(body, &reqBody); err == nil {
			siaService := a.siaSvc

			authReq, err := siaService.GetAuthRequest(ctx, requestID)
			if err != nil {
				a.Logger().Error("failed to get auth request for app account registration",
					zap.String("requestID", requestID), zap.Error(err))
			} else {
				siaAccount, err := siaService.GetAccount(ctx, authReq.UserID)
				if err != nil {
					a.Logger().Error("failed to get sia account for app account registration",
						zap.Uint("userID", authReq.UserID), zap.Error(err))
				} else if _, err := siaService.RegisterAppAccount(ctx, siaAccount.ID, reqBody.AppKey); err != nil {
					a.Logger().Error("failed to register app account",
						zap.Uint("siaAccountID", siaAccount.ID),
						zap.Error(err))
				}
			}

			if err := siaService.DeleteAuthRequest(ctx, requestID); err != nil {
				a.Logger().Warn("failed to delete auth request", zap.String("requestID", requestID), zap.Error(err))
			}
		}
	}

	writeProxyResponse(c, result)
	return nil
}
