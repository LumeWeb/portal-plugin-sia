package config

import (
	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
	"go.lumeweb.com/portal/config"
)

var (
	_ config.APIConfig                    = (*APIConfig)(nil)
	_ configmanager.ConfigSchemaProvider = (*APIConfig)(nil)
	_ config.Defaults                     = (*APIConfig)(nil)
)

type APIConfig struct{}

func (c APIConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{})
}

func (c APIConfig) Defaults() map[string]any {
	return map[string]any{}
}
