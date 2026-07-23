package provider

import (
	"context"

	"github.com/east-true/agent-config-inspector/internal/workspace"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

type Options struct {
	IncludeUserContext bool
	MaxImportDepth     int
	ExternalSources    []ExternalSource
}

type ExternalSource struct {
	Label   string
	Kind    string
	Content []byte
}

type Adapter interface {
	Identity() agentconfig.ProviderIdentity
	Resolve(context.Context, *workspace.View, string, Options) (agentconfig.Resolution, error)
}
