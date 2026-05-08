package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal-plugin-sia/internal/db"
	siaMocks "go.lumeweb.com/portal-plugin-sia/internal/testing/mocks"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	coreMocks "go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"go.sia.tech/core/types"
	"gorm.io/gorm"
)

const (
	// TestConnectKey is a valid connect key for testing
	TestConnectKey = "test-connect-key-12345"

	// TestAccountKey is a valid account key for testing
	TestAccountKey = "test-account-key-67890"

	// TestQuotaKey is a valid quota key for testing
	TestQuotaKey = "test-quota-key-abcde"

	// TestUserID is the default user ID for testing
	TestUserID uint = 1

	// TestUserEmail is the email address for test users
	TestUserEmail = "test@example.com"

	// TestUserPassword is the password for test users
	TestUserPassword = "example"
)

// mockHelper provides common mock setup functions for tests
type mockHelper struct {
	ctx coreTesting.TestContext
	t   *testing.T
}

func newMockHelper(t *testing.T, ctx coreTesting.TestContext) *mockHelper {
	helper := &mockHelper{
		ctx: ctx,
		t:   t,
	}
	helper.setupHTTPServiceMocks()
	return helper
}

// setupHTTPServiceMocks configures HTTP service mock expectations needed
// by API struct methods (resolvePublicHost, resolvePublicURL, appendPort).
func (m *mockHelper) setupHTTPServiceMocks() {
	mockHTTPSvc := coreTesting.GetMockHTTPService(m.ctx)
	mockHTTPSvc.EXPECT().Port().Return(uint16(443)).Maybe()
	mockHTTPSvc.EXPECT().APISubdomain(mock.AnythingOfType("string"), mock.AnythingOfType("bool")).Return("sia.example.com").Maybe()
}

// createMockSiaAccount creates a standardized SiaAccount mock object
func createMockSiaAccount(userID uint, _, connectKey, quotaKey string) *db.SiaAccount {
	return &db.SiaAccount{
		UserID:     userID,
		ConnectKey: connectKey,
		QuotaKey:   quotaKey,
	}
}

// SetupSiaServiceMocks configures common Sia service mock expectations
func (m *mockHelper) SetupSiaServiceMocks(userID uint) *siaMocks.MockSiaService {
	mockSiaService := core.GetService[*siaMocks.MockSiaService](m.ctx, pluginCore.SIA_SERVICE)

	// Setup AccountExists expectation
	mockSiaService.EXPECT().AccountExists(mock.Anything, userID).Return(true, nil).Maybe()

	// Setup GetAccount expectation
	mockSiaService.EXPECT().GetAccount(mock.Anything, userID).Return(
		createMockSiaAccount(userID, TestAccountKey, TestConnectKey, TestQuotaKey), nil).Maybe()

	// Setup indexd object/slab management expectations
	mockSiaService.EXPECT().DeleteObject(mock.Anything, mock.AnythingOfType("uint"), mock.AnythingOfType("string")).Return(nil).Maybe()
	mockSiaService.EXPECT().DeleteSlab(mock.Anything, mock.AnythingOfType("uint"), mock.AnythingOfType("string")).Return(nil).Maybe()
	mockSiaService.EXPECT().RegisterSlab(mock.Anything, mock.AnythingOfType("uint"), mock.AnythingOfType("string")).Return(nil).Maybe()
	mockSiaService.EXPECT().RegisterObject(mock.Anything, mock.AnythingOfType("uint"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string")).Return(nil).Maybe()
	mockSiaService.EXPECT().PruneSlabs(mock.Anything, mock.AnythingOfType("uint")).Return(nil).Maybe()

	// Setup auth request tracking
	mockSiaService.EXPECT().StoreAuthRequest(mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("uint")).Return(nil).Maybe()

	return mockSiaService
}

// SetupQuotaServiceMocks configures common quota service mock expectations
func (m *mockHelper) SetupQuotaServiceMocks(userID uint) *siaMocks.MockQuotaService {
	mockQuotaService := core.GetService[*siaMocks.MockQuotaService](m.ctx, pluginCore.QUOTA_SERVICE)

	// Setup EnforceFundingTarget expectation
	mockQuotaService.EXPECT().EnforceFundingTarget(mock.Anything, userID).Return(nil).Maybe()

	return mockQuotaService
}

// SetupAllCommonMocks configures all common service mocks for basic test scenarios
func (m *mockHelper) SetupAllCommonMocks(userID uint) {
	m.SetupSiaServiceMocks(userID)
	m.SetupQuotaServiceMocks(userID)
}

// createTestUserAndLogin creates a test user, logs them in, and returns a JWT token
func createTestUserAndLogin(ctx coreTesting.TestContext) (string, uint) {
	mockAuth := core.GetService[*coreTesting.MockAuthService](ctx, core.AUTH_SERVICE)
	token, user, err := mockAuth.CreateAndLoginUser(TestUserEmail, TestUserPassword)
	if err != nil {
		ctx.T().Fatalf("failed to create and login test user: %v", err)
	}

	return token, user.ID
}

// createTestUser creates a test user and returns a JWT token without expecting LoginPassword to be called
// This is for tests that make authenticated requests but don't call the login endpoint
func createTestUser(ctx coreTesting.TestContext) (string, uint) {
	// Generate test token using the jwt helper without expecting LoginPassword call
	jwtHelper := coreTesting.NewJWTHelper(ctx)
	token, err := jwtHelper.CreateLoginToken(TestUserID)
	if err != nil {
		ctx.T().Fatalf("failed to create test token: %v", err)
	}

	return token, TestUserID
}

// SetupAuthenticatedTest creates a test user, logs them in, and sets up common mocks
func (m *mockHelper) SetupAuthenticatedTest() (string, uint) {
	token, userID := createTestUser(m.ctx)

	m.SetupAllCommonMocks(userID)

	return token, userID
}

// makeRequest creates and executes an unauthenticated API request, returning the response
func (m *mockHelper) makeRequest(method, url string, body []byte) *httptest.ResponseRecorder {
	req := m.ctx.NewAPIRequest(method, url, body)
	rec := httptest.NewRecorder()
	m.ctx.Router().ServeHTTP(rec, req)
	return rec
}

// makeAuthenticatedRequest creates and executes an authenticated API request, returning the response
func (m *mockHelper) makeAuthenticatedRequest(method, url string, token string, body []byte) *httptest.ResponseRecorder {
	req := m.ctx.NewAPIRequest(method, url, body)
	setAuthHeader(req, token)
	rec := httptest.NewRecorder()
	m.ctx.Router().ServeHTTP(rec, req)
	return rec
}

// setAuthHeader sets the Authorization header with a Bearer token
func setAuthHeader(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
}

// makeSignedRequest creates and executes a Sia signed URL authenticated request.
// It computes the request hash using the same algorithm as SiaSignedURLMiddleware,
// signs it, and appends sc/ss/sv query parameters to the URL.
func (m *mockHelper) makeSignedRequest(method, rawURL string, sk types.PrivateKey, body []byte) *httptest.ResponseRecorder {
	validUntil := time.Now().UTC().Add(1 * time.Hour)

	parsed, err := url.Parse(rawURL)
	if err != nil {
		m.t.Fatalf("failed to parse URL: %v", err)
	}

	req := m.ctx.NewAPIRequest(method, rawURL, body)
	hostname := req.Host

	hash := requestHash(method, hostname, parsed.Path, validUntil, body)
	sig := sk.SignHash(hash)

	pk := sk.PublicKey()
	q := parsed.Query()
	q.Set("sc", base64.URLEncoding.EncodeToString(pk[:]))
	q.Set("ss", base64.URLEncoding.EncodeToString(sig[:]))
	q.Set("sv", fmt.Sprintf("%d", validUntil.Unix()))
	parsed.RawQuery = q.Encode()

	signedReq := m.ctx.NewAPIRequest(method, parsed.String(), body)
	rec := httptest.NewRecorder()
	m.ctx.Router().ServeHTTP(rec, signedReq)
	return rec
}

// storageHashDefaultMatcher returns a nil-safe matcher for *core.StorageHashDefault arguments.
func storageHashDefaultMatcher() interface{} {
	return mock.MatchedBy(func(h *core.StorageHashDefault) bool { return true })
}

// storageHashMatcher returns a nil-safe matcher for core.StorageHash (interface) arguments.
func storageHashMatcher() interface{} {
	return mock.MatchedBy(func(h core.StorageHash) bool { return true })
}

// setupPinServiceMocks configures all pin service mock expectations.
func (m *mockHelper) setupPinServiceMocks() {
	mockPinSvc := core.GetService[*coreMocks.MockPinService](m.ctx, core.PIN_SERVICE)
	mockPinSvc.EXPECT().CreatePin(mock.Anything, mock.AnythingOfType("*models.Pin"), mock.Anything).Return(&models.Pin{
		Model:    gorm.Model{ID: 1},
		UserID:   TestUserID,
		UploadID: 1,
	}, nil).Maybe()
	mockPinSvc.EXPECT().GetPinByHash(mock.Anything, storageHashDefaultMatcher(), mock.AnythingOfType("uint")).Return(&models.Pin{
		Model:    gorm.Model{ID: 1},
		UserID:   TestUserID,
		UploadID: 1,
	}, nil).Maybe()
	mockPinSvc.EXPECT().DeletePin(mock.Anything, mock.AnythingOfType("uint")).Return(nil).Maybe()
	mockPinSvc.EXPECT().UploadPinnedByUser(mock.Anything, storageHashDefaultMatcher(), mock.AnythingOfType("uint")).Return(false, nil).Maybe()
}

// setupUploadServiceMocks configures all upload service mock expectations.
func (m *mockHelper) setupUploadServiceMocks() {
	mockUploadSvc := core.GetService[*coreMocks.MockUploadService](m.ctx, core.UPLOAD_SERVICE)
	mockUploadSvc.EXPECT().SaveUpload(mock.Anything, mock.AnythingOfType("*models.Upload")).Return(nil).Maybe()
	mockUploadSvc.EXPECT().GetUploadByID(mock.Anything, mock.AnythingOfType("uint")).Return(&models.Upload{
		Model:  gorm.Model{ID: 1},
		UserID: TestUserID,
	}, nil).Maybe()
	mockUploadSvc.EXPECT().DeleteUpload(mock.Anything, storageHashMatcher()).Return(nil).Maybe()
}

// setupSignedAccount generates a key pair and configures mock expectations
// for Sia signed URL authentication. Returns the private key for signing requests.
func (m *mockHelper) setupSignedAccount() (types.PrivateKey, uint) {
	sk := types.GeneratePrivateKey()

	pk := sk.PublicKey()

	mockSiaService := core.GetService[*siaMocks.MockSiaService](m.ctx, pluginCore.SIA_SERVICE)
	mockSiaService.EXPECT().GetAppAccountByKey(mock.Anything, pk).Return(
		&db.SiaAppAccount{
			Model:        gorm.Model{ID: 1},
			SiaAccountID: 1,
			AccountKey:   db.DBAccountKeyFromPublicKey(pk),
		}, nil).Maybe()

	mockSiaService.EXPECT().GetAccount(mock.Anything, uint(1)).Return(
		createMockSiaAccount(TestUserID, TestAccountKey, TestConnectKey, TestQuotaKey), nil).Maybe()

	// Setup indexd object/slab management expectations
	mockSiaService.EXPECT().RegisterSlab(mock.Anything, mock.AnythingOfType("uint"), mock.AnythingOfType("string")).Return(nil).Maybe()
	mockSiaService.EXPECT().RegisterObject(mock.Anything, mock.AnythingOfType("uint"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string")).Return(nil).Maybe()
	mockSiaService.EXPECT().DeleteObject(mock.Anything, mock.AnythingOfType("uint"), mock.AnythingOfType("string")).Return(nil).Maybe()
	mockSiaService.EXPECT().DeleteSlab(mock.Anything, mock.AnythingOfType("uint"), mock.AnythingOfType("string")).Return(nil).Maybe()
	mockSiaService.EXPECT().PruneSlabs(mock.Anything, mock.AnythingOfType("uint")).Return(nil).Maybe()

	m.SetupQuotaServiceMocks(TestUserID)
	m.setupUploadServiceMocks()
	m.setupPinServiceMocks()

	return sk, TestUserID
}
