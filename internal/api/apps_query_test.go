package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	siaMocks "go.lumeweb.com/portal-plugin-sia/internal/testing/mocks"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/queryutil"
)

func TestListAppsPreservesNumericOnlyHexAccountKey(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		helper := newMockHelper(t, ctx)
		token, _ := helper.SetupAuthenticatedTest()
		svc := core.GetService[*siaMocks.MockSiaService](ctx, pluginCore.SIA_SERVICE)
		key := strings.Repeat("0", 64)
		svc.EXPECT().ListApps(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, _ uint, filters []queryutil.CrudFilter, _ []queryutil.Sort, _ queryutil.Pagination) {
				require.Len(t, filters, 1)
				require.Equal(t, key, filters[0].GetValue())
			}).Return([]pluginCore.AppAccount{}, int64(0), nil).Once()
		query := url.Values{"filters[account_key][eq]": {key}}
		resp := helper.makeAuthenticatedRequest(http.MethodGet, "/api/apps?"+query.Encode(), token, nil)
		require.Equal(t, http.StatusOK, resp.Code)
	}, TestOptions)
}

func TestListAppsRejectsUnsafeQueryBeforeCallingService(t *testing.T) {
	for _, query := range []url.Values{
		{"filters[id) OR 1=1 --][eq]": {"1"}},
		{"filters[or][0][id][eq]": {"1"}, "filters[or][1][name) OR 1=1 --][eq]": {"1"}},
		{"_sort": {"(SELECT 1)"}},
		{"filters[sia_account_id][eq]": {"2"}},
		{"filters[account_key][eq]": {"not-a-public-key"}},
		{"filters[account_key][contains]": {"ab"}},
	} {
		t.Run(query.Encode(), func(t *testing.T) {
			coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
				helper := newMockHelper(t, ctx)
				token, _ := helper.SetupAuthenticatedTest()
				svc := core.GetService[*siaMocks.MockSiaService](ctx, pluginCore.SIA_SERVICE)
				svc.EXPECT().ListApps(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]pluginCore.AppAccount{}, int64(0), nil).Maybe()
				resp := helper.makeAuthenticatedRequest(http.MethodGet, "/api/apps?"+query.Encode(), token, nil)
				require.Equal(t, http.StatusBadRequest, resp.Code)
				svc.AssertNotCalled(t, "ListApps", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			}, TestOptions)
		})
	}
}
