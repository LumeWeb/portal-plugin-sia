package api

import (
	"errors"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
)

// Bodyless mutations must consume the limited body before changing state:
// Echo's BodyLimit detects unknown-length excess only when the body is read.
func checkIgnoredBody(c echo.Context) error {
	_, err := io.Copy(io.Discard, c.Request().Body)
	if err != nil {
		return bodyReadError(err, http.StatusBadRequest)
	}
	return nil
}

// Preserve BodyLimit's 413 instead of turning it into a parse or auth error.
func bodyReadError(err error, fallbackStatus int) error {
	if errors.Is(err, echo.ErrStatusRequestEntityTooLarge) {
		return echo.ErrStatusRequestEntityTooLarge
	}
	return echo.NewHTTPError(fallbackStatus, "failed to read request body")
}
