package db

import "time"

type FundingCursor struct {
	ID                 int64
	LastFundingEventID int64
	LastFundingEventAt time.Time
}

func (FundingCursor) TableName() string { return "sia_funding_cursors" }
