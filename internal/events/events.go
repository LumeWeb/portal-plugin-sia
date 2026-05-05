package events

import (
	"context"

	"go.lumeweb.com/portal-plugin-sia/internal/db"
	core "go.lumeweb.com/portal/core"
)

const (
	EVENT_SIA_ACCOUNT_REGISTERED = "sia.account.registered"
)

type SiaAccountRegisteredEvent struct {
	Ctx     context.Context
	Account *db.SiaAccount
}

func NewSiaAccountRegisteredEvent(eventCtx context.Context, account *db.SiaAccount) *SiaAccountRegisteredEvent {
	if account == nil {
		panic("account cannot be nil in NewSiaAccountRegisteredEvent")
	}
	return &SiaAccountRegisteredEvent{
		Ctx:     eventCtx,
		Account: account,
	}
}

func OnSiaAccountRegistered(ctx core.Context, handler func(context.Context, *db.SiaAccount) error, priority ...int) {
	core.Listen[SiaAccountRegisteredEvent](ctx, EVENT_SIA_ACCOUNT_REGISTERED, func(e *core.CoreEvent[SiaAccountRegisteredEvent]) error {
		return handler(e.Data.Ctx, e.Data.Account)
	}, priority...)
}
