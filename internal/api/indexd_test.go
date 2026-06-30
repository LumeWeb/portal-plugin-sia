package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal-plugin-sia/internal/db"
	siaMocks "go.lumeweb.com/portal-plugin-sia/internal/testing/mocks"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	indexdApp "go.sia.tech/indexd/api/app"
	"go.sia.tech/core/types"
	"go.sia.tech/indexd/slabs"
	"gorm.io/gorm"
)

func TestPinSlab_Success(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		sk, _ := helper.setupSignedAccount()

		params := newTestSlabPinParams(t)
		reqBody := mustMarshalJSON(t, []slabs.SlabPinParams{params})
		resp := helper.makeSignedRequest(http.MethodPost, "/slabs", sk, reqBody)

		assert.Equal(t, http.StatusOK, resp.Code)
	}, TestOptions)
}

func TestPinSlab_ValidationError(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		sk, _ := helper.setupSignedAccount()

		resp := helper.makeSignedRequest(http.MethodPost, "/slabs", sk, []byte("invalid json"))

		assert.Equal(t, http.StatusBadRequest, resp.Code)
	}, TestOptions)
}

func TestPinSlab_AuthError(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)

		params := newTestSlabPinParams(t)
		reqBody := mustMarshalJSON(t, []slabs.SlabPinParams{params})
		resp := helper.makeRequest(http.MethodPost, "/slabs", reqBody)

		assert.Equal(t, http.StatusUnauthorized, resp.Code)
	}, TestOptions)
}

func TestUnpinSlab_Success(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		sk, _ := helper.setupSignedAccount()

		params := newTestSlabPinParams(t)
		slabID := formatSlabID(params)
		resp := helper.makeSignedRequest(http.MethodDelete, fmt.Sprintf("/slabs/%s", slabID), sk, nil)

		assert.Equal(t, http.StatusNoContent, resp.Code)
	}, TestOptions)
}

func TestUnpinSlab_NotFound(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		sk, _ := helper.setupSignedAccount()

		slabID := nonexistentSlabID
		resp := helper.makeSignedRequest(http.MethodDelete, fmt.Sprintf("/slabs/%s", slabID), sk, nil)

		assert.Equal(t, http.StatusNotFound, resp.Code)
	}, TestOptions)
}

func TestUnpinSlab_AuthError(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)

		params := newTestSlabPinParams(t)
		slabID := formatSlabID(params)
		resp := helper.makeRequest(http.MethodDelete, fmt.Sprintf("/slabs/%s", slabID), nil)

		assert.Equal(t, http.StatusUnauthorized, resp.Code)
	}, TestOptions)
}

func TestPinObject_Success(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		sk, _ := helper.setupSignedAccount()

		req := newTestPinObjectRequest(t, sk)
		reqBody := mustMarshalJSON(t, req)
		resp := helper.makeSignedRequest(http.MethodPost, "/objects", sk, reqBody)

		assert.Equal(t, http.StatusNoContent, resp.Code)
	}, TestOptions)
}

func TestPinObject_ValidationError(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		sk, _ := helper.setupSignedAccount()

		resp := helper.makeSignedRequest(http.MethodPost, "/objects", sk, []byte("invalid json"))

		assert.Equal(t, http.StatusBadRequest, resp.Code)
	}, TestOptions)
}

func TestUnpinObject_Success(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		sk, _ := helper.setupSignedAccount()

		objReq := newTestPinObjectRequest(t, sk)
		objKey := formatObjectID(objReq)
		resp := helper.makeSignedRequest(http.MethodDelete, fmt.Sprintf("/objects/%s", objKey), sk, nil)

		assert.Equal(t, http.StatusNoContent, resp.Code)
	}, TestOptions)
}

func TestUnpinObject_NotFound(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		sk, _ := helper.setupSignedAccount()

		objKey := nonexistentObjKey
		resp := helper.makeSignedRequest(http.MethodDelete, fmt.Sprintf("/objects/%s", objKey), sk, nil)

		assert.Equal(t, http.StatusNotFound, resp.Code)
	}, TestOptions)
}

func TestAuthConnect_Success(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		token, _ := helper.SetupAuthenticatedTest()

		reqBody := indexdApp.ApproveAppRequest{Approve: true}
		resp := helper.makeAuthenticatedRequest(http.MethodPost, "/auth/connect/test-request-id", token, mustMarshalJSON(t, reqBody))

		assert.Equal(t, http.StatusNoContent, resp.Code)
	}, TestOptions)
}

func TestAuthConnect_Reject(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		token, _ := helper.SetupAuthenticatedTest()

		mockSiaService := core.GetService[*siaMocks.MockSiaService](ctx, pluginCore.SIA_SERVICE)
		mockSiaService.EXPECT().DeleteAuthRequest(mock.Anything, "test-request-id").Return(nil)

		reqBody := indexdApp.ApproveAppRequest{Approve: false}
		resp := helper.makeAuthenticatedRequest(http.MethodPost, "/auth/connect/test-request-id", token, mustMarshalJSON(t, reqBody))

		assert.Equal(t, http.StatusNoContent, resp.Code)
	}, TestOptions)
}

func TestAuthConnect_InvalidRequest(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		token, _ := helper.SetupAuthenticatedTest()

		resp := helper.makeAuthenticatedRequest(http.MethodPost, "/auth/connect/test-request-id", token, []byte("invalid json"))

		assert.Equal(t, http.StatusBadRequest, resp.Code)
	}, TestOptions)
}

func TestAuthConnectInit_Success(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)

		reqBody := indexdApp.RegisterAppRequest{
			Name: "test-app",
		}
		resp := helper.makeRequest(http.MethodPost, "/auth/connect", mustMarshalJSON(t, reqBody))

		assert.Equal(t, http.StatusOK, resp.Code)

		var result indexdApp.RegisterAppResponse
		assert.NoError(t, json.Unmarshal(resp.Body.Bytes(), &result))
		assert.NotEmpty(t, result.ResponseURL)
		assert.NotEmpty(t, result.StatusURL)
		assert.NotEmpty(t, result.RegisterURL)
	}, TestOptions)
}

func TestAuthConnectUI_UnauthenticatedRedirect(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)

		resp := helper.makeRequest(http.MethodGet, "/auth/connect/test-request-id", nil)

		assert.Equal(t, http.StatusFound, resp.Code)
		loc := resp.Header().Get("Location")
		assert.Contains(t, loc, "to=")
	}, TestOptions)
}

func TestAuthConnectUI_AuthenticatedRendersPage(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		token, _ := helper.SetupAuthenticatedTest()

		resp := helper.makeAuthenticatedRequest(http.MethodGet, "/auth/connect/test-request-id", token, nil)

		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Contains(t, resp.Header().Get("Content-Type"), "text/html")
		body := resp.Body.String()
		assert.Contains(t, body, `src="https://example.com/logo.png"`)
		assert.Contains(t, body, "Test App")
		assert.Contains(t, body, "A test application")
		assert.Contains(t, body, `var callbackURL = "https:\/\/example.com\/callback"`)
	}, TestOptions)
}

func TestParseAuthConnectHTML(t *testing.T) {
	html := fakeIndexdAuthConnectHTML("My App", "My Description", "https://example.com/logo.png", "https://callback.example.com")

	data, err := parseAuthConnectHTML(html)
	assert.NoError(t, err)
	assert.Equal(t, "https://example.com/logo.png", data.AppLogoURL)
	assert.Equal(t, "My App", data.AppName)
	assert.Equal(t, "My Description", data.AppDescription)
	assert.Equal(t, "https://callback.example.com", data.CallbackURL)
}

func TestAuthConnectStatus_Success(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)

		resp := helper.makeRequest(http.MethodGet, "/auth/connect/test-request-id/status", nil)

		assert.Equal(t, http.StatusOK, resp.Code)

		var result indexdApp.AuthConnectStatusResponse
		assert.NoError(t, json.Unmarshal(resp.Body.Bytes(), &result))
	}, TestOptions)
}

func TestAuthConnectRegister_Success(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		mockSiaService := core.GetService[*siaMocks.MockSiaService](ctx, pluginCore.SIA_SERVICE)

		authReq := &db.AuthRequest{
			RequestID: "test-request-id",
			UserID:    TestUserID,
		}
		mockSiaService.EXPECT().GetAuthRequest(mock.Anything, "test-request-id").Return(authReq, nil)
		siaAccount := &db.SiaAccount{Model: gorm.Model{ID: 2}, UserID: TestUserID}
		mockSiaService.EXPECT().GetAccount(mock.Anything, TestUserID).Return(siaAccount, nil)
		mockSiaService.EXPECT().RegisterAppAccount(mock.Anything, uint(2), mock.AnythingOfType("types.PublicKey")).Return(&db.SiaAppAccount{}, nil)
		mockSiaService.EXPECT().DeleteAuthRequest(mock.Anything, "test-request-id").Return(nil)

		sk := types.GeneratePrivateKey()
		pk := sk.PublicKey()
		reqBody := indexdApp.RegisterAppKeyRequest{
			AppKey:    pk,
			Signature: sk.SignHash(types.Hash256{}),
		}
		resp := helper.makeRequest(http.MethodPost, "/auth/connect/test-request-id/register", mustMarshalJSON(t, reqBody))

		assert.Equal(t, http.StatusNoContent, resp.Code)
	}, TestOptions)
}
