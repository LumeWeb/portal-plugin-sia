package core

import (
	mh "github.com/multiformats/go-multihash"
)

const (
	SIA_SERVICE   = "sia.sia"
	QUOTA_SERVICE = "sia.quota"
)

const (
	MimeTypeSiaObject = "application/x-sia-object"
	MimeTypeSiaSlab   = "application/x-sia-slab"
)

const HashTypeBLAKE2b256 = mh.BLAKE2B_MIN + 32
