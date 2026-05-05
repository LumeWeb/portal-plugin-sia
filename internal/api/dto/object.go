package dto

import "go.sia.tech/indexd/slabs"

// ObjectListResponse is the response type for listing objects.
// This provides a named type for the swagger documentation.
type ObjectListResponse []slabs.ObjectEvent
