package db

import "gorm.io/gorm"

type SiaSlab struct {
	gorm.Model
	SiaAppAccountID uint
	SlabID          string
}

func (SiaSlab) TableName() string {
	return "sia_slabs"
}

// SiaObject tracks objects registered by app accounts to detect orphaned slabs
type SiaObject struct {
	gorm.Model
	SiaAppAccountID uint
	ObjectID        string
	ObjectSlabs     []SiaObjectSlab
}

func (SiaObject) TableName() string {
	return "sia_objects"
}

// SiaObjectSlab joins SiaObject to SiaSlabs (N:M relationship)
type SiaObjectSlab struct {
	gorm.Model
	SiaObjectID uint
	SiaSlabID   uint
}

func (SiaObjectSlab) TableName() string {
	return "sia_object_slabs"
}
