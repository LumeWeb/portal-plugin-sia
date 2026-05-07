package util

import (
	core "go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"

	"go.lumeweb.com/portal-plugin-sia/internal"
	"go.lumeweb.com/portal-plugin-sia/internal/config"
)

func GetProtocolMock() coreTesting.TestContextBuilderOption {
	return coreTesting.WithCustomMockProtocol(internal.ProtocolName, func(ctx coreTesting.TestContext) core.Protocol {
		protoMock := coreTesting.NewMockProtocol(ctx.T(), internal.ProtocolName)

		protoMock.DisplayNameValue = internal.ProtocolDisplayName
		protoMock.ConfigValue = config.ProtocolConfig{AppURL: "http://localhost:8081"}

		return protoMock
	})
}
