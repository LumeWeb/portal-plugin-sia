package sia

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/queryutil"
	"go.lumeweb.com/queryutil/filter"
	"go.sia.tech/indexd/api/admin"
)

func TestListAppsFiltersByHexPublicKey(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := core.GetService[*SiaService](ctx, pluginCore.SIA_SERVICE)
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
		}))
		defer upstream.Close()
		svc.adminClient = admin.NewClient(upstream.URL, "test")
		owner, err := svc.RegisterAccount(ctx, 1)
		require.NoError(t, err)
		key := generateTestPublicKey()
		_, err = svc.RegisterAppAccount(ctx, owner.ID, key)
		require.NoError(t, err)
		_, err = svc.RegisterAppAccount(ctx, owner.ID, generateTestPublicKey())
		require.NoError(t, err)
		filters := []queryutil.CrudFilter{queryutil.NewLogicalFilter("account_key", filter.OpEq, hex.EncodeToString(key[:]))}
		rows, total, err := svc.ListApps(ctx, 1, filters, nil, queryutil.Pagination{})
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Len(t, rows, 1)
		require.Equal(t, key, rows[0].PublicKey)
	}, TestOptions)
}

func TestListAppsDefaultsUnspecifiedServicePaginationToTen(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := core.GetService[*SiaService](ctx, pluginCore.SIA_SERVICE)
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
		}))
		defer upstream.Close()
		svc.adminClient = admin.NewClient(upstream.URL, "test")
		owner, err := svc.RegisterAccount(ctx, 1)
		require.NoError(t, err)
		for i := 0; i < 11; i++ {
			_, err = svc.RegisterAppAccount(ctx, owner.ID, generateTestPublicKey())
			require.NoError(t, err)
		}
		rows, total, err := svc.ListApps(ctx, 1, nil, nil, queryutil.Pagination{})
		require.NoError(t, err)
		require.EqualValues(t, 11, total)
		require.Len(t, rows, 10)
	}, TestOptions)
}

func TestListAppsRejectsInvalidServicePagination(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := core.GetService[*SiaService](ctx, pluginCore.SIA_SERVICE)
		for _, page := range []queryutil.Pagination{
			{Start: 0, End: 101, PageSize: 101},
			{Start: -1, End: 9, PageSize: 10},
			{Start: 0, End: 10, PageSize: 0},
			{Start: 0, End: 20, PageSize: 10},
		} {
			rows, total, err := svc.ListApps(ctx, 1, nil, nil, page)
			require.Error(t, err, "pagination: %+v", page)
			require.Empty(t, rows)
			require.Zero(t, total)
		}
	}, TestOptions)
}

func TestListAppsRejectsUnsafeQueryFields(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := core.GetService[*SiaService](ctx, pluginCore.SIA_SERVICE)
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
		}))
		defer upstream.Close()
		svc.adminClient = admin.NewClient(upstream.URL, "test")
		owner, err := svc.RegisterAccount(ctx, 1)
		require.NoError(t, err)
		other, err := svc.RegisterAccount(ctx, 2)
		require.NoError(t, err)
		_, err = svc.RegisterAppAccount(ctx, owner.ID, generateTestPublicKey())
		require.NoError(t, err)
		_, err = svc.RegisterAppAccount(ctx, other.ID, generateTestPublicKey())
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/api/apps?"+url.Values{"filters[id) OR 1=1 --][eq]": {"1"}}.Encode(), nil)
		injected, _, _, err := queryutil.ParseRequest(req)
		require.NoError(t, err)
		cases := []struct {
			name    string
			filters []queryutil.CrudFilter
			sorts   []queryutil.Sort
		}{
			{"filter expression", injected, nil},
			{"nested expression", []queryutil.CrudFilter{queryutil.NewConditionalFilter(filter.LogicalOr, injected)}, nil},
			{"tenant field", []queryutil.CrudFilter{queryutil.NewLogicalFilter("sia_account_id", filter.OpEq, 2)}, nil},
			{"enriched field", []queryutil.CrudFilter{queryutil.NewLogicalFilter("name", filter.OpEq, "app")}, nil},
			{"malformed key", []queryutil.CrudFilter{queryutil.NewLogicalFilter("account_key", filter.OpEq, "bad")}, nil},
			{"sort expression", nil, []queryutil.Sort{{Field: "(SELECT 1)", Order: filter.OrderAsc}}},
			{"sort direction", nil, []queryutil.Sort{{Field: "id", Order: filter.SortOrder("asc; SELECT 1")}}},
			{"nil filter", []queryutil.CrudFilter{nil}, nil},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rows, total, err := svc.ListApps(ctx, 1, tc.filters, tc.sorts, queryutil.Pagination{})
				require.ErrorIs(t, err, filter.ErrInvalidQuery)
				require.Empty(t, rows)
				require.Zero(t, total)
			})
		}
		allowed := []queryutil.CrudFilter{queryutil.NewConditionalFilter(filter.LogicalOr, []queryutil.CrudFilter{
			queryutil.NewLogicalFilter("id", filter.OpEq, 1),
			queryutil.NewLogicalFilter("id", filter.OpEq, 2),
		})}
		rows, total, err := svc.ListApps(ctx, 1, allowed, []queryutil.Sort{{Field: "id", Order: filter.OrderDesc}}, queryutil.Pagination{})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.EqualValues(t, 1, total)
	}, TestOptions)
}
