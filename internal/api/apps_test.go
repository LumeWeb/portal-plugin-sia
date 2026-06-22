package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.sia.tech/core/types"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	siaMocks "go.lumeweb.com/portal-plugin-sia/internal/testing/mocks"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
)

// testPubkeyHex is a valid 32-byte ed25519 public key in hex (64 chars).
const testPubkeyHex = "0000000000000000000000000000000000000000000000000000000000000001"

// testPubkey is the parsed form of testPubkeyHex.
var testPubkey = types.PublicKey{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}

// ---------------------------------------------------------------------------
// GET /apps
// ---------------------------------------------------------------------------

func TestListApps_Success(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		token, userID := helper.SetupAuthenticatedTest()

		mockSiaService := core.GetService[*siaMocks.MockSiaService](ctx, pluginCore.SIA_SERVICE)

		expectedSummary := &pluginCore.AppsSummary{
			AppCount: 2,
			Apps: []pluginCore.AppSummary{
				{
					Name:        "Test App",
					Description: "A test application",
					LogoURL:     "https://example.com/logo.png",
					ServiceURL:  "https://example.com",
					PinnedData:  300_000_000,
					LastUsed:    time.Now().UTC(),
				},
				{
					Name:        "Another App",
					Description: "Another test app",
					LogoURL:     "https://example.com/logo2.png",
					ServiceURL:  "https://example2.com",
					PinnedData:  200_000_000,
					LastUsed:    time.Now().UTC(),
				},
			},
		}

		mockSiaService.EXPECT().GetAppsSummary(mock.Anything, userID).Return(expectedSummary, nil).Once()

		resp := helper.makeAuthenticatedRequest(http.MethodGet, "/apps", token, nil)

		assert.Equal(t, http.StatusOK, resp.Code)

		var body map[string]any
		err := json.Unmarshal(resp.Body.Bytes(), &body)
		assert.NoError(t, err)

		assert.Equal(t, float64(2), body["appCount"])

		apps, ok := body["apps"].([]any)
		assert.True(t, ok)
		assert.Len(t, apps, 2)

		firstApp := apps[0].(map[string]any)
		assert.Equal(t, "Test App", firstApp["name"])
		assert.Equal(t, "A test application", firstApp["description"])
		assert.Equal(t, "https://example.com/logo.png", firstApp["logoURL"])
		assert.Equal(t, "https://example.com", firstApp["serviceURL"])
		assert.Equal(t, float64(300_000_000), firstApp["pinnedData"])
	}, TestOptions)
}

func TestListApps_Unauthorized(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)

		// No token — should get 401
		resp := helper.makeRequest(http.MethodGet, "/apps", nil)

		assert.Equal(t, http.StatusUnauthorized, resp.Code)
	}, TestOptions)
}

func TestListApps_Empty(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		token, userID := helper.SetupAuthenticatedTest()

		mockSiaService := core.GetService[*siaMocks.MockSiaService](ctx, pluginCore.SIA_SERVICE)

		emptySummary := &pluginCore.AppsSummary{
			AppCount: 0,
			Apps:     []pluginCore.AppSummary{},
		}

		mockSiaService.EXPECT().GetAppsSummary(mock.Anything, userID).Return(emptySummary, nil).Once()

		resp := helper.makeAuthenticatedRequest(http.MethodGet, "/apps", token, nil)

		assert.Equal(t, http.StatusOK, resp.Code)

		var body map[string]any
		err := json.Unmarshal(resp.Body.Bytes(), &body)
		assert.NoError(t, err)

		assert.Equal(t, float64(0), body["appCount"])
		apps, ok := body["apps"].([]any)
		assert.True(t, ok)
		assert.Len(t, apps, 0)
	}, TestOptions)
}

// ---------------------------------------------------------------------------
// DELETE /apps/:pubkey
// ---------------------------------------------------------------------------

func TestDeleteApp_Success(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		token, _ := helper.SetupAuthenticatedTest()

		mockSiaService := core.GetService[*siaMocks.MockSiaService](ctx, pluginCore.SIA_SERVICE)
		// DeleteAppAccount is called with (ctx, siaAccountID, pubkey)
		// siaAccountID comes from GetAccount (mocked in SetupSiaServiceMocks)
		mockSiaService.EXPECT().DeleteAppAccount(mock.Anything, mock.AnythingOfType("uint"), testPubkey).Return(nil).Once()

		resp := helper.makeAuthenticatedRequest(http.MethodDelete, "/apps/"+testPubkeyHex, token, nil)

		assert.Equal(t, http.StatusNoContent, resp.Code)
	}, TestOptions)
}

func TestDeleteApp_InvalidPubkey(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		token, _ := helper.SetupAuthenticatedTest()

		// "abc" is not valid hex — should return 400 before hitting the service
		resp := helper.makeAuthenticatedRequest(http.MethodDelete, "/apps/abc", token, nil)

		assert.Equal(t, http.StatusBadRequest, resp.Code)
	}, TestOptions)
}

func TestDeleteApp_NotFound(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		token, _ := helper.SetupAuthenticatedTest()

		mockSiaService := core.GetService[*siaMocks.MockSiaService](ctx, pluginCore.SIA_SERVICE)

		mockSiaService.EXPECT().DeleteAppAccount(mock.Anything, mock.AnythingOfType("uint"), testPubkey).
			Return(assert.AnError).Once()

		resp := helper.makeAuthenticatedRequest(http.MethodDelete, "/apps/"+testPubkeyHex, token, nil)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	}, TestOptions)
}

func TestDeleteApp_Unauthorized(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)

		resp := helper.makeRequest(http.MethodDelete, "/apps/"+testPubkeyHex, nil)

		assert.Equal(t, http.StatusUnauthorized, resp.Code)
	}, TestOptions)
}

// ---------------------------------------------------------------------------
// POST /prune
// ---------------------------------------------------------------------------

func TestPruneAccount_Success(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		token, userID := helper.SetupAuthenticatedTest()

		mockSiaService := core.GetService[*siaMocks.MockSiaService](ctx, pluginCore.SIA_SERVICE)

		mockSiaService.EXPECT().PruneAccount(mock.Anything, userID).Return(nil).Once()

		resp := helper.makeAuthenticatedRequest(http.MethodPost, "/prune", token, nil)

		assert.Equal(t, http.StatusNoContent, resp.Code)
	}, TestOptions)
}

func TestPruneAccount_Unauthorized(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)

		resp := helper.makeRequest(http.MethodPost, "/prune", nil)

		assert.Equal(t, http.StatusUnauthorized, resp.Code)
	}, TestOptions)
}

func TestPruneAccount_InternalError(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		token, userID := helper.SetupAuthenticatedTest()

		mockSiaService := core.GetService[*siaMocks.MockSiaService](ctx, pluginCore.SIA_SERVICE)

		mockSiaService.EXPECT().PruneAccount(mock.Anything, userID).
			Return(assert.AnError).Once()

		resp := helper.makeAuthenticatedRequest(http.MethodPost, "/prune", token, nil)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	}, TestOptions)
}
