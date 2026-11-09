package api

import (
	"errors"

	"github.com/labstack/echo/v4"
)

// Preserve BodyLimit's 413 instead of turning it into a parse or auth error.
func bodyReadError(err error, fallbackStatus int) error {
	if errors.Is(err, echo.ErrStatusRequestEntityTooLarge) {
		return echo.ErrStatusRequestEntityTooLarge
	}
	return echo.NewHTTPError(fallbackStatus, "failed to read request body")
}
