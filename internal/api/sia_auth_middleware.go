package api

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	mcontext "go.lumeweb.com/portal-middleware/context"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	core "go.lumeweb.com/portal/core"
	"go.sia.tech/core/types"
)

const (
	queryParamCredential = "sc"
	queryParamSignature  = "ss"
	queryParamValidUntil = "sv"

	siaAppAccountIDKey = "siaAppAccountID"
)

// SiaSignedURLMiddleware validates Sia ed25519 signed URL authentication.
// It verifies the sc (public key), ss (signature), sv (validUntil) query
// parameters against the provided hostname, looks up the associated Sia
// account, and sets the Portal userID in the Echo context so downstream
// handlers can use mcontext.GetUserID unchanged.
func SiaSignedURLMiddleware(ctx core.Context, hostname string) echo.MiddlewareFunc {
	siaService := core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()

			if !isSignedRequest(req) {
				return echo.NewHTTPError(http.StatusUnauthorized,
					fmt.Sprintf("missing required query parameters: %q, %q, %q",
						queryParamCredential, queryParamSignature, queryParamValidUntil))
			}

			pk, err := parseCredential(req)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized,
					fmt.Sprintf("invalid %q parameter: %v", queryParamCredential, err))
			}

			sig, err := parseSignature(req)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized,
					fmt.Sprintf("invalid %q parameter: %v", queryParamSignature, err))
			}

			validUntil, err := parseValidUntil(req)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized,
					fmt.Sprintf("invalid %q parameter: %v", queryParamValidUntil, err))
			}

			if validUntil.Before(time.Now().UTC()) {
				return echo.NewHTTPError(http.StatusUnauthorized, "signature expired")
			}

			body, err := io.ReadAll(req.Body)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "failed to read request body")
			}
			req.Body = io.NopCloser(bodyReader(body))

			hash := requestHash(req.Method, hostname, req.URL.Path, validUntil, body)
			if !pk.VerifyHash(hash, sig) {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid signature")
			}

			accountKeyStr := base64.URLEncoding.EncodeToString(pk[:])
			appAccount, err := siaService.GetAppAccountByKey(req.Context(), accountKeyStr)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "unknown account")
			}

			siaAccount, err := siaService.GetAccount(req.Context(), appAccount.SiaAccountID)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "unknown sia account")
			}

			c.Set(string(mcontext.UserIDKey), siaAccount.UserID)
			c.Set(siaAppAccountIDKey, appAccount.ID)
			return next(c)
		}
	}
}

func isSignedRequest(req *http.Request) bool {
	q := req.URL.Query()
	return q.Has(queryParamCredential) &&
		q.Has(queryParamSignature) &&
		q.Has(queryParamValidUntil)
}

func parseCredential(req *http.Request) (types.PublicKey, error) {
	var pk types.PublicKey
	n, err := base64.URLEncoding.Decode(pk[:], []byte(req.URL.Query().Get(queryParamCredential)))
	if err != nil {
		return types.PublicKey{}, fmt.Errorf("invalid base64 encoding for credential")
	} else if n != len(pk) {
		return types.PublicKey{}, fmt.Errorf("invalid credential length: expected %d bytes", len(pk))
	}
	return pk, nil
}

func parseSignature(req *http.Request) (types.Signature, error) {
	var sig types.Signature
	n, err := base64.URLEncoding.Decode(sig[:], []byte(req.URL.Query().Get(queryParamSignature)))
	if err != nil {
		return types.Signature{}, fmt.Errorf("invalid base64 encoding for signature")
	} else if n != len(sig) {
		return types.Signature{}, fmt.Errorf("invalid signature length: expected %d bytes", len(sig))
	}
	return sig, nil
}

func parseValidUntil(req *http.Request) (time.Time, error) {
	ts, err := parseUint64QueryParam(req, queryParamValidUntil)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timestamp: %v", err)
	}
	return time.Unix(int64(ts), 0), nil
}

func parseUint64QueryParam(req *http.Request, key string) (uint64, error) {
	var v uint64
	_, err := fmt.Sscanf(req.URL.Query().Get(key), "%d", &v)
	return v, err
}

func requestHash(method, hostname, path string, validUntil time.Time, body []byte) types.Hash256 {
	h := types.NewHasher()
	h.E.Write([]byte(method))
	h.E.Write([]byte(hostname))
	h.E.Write([]byte(path))
	h.E.WriteUint64(uint64(validUntil.Unix()))
	if body != nil {
		h.E.Write(body)
	}
	return h.Sum()
}

type bytesReadCloser struct {
	*bytes.Reader
}

func bodyReader(b []byte) *bytesReadCloser {
	return &bytesReadCloser{bytes.NewReader(b)}
}

func (b *bytesReadCloser) Close() error { return nil }

func getSiaAppAccountID(c echo.Context) (uint, bool) {
	id, ok := c.Get(siaAppAccountIDKey).(uint)
	return id, ok
}
