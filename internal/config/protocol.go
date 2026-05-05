package config

import (
	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
	"go.lumeweb.com/portal/config"
)

var (
	_ config.ProtocolConfig              = (*ProtocolConfig)(nil)
	_ configmanager.ConfigSchemaProvider = (*ProtocolConfig)(nil)
	_ config.Defaults                     = (*ProtocolConfig)(nil)
)

type ProtocolConfig struct {
	Key    string `config:"key"`     // Admin password for indexd
	URL    string `config:"url"`     // Admin URL for indexd (indexd admin API)
	AppURL string `config:"app_url"` // App URL for user-facing indexd (app API) - internal
}

func (c ProtocolConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Key":    z.String(),
		"URL":    z.String(),
		"AppURL": z.String(),
	})
}

func (c ProtocolConfig) Defaults() map[string]any {
	return map[string]any{
		"Key":    "",
		"URL":    "",
		"AppURL": "",
	}
}
