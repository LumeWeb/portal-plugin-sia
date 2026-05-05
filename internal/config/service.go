package config

import (
	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
	"go.lumeweb.com/portal/config"
)

var (
	_ config.ServiceConfig                = (*ServiceConfig)(nil)
	_ configmanager.ConfigSchemaProvider = (*ServiceConfig)(nil)
	_ config.Defaults                     = (*ServiceConfig)(nil)
)

type ServiceConfig struct{}

func (c ServiceConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{})
}

func (c ServiceConfig) Defaults() map[string]any {
	return map[string]any{}
}
