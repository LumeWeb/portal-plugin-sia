package sia

import (
	"go.lumeweb.com/portal-plugin-sia/internal/info"
	core "go.lumeweb.com/portal/core"
)

func init() {
	core.RegisterPlugin(info.GetPluginInfo())
}
