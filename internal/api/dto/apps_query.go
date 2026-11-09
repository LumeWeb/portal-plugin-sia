package dto

import (
	"encoding/hex"
	"errors"

	"go.lumeweb.com/queryutil/filter"
)

var appFields = []string{"id", "account_key", "created_at", "updated_at"}

// AllScalarFieldRules also allows pattern and null operators, which are not
// useful for these non-null ID and timestamp columns.
var scalarOperators = []filter.Operator{
	filter.OpEq, filter.OpNe, filter.OpLt, filter.OpGt, filter.OpLte, filter.OpGte,
	filter.OpIn, filter.OpNin, filter.OpBetween, filter.OpNbetween,
}

// AppQueryPolicy restricts local query fields and converts public keys to the raw bytes
// stored in sia_app_accounts. Display metadata is fetched from indexd.
func AppQueryPolicy() filter.QueryPolicy {
	return filter.QueryPolicy{
		Fields: map[string]filter.FieldRule{
			"id":          {Operators: scalarOperators},
			"created_at":  {Operators: scalarOperators},
			"updated_at":  {Operators: scalarOperators},
			"account_key": {Operators: []filter.Operator{filter.OpEq, filter.OpNe, filter.OpIn, filter.OpNin}, Convert: convertAccountKey},
		},
		SortFields: appFields,
	}
}

func convertAccountKey(op filter.Operator, value any) (any, error) {
	if op == filter.OpIn || op == filter.OpNin {
		values, ok := value.([]any)
		if !ok || len(values) == 0 {
			return nil, errors.New("invalid account key list")
		}
		converted := make([]any, len(values))
		for i, v := range values {
			key, err := decodeAccountKey(v)
			if err != nil {
				return nil, err
			}
			converted[i] = key
		}
		return converted, nil
	}
	return decodeAccountKey(value)
}

func decodeAccountKey(value any) ([]byte, error) {
	encoded, ok := value.(string)
	if !ok || len(encoded) != 64 {
		return nil, errors.New("account key must be 64 hex characters")
	}
	key, err := hex.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("invalid account key hex")
	}
	return key, nil
}
