package api

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	siaMocks "go.lumeweb.com/portal-plugin-sia/internal/testing/mocks"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
)

func TestConnectRequestBodyLimit(t *testing.T) {
	const limit = 10 << 20
	for _, path := range []string{"/auth/connect", "/auth/connect/test-request-id/register"} {
		for _, length := range []int64{-1, 1, limit + 1} {
			t.Run(path+" length="+strconv.FormatInt(length, 10), func(t *testing.T) {
				coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
					req := ctx.NewAPIRequest(http.MethodPost, path, nil)
					req.Body = io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{'x'}, limit+1)))
					req.ContentLength = length
					rec := httptest.NewRecorder()
					ctx.Router().ServeHTTP(rec, req)
					require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
				}, TestOptions)
			})
		}
	}
}

func TestConnectAcceptsRequestBodyAtLimit(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		newMockHelper(t, ctx)
		payload := append(bytes.Repeat([]byte{' '}, (10<<20)-2), '{', '}')
		req := ctx.NewAPIRequest(http.MethodPost, "/auth/connect", payload)
		req.ContentLength = -1
		rec := httptest.NewRecorder()
		ctx.Router().ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}, TestOptions)
}

func TestPruneRejectsOversizedUnknownLengthBodyBeforeMutation(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		token, _ := helper.SetupAuthenticatedTest()
		req := ctx.NewAPIRequest(http.MethodPost, "/api/prune", nil)
		req.Body = io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{'x'}, (10<<20)+1)))
		req.ContentLength = -1
		setAuthHeader(req, token)
		rec := httptest.NewRecorder()
		ctx.Router().ServeHTTP(rec, req)
		require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
		svc := core.GetService[*siaMocks.MockSiaService](ctx, pluginCore.SIA_SERVICE)
		svc.AssertNotCalled(t, "PruneAccount", mock.Anything, mock.Anything)
	}, TestOptions)
}

func TestDeleteAppRejectsOversizedUnknownLengthBodyBeforeLookup(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		token, _ := helper.SetupAuthenticatedTest()
		req := ctx.NewAPIRequest(http.MethodDelete, "/api/apps/"+strings.Repeat("0", 64), nil)
		req.Body = io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{'x'}, (10<<20)+1)))
		req.ContentLength = -1
		setAuthHeader(req, token)
		rec := httptest.NewRecorder()
		ctx.Router().ServeHTTP(rec, req)
		require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
		svc := core.GetService[*siaMocks.MockSiaService](ctx, pluginCore.SIA_SERVICE)
		svc.AssertNotCalled(t, "GetAccount", mock.Anything, mock.Anything)
	}, TestOptions)
}

func TestSignedBodyIsLimitedBeforeAccountLookup(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		sk, _ := helper.setupSignedAccount()
		resp := helper.makeSignedRequest(http.MethodPost, "/objects", sk, bytes.Repeat([]byte{'x'}, (10<<20)+1))
		require.Equal(t, http.StatusRequestEntityTooLarge, resp.Code)
		svc := core.GetService[*siaMocks.MockSiaService](ctx, pluginCore.SIA_SERVICE)
		svc.AssertNotCalled(t, "GetAppAccountByKey", mock.Anything, mock.Anything)
	}, TestOptions)
}
