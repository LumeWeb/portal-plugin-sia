package api

import (
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	core "go.lumeweb.com/portal/core"
)

func (a *API) proxyHandler(c echo.Context) error {
	ctx, span := core.TraceMethod(c.Request().Context(), "API.proxyHandler")
	defer span.End()
	c.SetRequest(c.Request().WithContext(ctx))

	timer := prometheus.NewTimer(ProxyDuration.WithLabelValues())
	defer timer.ObserveDuration()

	a.proxy.ServeHTTP(c.Response(), c.Request())

	if c.Response().Status >= 400 {
		ProxyTotal.WithLabelValues(LabelStatusError).Inc()
	} else {
		ProxyTotal.WithLabelValues(LabelStatusSuccess).Inc()
	}

	return nil
}
