package registry

import (
	"fmt"
	"sort"
	"strings"

	"github.com/east-true/agent-config-inspector/internal/provider"
	"github.com/east-true/agent-config-inspector/internal/provider/claude"
	"github.com/east-true/agent-config-inspector/internal/provider/codex"
	"github.com/east-true/agent-config-inspector/internal/provider/copilot"
	"github.com/east-true/agent-config-inspector/internal/provider/gemini"
	"github.com/east-true/agent-config-inspector/internal/provider/kimi"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

type Registry struct {
	adapters map[string]provider.Adapter
	aliases  map[string]string
}

type UnsupportedError struct{ ID string }

func (e *UnsupportedError) Error() string { return fmt.Sprintf("unsupported provider %q", e.ID) }

func Builtin() *Registry {
	items := []provider.Adapter{claude.New(), codex.New(), copilot.New(), gemini.New(), kimi.New()}
	result := &Registry{adapters: make(map[string]provider.Adapter), aliases: make(map[string]string)}
	for _, item := range items {
		result.adapters[item.Identity().ID] = item
	}
	result.aliases["claude"] = "anthropic-claude-code/cli"
	result.aliases["claude-code"] = "anthropic-claude-code/cli"
	result.aliases["codex"] = "openai-codex/cli"
	result.aliases["copilot"] = "github-copilot/cli"
	result.aliases["copilot-cli"] = "github-copilot/cli"
	result.aliases["gemini"] = "google-gemini/cli"
	result.aliases["gemini-cli"] = "google-gemini/cli"
	result.aliases["kimi"] = "moonshotai-kimi-code/cli"
	result.aliases["kimi-code"] = "moonshotai-kimi-code/cli"
	return result
}

func (r *Registry) Get(id string) (provider.Adapter, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if canonical, ok := r.aliases[id]; ok {
		id = canonical
	}
	adapter, ok := r.adapters[id]
	if !ok {
		return nil, &UnsupportedError{ID: id}
	}
	return adapter, nil
}

func (r *Registry) Identities() []agentconfig.ProviderIdentity {
	items := make([]agentconfig.ProviderIdentity, 0, len(r.adapters))
	for _, adapter := range r.adapters {
		items = append(items, adapter.Identity())
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

func (r *Registry) DefaultIDs() []string {
	identities := r.Identities()
	result := make([]string, 0, len(identities))
	for _, identity := range identities {
		result = append(result, identity.ID)
	}
	return result
}
