package api

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/labstack/echo/v4"
	mcontext "go.lumeweb.com/portal-middleware/context"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
	"golang.org/x/net/html"

	_ "embed"
)

type connectPageData struct {
	AppName        string
	AppDescription string
	AppLogoURL     string
	CallbackURL    string
	RequestID      string
}

//go:embed connect.html
var connectHTML string

var connectTemplate = template.Must(template.New("connect").Parse(connectHTML))

func parseAuthConnectHTML(htmlBody string) (connectPageData, error) {
	var data connectPageData

	doc, err := html.Parse(strings.NewReader(htmlBody))
	if err != nil {
		return data, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Data == "div" && hasClass(n, "logo") {
				img := findFirst(n, "img")
				if img != nil {
					data.AppLogoURL = getAttr(img, "src")
				}
			}

			// .header > div > p.subtitle = app name (distinct from .copy > p.subtitle = description)
			if n.Data == "div" && hasClass(n, "header") {
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.ElementNode && c.Data == "div" {
						p := findFirstByClass(c, "p", "subtitle")
						if p != nil {
							data.AppName = textContent(p)
						}
					}
				}
			}

			if n.Data == "div" && hasClass(n, "copy") {
				p := findFirstByClass(n, "p", "subtitle")
				if p != nil {
					data.AppDescription = textContent(p)
				}
			}

			if n.Data == "script" {
				text := textContent(n)
				data.CallbackURL = extractCallbackURL(text)
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(doc)
	return data, nil
}

func hasClass(n *html.Node, class string) bool {
	for _, attr := range n.Attr {
		if attr.Key == "class" {
			return slices.Contains(strings.Fields(attr.Val), class)
		}
	}
	return false
}

func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func findFirst(parent *html.Node, tag string) *html.Node {
	for c := parent.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			return c
		}
	}
	return nil
}

func findFirstByClass(parent *html.Node, tag, class string) *html.Node {
	for c := parent.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag && hasClass(c, class) {
			return c
		}
	}
	return nil
}

func textContent(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

func extractCallbackURL(scriptContent string) string {
	const prefix = `const callbackUrl = "`
	const suffix = `";`

	_, after, found := strings.Cut(scriptContent, prefix)
	if !found {
		return ""
	}

	result, _, found := strings.Cut(after, suffix)
	if !found {
		return ""
	}

	return result
}

func (a *API) HandleGETAuthConnect(c echo.Context) error {
	requestID := c.Param("requestID")
	if requestID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing request ID")
	}

	ctx := c.Request().Context()

	targetURL, err := a.buildAppURL(c.Request().URL, fmt.Sprintf("/auth/connect/%s", requestID))
	if err != nil {
		a.Logger().Error("failed to build proxy URL", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	proxyReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		a.Logger().Error("failed to create proxy request", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	proxyReq.Header.Set("User-Agent", c.Request().Header.Get("User-Agent"))
	proxyReq.Host = a.resolvePublicHost()

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

	if resp.StatusCode != http.StatusOK {
		copyProxyResponseHeaders(c.Response().Writer, resp)
		c.Response().Status = resp.StatusCode
		c.Response().Write(respBody)
		return nil
	}

	data, err := parseAuthConnectHTML(string(respBody))
	if err != nil {
		a.Logger().Error("failed to parse auth connect HTML", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	data.RequestID = requestID

	_, err = mcontext.GetUserID(c)
	if err != nil {
		httpSvc := core.GetService[core.HTTPService](a.Context(), core.HTTP_SERVICE)
		dashboardURL := a.appendPort(httpSvc.APISubdomain("dashboard", true))

		dest, err := url.Parse(dashboardURL)
		if err != nil {
			a.Logger().Error("failed to parse dashboard URL", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
		dest.RawQuery = url.Values{"return": {c.Request().URL.String()}}.Encode()

		return c.Redirect(http.StatusFound, dest.String())
	}

	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	return connectTemplate.Execute(c.Response().Writer, data)
}
