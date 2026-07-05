package api

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/labstack/echo/v4"
	mcontext "go.lumeweb.com/portal-middleware/context"
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

type quotaErrorPageData struct {
	SubscriptionURL string
}

type layoutData struct {
	AriaLabelledBy  string
	AriaDescribedBy string
	MetaDescription string
	PageData        any
}

//go:embed connect_layout.html
var connectLayoutHTML string

//go:embed connect.html
var connectHTML string

//go:embed connect_quota_error.html
var connectQuotaErrorHTML string

//go:embed connect_system_error.html
var connectSystemErrorHTML string

var (
	connectTemplate     *template.Template
	quotaErrorTemplate  *template.Template
	systemErrorTemplate *template.Template
)

func init() {
	connectTemplate = template.Must(template.New("connect").
		Parse(connectLayoutHTML))
	template.Must(connectTemplate.New("page").Parse(connectHTML))
	template.Must(connectTemplate.Parse(`{{define "connect"}}{{template "layout" .}}{{end}}`))

	quotaErrorTemplate = template.Must(template.New("quota-error").
		Parse(connectLayoutHTML))
	template.Must(quotaErrorTemplate.New("page").Parse(connectQuotaErrorHTML))
	template.Must(quotaErrorTemplate.Parse(`{{define "quota-error"}}{{template "layout" .}}{{end}}`))

	systemErrorTemplate = template.Must(template.New("system-error").
		Parse(connectLayoutHTML))
	template.Must(systemErrorTemplate.New("page").Parse(connectSystemErrorHTML))
	template.Must(systemErrorTemplate.Parse(`{{define "system-error"}}{{template "layout" .}}{{end}}`))
}

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

	result, err := a.proxyToIndexd(c, http.MethodGet, targetURL, nil, a.resolvePublicHost(), http.StatusOK, "", "")
	if err != nil {
		return err
	}

	if result.StatusCode != http.StatusOK {
		writeProxyResponse(c, result)
		return nil
	}

	respBody := result.Body

	data, err := parseAuthConnectHTML(string(respBody))
	if err != nil {
		a.Logger().Error("failed to parse auth connect HTML", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	data.RequestID = requestID

	userID, err := mcontext.GetUserID(c)
	if err != nil {
		dashboardURL := a.appendPort(a.httpSvc.APISubdomain("dashboard", true))

		dest, err := url.Parse(dashboardURL)
		if err != nil {
			a.Logger().Error("failed to parse dashboard URL", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
		dest.RawQuery = url.Values{"to": {a.resolvePublicURL() + c.Request().URL.String()}}.Encode()

		return c.Redirect(http.StatusFound, dest.String())
	}

	quotaResult, err := a.quotaSvc.ConnectQuotaCheck(ctx, userID)
	if err != nil {
		a.Logger().Error("failed to check connect quota", zap.Uint("userID", userID), zap.Error(err))
	} else if quotaResult != nil {
		if !quotaResult.HasUsableHosts {
			c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
			return systemErrorTemplate.ExecuteTemplate(c.Response().Writer, "system-error", layoutData{})
		}
		if !quotaResult.HasQuota {
			subscriptionURL := a.resolveSubscriptionURL()
			c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
			return quotaErrorTemplate.ExecuteTemplate(c.Response().Writer, "quota-error", layoutData{
				PageData: quotaErrorPageData{
					SubscriptionURL: subscriptionURL,
				},
			})
		}
	}

	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	return connectTemplate.ExecuteTemplate(c.Response().Writer, "connect", layoutData{
		AriaLabelledBy:  "connect-heading",
		AriaDescribedBy: "connect-description",
		MetaDescription: "Approve or reject application connection request",
		PageData:        data,
	})
}

func (a *API) resolveSubscriptionURL() string {
	dashboardURL := a.appendPort(a.httpSvc.APISubdomain("dashboard", true))
	return dashboardURL + "/account/subscription"
}
