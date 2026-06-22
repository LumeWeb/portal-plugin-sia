package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/queryutil"
	"go.sia.tech/core/types"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	siaMocks "go.lumeweb.com/portal-plugin-sia/internal/testing/mocks"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"gorm.io/gorm"
)

// testPubkeyHex is a valid 32-byte ed25519 public key in hex (64 chars).
const testPubkeyHex = "0000000000000000000000000000000000000000000000000000000000000001"

// testPubkey is the parsed form of testPubkeyHex.
var testPubkey = types.PublicKey{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}

// ---------------------------------------------------------------------------
// GET /apps (queryutil list)
// ---------------------------------------------------------------------------

func TestListApps_Success(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		token, userID := helper.SetupAuthenticatedTest()

		mockSiaService := core.GetService[*siaMocks.MockSiaService](ctx, pluginCore.SIA_SERVICE)

		expectedApps := []pluginCore.AppAccount{
			{
				PublicKey:   testPubkey,
				Name:        "Test App",
				Description: "A test application",
				LogoURL:     "https://example.com/logo.png",
				ServiceURL:  "https://example.com",
				PinnedData:  300_000_000,
				LastUsed:    time.Now().UTC(),
			},
			{
				PublicKey:   types.PublicKey{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
				Name:        "Another App",
				Description: "Another test app",
				LogoURL:     "https://example.com/logo2.png",
				ServiceURL:  "https://example2.com",
				PinnedData:  200_000_000,
				LastUsed:    time.Now().UTC(),
			},
		}

		mockSiaService.EXPECT().ListApps(
			mock.Anything, userID, mock.Anything, mock.Anything, mock.Anything,
		).Return(expectedApps, int64(2), nil).Once()

		resp := helper.makeAuthenticatedRequest(http.MethodGet, "/apps", token, nil)

		assert.Equal(t, http.StatusOK, resp.Code)

		var body map[string]any
		err := json.Unmarshal(resp.Body.Bytes(), &body)
		assert.NoError(t, err)

		assert.Equal(t, float64(2), body["total"])

		data, ok := body["data"].([]any)
		assert.True(t, ok)
		assert.Len(t, data, 2)

		firstApp := data[0].(map[string]any)
		assert.Equal(t, testPubkeyHex, firstApp["publicKey"])
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

		mockSiaService.EXPECT().ListApps(
			mock.Anything, userID, mock.Anything, mock.Anything, mock.Anything,
		).Return([]pluginCore.AppAccount{}, int64(0), nil).Once()

		resp := helper.makeAuthenticatedRequest(http.MethodGet, "/apps", token, nil)

		assert.Equal(t, http.StatusOK, resp.Code)

		var body map[string]any
		err := json.Unmarshal(resp.Body.Bytes(), &body)
		assert.NoError(t, err)

		assert.Equal(t, float64(0), body["total"])
		data, ok := body["data"].([]any)
		assert.True(t, ok)
		assert.Len(t, data, 0)
	}, TestOptions)
}

func TestListApps_WithFilters(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		token, userID := helper.SetupAuthenticatedTest()

		mockSiaService := core.GetService[*siaMocks.MockSiaService](ctx, pluginCore.SIA_SERVICE)

		expectedApps := []pluginCore.AppAccount{
			{
				PublicKey:   testPubkey,
				Name:        "Filtered App",
				Description: "Matches filter",
				ServiceURL:  "https://example.com",
				PinnedData:  100_000_000,
				LastUsed:    time.Now().UTC(),
			},
		}

		mockSiaService.EXPECT().ListApps(
			mock.Anything, userID, mock.Anything, mock.Anything, mock.Anything,
		).Return(expectedApps, int64(1), nil).Once()

		// Pass queryutil filter params
		resp := helper.makeAuthenticatedRequest(http.MethodGet, "/apps?filters=%5B%7B%22field%22%3A%22name%22%2C%22operator%22%3A%22eq%22%2C%22value%22%3A%22Filtered%20App%22%7D%5D", token, nil)

		assert.Equal(t, http.StatusOK, resp.Code)

		var body map[string]any
		err := json.Unmarshal(resp.Body.Bytes(), &body)
		assert.NoError(t, err)

		assert.Equal(t, float64(1), body["total"])
		data, ok := body["data"].([]any)
		assert.True(t, ok)
		assert.Len(t, data, 1)
	}, TestOptions)
}

func TestListApps_InternalError(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		token, userID := helper.SetupAuthenticatedTest()

		mockSiaService := core.GetService[*siaMocks.MockSiaService](ctx, pluginCore.SIA_SERVICE)

		mockSiaService.EXPECT().ListApps(
			mock.Anything, userID, mock.Anything, mock.Anything, mock.Anything,
		).Return(nil, int64(0), assert.AnError).Once()

		resp := helper.makeAuthenticatedRequest(http.MethodGet, "/apps", token, nil)

		assert.Equal(t, http.StatusInternalServerError, resp.Code)
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
			Return(gorm.ErrRecordNotFound).Once()

		resp := helper.makeAuthenticatedRequest(http.MethodDelete, "/apps/"+testPubkeyHex, token, nil)

		assert.Equal(t, http.StatusNotFound, resp.Code)
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

// Suppress unused import warning for queryutil (used in mock matching)
var _ = queryutil.Pagination{}
