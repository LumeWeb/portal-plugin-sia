package appquery

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.lumeweb.com/queryutil"
	"go.lumeweb.com/queryutil/filter"
)

type customFilter struct{ *filter.LogicalFilter }

func TestPolicyRejectsUnsupportedFilterImplementationsAndGroups(t *testing.T) {
	cycle := &filter.ConditionalFilter{Operator: filter.LogicalAnd}
	cycle.Filters = []queryutil.CrudFilter{cycle}
	for _, f := range []queryutil.CrudFilter{
		(*filter.LogicalFilter)(nil),
		&customFilter{queryutil.NewLogicalFilter("id", filter.OpEq, 1)},
		&filter.ConditionalFilter{Operator: filter.LogicalAnd},
		&filter.ConditionalFilter{Operator: "unknown", Filters: []queryutil.CrudFilter{queryutil.NewLogicalFilter("id", filter.OpEq, 1)}},
		cycle,
	} {
		_, err := Policy().Normalize([]queryutil.CrudFilter{f}, nil)
		require.ErrorIs(t, err, filter.ErrInvalidQuery)
	}
}

func TestPolicyConvertsNestedHexAccountKeysWithoutMutatingInput(t *testing.T) {
	hexKey := strings.Repeat("ab", 32)
	leaf := queryutil.NewLogicalFilter("account_key", filter.OpIn, []any{hexKey})
	input := []queryutil.CrudFilter{queryutil.NewConditionalFilter(filter.LogicalOr, []queryutil.CrudFilter{leaf})}
	normalized, err := Policy().Normalize(input, nil)
	require.NoError(t, err)
	converted := normalized[0].GetFilters()[0].GetValue().([]any)
	require.Equal(t, []byte(strings.Repeat("\xab", 32)), converted[0])
	require.Equal(t, hexKey, leaf.Value().([]any)[0])
}

func TestPolicyRejectsMalformedAccountKeysAndUnsupportedOperators(t *testing.T) {
	for _, tc := range []struct {
		op    filter.Operator
		value any
	}{
		{filter.OpEq, "abcd"},
		{filter.OpEq, strings.Repeat("zz", 32)},
		{filter.OpEq, 42},
		{filter.OpIn, []any{strings.Repeat("ab", 32), "bad"}},
		{filter.OpContains, "ab"},
	} {
		_, err := Policy().Normalize([]queryutil.CrudFilter{queryutil.NewLogicalFilter("account_key", tc.op, tc.value)}, nil)
		require.ErrorIs(t, err, filter.ErrInvalidQuery)
	}
}
