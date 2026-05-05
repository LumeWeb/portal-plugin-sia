package db

import (
	"time"

	"gorm.io/gorm"
)

type SiaAccount struct {
	gorm.Model
	UserID             uint
	ConnectKey         string
	QuotaKey           string
	FundTargetBytes    uint64
	LastFundingEventID int64
	LastFundingEventAt time.Time
	AppAccounts        []SiaAppAccount
}

func (SiaAccount) TableName() string {
	return "sia_accounts"
}

type SiaAppAccount struct {
	gorm.Model
	SiaAccountID uint
	AccountKey   string
}

func (SiaAppAccount) TableName() string {
	return "sia_app_accounts"
}
