package protocol

import (
	"context"
	"fmt"
	"io"

	mh "github.com/multiformats/go-multihash"
	"go.lumeweb.com/portal/config"
	core "go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models/data_models"

	pluginCore "go.lumeweb.com/portal-plugin-sia/core"
	"go.lumeweb.com/portal-plugin-sia/internal"
	pluginConfig "go.lumeweb.com/portal-plugin-sia/internal/config"
	"gorm.io/gorm"
)

type Protocol struct {
	*core.BaseComponent
	siaService pluginCore.SiaService
}

type pinHandler struct{}

func (p pinHandler) CreateProtocolPin(ctx context.Context, id uint, data any) error {
	_, span := core.TraceMethod(ctx, "pinHandler.CreateProtocolPin")
	defer span.End()
	return nil
}

func (p pinHandler) GetProtocolPin(ctx context.Context, tx *gorm.DB, id uint) (any, error) {
	_, span := core.TraceMethod(ctx, "pinHandler.GetProtocolPin")
	defer span.End()
	return nil, nil
}

func (p pinHandler) UpdateProtocolPin(ctx context.Context, id uint, data any) error {
	_, span := core.TraceMethod(ctx, "pinHandler.UpdateProtocolPin")
	defer span.End()
	return nil
}

func (p pinHandler) DeleteProtocolPin(ctx context.Context, id uint) error {
	_, span := core.TraceMethod(ctx, "pinHandler.DeleteProtocolPin")
	defer span.End()
	return nil
}

func (p pinHandler) QueryProtocolPin(ctx context.Context, query any) *gorm.DB {
	_, span := core.TraceMethod(ctx, "pinHandler.QueryProtocolPin")
	defer span.End()
	return nil
}

func (p pinHandler) GetProtocolPinModel() data_models.PinDataModel {
	return nil
}

func (p Protocol) PinHandler() core.ProtocolPinHandler {
	return &pinHandler{}
}

var (
	_ core.Protocol              = (*Protocol)(nil)
	_ core.StorageProtocol       = (*Protocol)(nil)
	_ core.ProtocolGetPinHandler = (*Protocol)(nil)
	_ core.ProtocolPinHandler    = (*pinHandler)(nil)
)

func (p *Protocol) EncodeFileName(hash core.StorageHash) string {
	decoded, err := mh.Decode(hash.Multihash())
	if err != nil {
		return hash.Multihash().HexString()
	}
	return fmt.Sprintf("%x", decoded.Digest)
}

func (p *Protocol) Hash(_ io.Reader, _ uint64) (core.StorageHash, error) {
	panic("sia: Hash is not supported; pins are created via indexd proxy, not direct hashing")
}

func (p *Protocol) Name() string {
	return internal.ProtocolName
}

func (p *Protocol) ID() string {
	return p.Name()
}

func (p *Protocol) DisplayName() string {
	return internal.ProtocolDisplayName
}

func (p *Protocol) GetConfig() config.ProtocolConfig {
	return &pluginConfig.ProtocolConfig{}
}

func (p *Protocol) Operations() []core.Operation {
	return []core.Operation{}
}

func (p *Protocol) Workflows() []core.WorkflowDefinition {
	return []core.WorkflowDefinition{}
}

func NewProtocol() (core.Protocol, []core.ContextBuilderOption, error) {
	proto := &Protocol{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			proto.siaService = core.GetService[pluginCore.SiaService](ctx, pluginCore.SIA_SERVICE)
			return nil
		}),
		core.ContextWithExitFunc(func(ctx core.Context) error {
			return nil
		}),
	)

	return proto, opts, nil
}
