package api

import (
	"bytes"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// indexdProxyResult holds the response from an indexd proxy request.
type indexdProxyResult struct {
	StatusCode int
	Body       []byte
	Resp       *http.Response
}

// proxyToIndexd sends a request to indexd and returns the response. It handles
// transport errors and logs non-success responses at debug level with method,
// path, host, status code, and response body for tracing.
//
// successCode is the HTTP status code the caller considers successful. Any other
// status is logged as a rejection and returned for the caller to forward.
//
// If basicAuthUser and basicAuthPass are non-empty, Basic Auth is set on the
// proxy request. This is used by the approve/reject endpoint to pass the user's
// connect key to indexd.
//
// On transport error, an echo.HTTPError is returned and the caller should
// propagate it directly.
func (a *API) proxyToIndexd(ctx echo.Context, method, targetURL string, body []byte, host string, successCode int, basicAuthUser, basicAuthPass string) (*indexdProxyResult, error) {
	proxyReq, err := http.NewRequestWithContext(
		ctx.Request().Context(),
		method,
		targetURL,
		bytes.NewReader(body),
	)
	if err != nil {
		a.Logger().Error("failed to create proxy request", zap.Error(err))
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	if body != nil {
		proxyReq.Header.Set("Content-Type", ctx.Request().Header.Get("Content-Type"))
	}
	proxyReq.Header.Set("User-Agent", ctx.Request().Header.Get("User-Agent"))
	proxyReq.Host = host

	if basicAuthUser != "" || basicAuthPass != "" {
		proxyReq.SetBasicAuth(basicAuthUser, basicAuthPass)
	}

	resp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		a.Logger().Error("failed to proxy request to indexd", zap.Error(err))
		return nil, echo.NewHTTPError(http.StatusBadGateway, "upstream error")
	}

	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		a.Logger().Error("failed to read upstream response", zap.Error(err))
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "failed to read response")
	}

	if resp.StatusCode != successCode {
		a.Logger().Debug("indexd rejected request",
			zap.String("method", method),
			zap.String("host", host),
			zap.String("path", proxyReq.URL.Path),
			zap.Int("statusCode", resp.StatusCode),
			zap.Int("successCode", successCode),
			zap.String("responseBody", string(respBody)),
		)
	}

	return &indexdProxyResult{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Resp:       resp,
	}, nil
}

// writeProxyResponse writes the indexd response back to the client, copying headers.
func writeProxyResponse(c echo.Context, result *indexdProxyResult) {
	copyProxyResponseHeaders(c.Response().Writer, result.Resp)
	c.Response().Status = result.StatusCode
	c.Response().Write(result.Body)
}
