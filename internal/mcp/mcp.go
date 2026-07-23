package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/east-true/agent-config-inspector/internal/provider/registry"
	"github.com/east-true/agent-config-inspector/internal/workspace"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

const (
	claudeID     = "anthropic-claude-code/cli"
	codexID      = "openai-codex/cli"
	maxMCPServer = 512
)

type UnsupportedError struct{ Provider string }

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("MCP inventory is not supported for provider %q", e.Provider)
}

type Service struct{ registry *registry.Registry }

func New(providerRegistry *registry.Registry) *Service { return &Service{registry: providerRegistry} }

func (s *Service) Inventory(_ context.Context, workspacePath string, options agentconfig.MCPInventoryOptions) (agentconfig.MCPInventoryReport, error) {
	if workspacePath == "" {
		workspacePath = "."
	}
	view, err := workspace.New(workspacePath, options.MaxSourceBytes, options.FollowSymlinks)
	if err != nil {
		return agentconfig.MCPInventoryReport{}, err
	}
	targets, err := normalizedTargets(view, options.Targets)
	if err != nil {
		return agentconfig.MCPInventoryReport{}, err
	}
	identities, err := s.identities(options.Providers)
	if err != nil {
		return agentconfig.MCPInventoryReport{}, err
	}
	providerIDs := make([]string, len(identities))
	for index, identity := range identities {
		providerIDs[index] = identity.ID
	}
	report := agentconfig.MCPInventoryReport{
		SchemaVersion: agentconfig.MCPInventorySchemaVersion,
		Tool:          agentconfig.ToolInfo{Name: "agent-config-inspector", Version: agentconfig.Version, AdapterRegistry: agentconfig.AdapterRegistryVersion},
		Request:       agentconfig.MCPInventoryRequest{Workspace: "<workspace>", Targets: targets, Providers: providerIDs},
		Privacy:       agentconfig.PrivacyInfo{Redaction: "safe", UserContextScanned: false, SensitiveOutput: false},
		Results:       []agentconfig.MCPInventoryResolution{}, Findings: []agentconfig.Finding{}, Complete: true,
	}
	for _, target := range targets {
		for _, identity := range identities {
			result, resultErr := inventoryProvider(view, identity, target)
			if resultErr != nil {
				return agentconfig.MCPInventoryReport{}, resultErr
			}
			if result.State != "complete" {
				report.Complete = false
			}
			report.Results = append(report.Results, result)
			report.Findings = append(report.Findings, result.Findings...)
		}
	}
	sortFindings(report.Findings)
	return report, nil
}

func (s *Service) identities(requested []string) ([]agentconfig.ProviderIdentity, error) {
	if len(requested) == 0 {
		requested = []string{"claude", "codex"}
	}
	seen := make(map[string]struct{})
	identities := make([]agentconfig.ProviderIdentity, 0, len(requested))
	for _, requestedID := range requested {
		adapter, err := s.registry.Get(requestedID)
		if err != nil {
			return nil, err
		}
		identity := adapter.Identity()
		if identity.ID != claudeID && identity.ID != codexID {
			return nil, &UnsupportedError{Provider: requestedID}
		}
		if _, duplicate := seen[identity.ID]; duplicate {
			continue
		}
		seen[identity.ID] = struct{}{}
		identities = append(identities, identity)
	}
	sort.Slice(identities, func(i, j int) bool { return identities[i].ID < identities[j].ID })
	return identities, nil
}

func inventoryProvider(view *workspace.View, identity agentconfig.ProviderIdentity, target string) (agentconfig.MCPInventoryResolution, error) {
	result := agentconfig.MCPInventoryResolution{
		Provider: identity, Capability: "repository-mcp-inventory", Target: target, ProjectRoot: view.ProjectRootDisplay(), State: "complete",
		AvailableServers: []agentconfig.MCPServerRecord{}, ExcludedServers: []agentconfig.MCPServerRecord{}, Evidence: evidence(identity.ID), Findings: []agentconfig.Finding{},
	}
	if identity.ID == claudeID {
		if err := inventoryClaude(view, target, &result); err != nil {
			return result, err
		}
	} else {
		if err := inventoryCodex(view, target, &result); err != nil {
			return result, err
		}
		result.Findings = append(result.Findings, agentconfig.Finding{
			Code: "ACI021", Severity: "info", Title: "Codex project trust is a runtime condition",
			Summary:   "Static inventory assumes project-scoped Codex configuration is trusted; an untrusted runtime may ignore repository MCP definitions.",
			Providers: []string{identity.ID}, Targets: []string{target}, Confidence: "high",
		})
	}
	sortRecords(result.AvailableServers)
	sortRecords(result.ExcludedServers)
	sortFindings(result.Findings)
	return result, nil
}

func inventoryClaude(view *workspace.View, target string, result *agentconfig.MCPInventoryResolution) error {
	const source = ".mcp.json"
	exists, err := view.Exists(source)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	file, err := view.Read(source)
	if err != nil {
		return sourceReadFailure(result, claudeID, target, source, err)
	}
	fragments, parseErr := parseClaudeSource(file.Bytes, file.Logical, file.Size)
	if parseErr != nil {
		result.State = "partial"
		result.Findings = append(result.Findings, finding("ACI090", "error", "Claude MCP source is invalid", parseErr.Error(), claudeID, target, source))
		return nil
	}
	if len(fragments) > maxMCPServer {
		fragments = fragments[:maxMCPServer]
		result.State = "partial"
		result.Findings = append(result.Findings, finding("ACI095", "error", "MCP server limit reached", fmt.Sprintf("At most %d repository MCP servers are inspected per provider and target.", maxMCPServer), claudeID, target, source))
	}
	var rawDocument struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	_ = json.Unmarshal(file.Bytes, &rawDocument)
	for _, fragment := range fragments {
		record, available, findings := inspectClaude(fragment, rawDocument.MCPServers[fragment.name], target)
		result.Findings = append(result.Findings, findings...)
		if available {
			result.AvailableServers = append(result.AvailableServers, record)
		} else {
			result.ExcludedServers = append(result.ExcludedServers, record)
		}
	}
	result.Findings = append(result.Findings, finding("ACI091", "info", "Claude project MCP approval is a runtime condition", "Repository MCP servers require runtime approval; static inventory does not read or predict approval state.", claudeID, target, source))
	return nil
}

func inspectClaude(fragment serverFragment, raw json.RawMessage, target string) (agentconfig.MCPServerRecord, bool, []agentconfig.Finding) {
	record := baseRecord(fragment)
	if !safeServerName(fragment.name) {
		record.Name = "<unsafe-server-name>"
		record.Status, record.Transport, record.Reason = "invalid", "unknown", "server name exceeds the safe 128-byte output bound or contains invalid characters"
		return record, false, []agentconfig.Finding{finding("ACI090", "error", "MCP server name is unsafe to report", record.Reason, claudeID, target, fragment.source)}
	}
	if _, reserved := claudeReservedNames[fragment.name]; reserved {
		record.Status, record.Transport, record.Reason = "invalid", "unknown", "server name is reserved by Claude Code and will be skipped"
		return record, false, []agentconfig.Finding{finding("ACI090", "error", "Claude MCP server name is reserved", record.Reason, claudeID, target, fragment.source)}
	}
	transport, transportState := claudeTransport(raw)
	record.Transport = transport
	record.CredentialFieldsPresent = hasAny(fragment.fields, "env", "headers", "headersHelper", "oauth")
	record.Executable = transport == "stdio" || hasAny(fragment.fields, "headersHelper")
	invalidReason := ""
	switch {
	case fragment.invalid || transportState == valueInvalid:
		invalidReason = "server declaration is not a valid MCP configuration object"
	case transportState == valueAbsent && fragment.url != valueAbsent:
		invalidReason = "remote Claude MCP servers require an explicit transport type"
	case transport == "unknown":
		invalidReason = "Claude MCP server requires a supported transport declaration"
	case transport == "stdio" && fragment.command != valuePresent:
		invalidReason = "stdio transport requires a non-empty string command"
	case transport != "stdio" && transport != "unknown" && fragment.url == valueEmpty:
		record.Status, record.Reason = "unconfigured", "remote transport has an empty URL and is not configured"
		finalizeDigest(&record)
		return record, false, riskFindings(record, claudeID, target)
	case transport != "stdio" && transport != "unknown" && fragment.url != valuePresent:
		invalidReason = "remote transport requires a non-empty string URL"
	}
	if invalidReason != "" {
		record.Status, record.Reason = "invalid", invalidReason
		findings := []agentconfig.Finding{finding("ACI090", "error", "Claude MCP server declaration is invalid", invalidReason, claudeID, target, fragment.source)}
		findings = append(findings, riskFindings(record, claudeID, target)...)
		finalizeDigest(&record)
		return record, false, findings
	}
	record.Status, record.Reason = "configured", "repository MCP server is statically configured; runtime approval and connectivity are unobserved"
	finalizeDigest(&record)
	return record, true, riskFindings(record, claudeID, target)
}

type mergedServer struct {
	fragment serverFragment
	sources  []string
	sizes    map[string]int64
}

func inventoryCodex(view *workspace.View, target string, result *agentconfig.MCPInventoryResolution) error {
	directories, err := view.DirectoriesToTarget(target)
	if err != nil {
		return err
	}
	merged := make(map[string]*mergedServer)
	for _, directory := range directories {
		source := workspace.Join(directory, ".codex/config.toml")
		exists, existsErr := view.Exists(source)
		if existsErr != nil {
			return existsErr
		}
		if !exists {
			continue
		}
		file, readErr := view.Read(source)
		if readErr != nil {
			if err := sourceReadFailure(result, codexID, target, source, readErr); err != nil {
				return err
			}
			continue
		}
		fragments, parseErr := parseCodexSource(file.Bytes, file.Logical, directory, file.Size)
		if parseErr != nil {
			result.State = "partial"
			result.Findings = append(result.Findings, finding("ACI090", "error", "Codex MCP source could not be safely interpreted", parseErr.Error(), codexID, target, source))
			continue
		}
		for _, fragment := range fragments {
			mergeCodex(merged, fragment)
		}
	}
	values := make([]*mergedServer, 0, len(merged))
	for _, server := range merged {
		values = append(values, server)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].fragment.name < values[j].fragment.name })
	if len(values) > maxMCPServer {
		values = values[:maxMCPServer]
		result.State = "partial"
		result.Findings = append(result.Findings, finding("ACI095", "error", "MCP server limit reached", fmt.Sprintf("At most %d repository MCP servers are inspected per provider and target.", maxMCPServer), codexID, target, ".codex/config.toml"))
	}
	for _, server := range values {
		record, available, findings := inspectCodex(*server, target)
		result.Findings = append(result.Findings, findings...)
		if available {
			result.AvailableServers = append(result.AvailableServers, record)
		} else {
			result.ExcludedServers = append(result.ExcludedServers, record)
		}
	}
	return nil
}

func mergeCodex(values map[string]*mergedServer, overlay serverFragment) {
	server := values[overlay.name]
	if server == nil {
		copyFragment := overlay
		copyFragment.fields = copySet(overlay.fields)
		copyFragment.invalidFields = copySet(overlay.invalidFields)
		values[overlay.name] = &mergedServer{fragment: copyFragment, sources: []string{overlay.source}, sizes: map[string]int64{overlay.source: overlay.sourceSize}}
		return
	}
	server.fragment.source = overlay.source
	server.fragment.scopeBase = overlay.scopeBase
	server.fragment.sourceSize = overlay.sourceSize
	server.fragment.invalid = server.fragment.invalid || overlay.invalid
	for field := range overlay.fields {
		server.fragment.fields[field] = struct{}{}
	}
	if overlay.command != valueAbsent {
		server.fragment.command = overlay.command
	}
	if overlay.url != valueAbsent {
		server.fragment.url = overlay.url
	}
	if overlay.enabled != nil {
		server.fragment.enabled = overlay.enabled
	}
	if overlay.required != nil {
		server.fragment.required = overlay.required
	}
	for _, field := range []string{"enabled", "required"} {
		if _, declared := overlay.fields[field]; !declared {
			continue
		}
		if _, invalid := overlay.invalidFields[field]; invalid {
			server.fragment.invalidFields[field] = struct{}{}
		} else {
			delete(server.fragment.invalidFields, field)
		}
	}
	if _, exists := server.sizes[overlay.source]; !exists {
		server.sources = append(server.sources, overlay.source)
		server.sizes[overlay.source] = overlay.sourceSize
	}
}

func inspectCodex(server mergedServer, target string) (agentconfig.MCPServerRecord, bool, []agentconfig.Finding) {
	record := baseRecord(server.fragment)
	record.ContributingSources = append([]string(nil), server.sources...)
	record.SourceBytes = 0
	for _, size := range server.sizes {
		record.SourceBytes += size
	}
	if !safeServerName(server.fragment.name) {
		record.Name, record.Transport, record.Status = "<unsafe-server-name>", "unknown", "invalid"
		record.Reason = "server name exceeds the safe 128-byte output bound or contains invalid characters"
		return record, false, []agentconfig.Finding{finding("ACI090", "error", "MCP server name is unsafe to report", record.Reason, codexID, target, server.fragment.source)}
	}
	record.Enabled = true
	if server.fragment.enabled != nil {
		record.Enabled = *server.fragment.enabled
	}
	if server.fragment.required != nil {
		record.Required = *server.fragment.required
	}
	record.CredentialFieldsPresent = hasAny(server.fragment.fields, "env", "env_vars", "experimental_environment", "auth", "bearer_token_env_var", "http_headers", "env_http_headers", "oauth", "oauth_resource", "scopes")
	record.Executable = server.fragment.command == valuePresent
	invalidReason := ""
	switch {
	case server.fragment.invalid || len(server.fragment.invalidFields) > 0 || server.fragment.command == valueInvalid || server.fragment.url == valueInvalid:
		invalidReason = "server declaration contains an invalid transport or boolean field"
	case server.fragment.command == valuePresent && server.fragment.url == valuePresent:
		invalidReason = "Codex MCP server cannot declare both stdio command and HTTP URL transports"
	case server.fragment.command == valuePresent:
		record.Transport = "stdio"
	case server.fragment.url == valuePresent:
		record.Transport = "http"
	case server.fragment.command == valueEmpty:
		record.Transport, invalidReason = "stdio", "stdio transport requires a non-empty string command"
	case server.fragment.url == valueEmpty:
		record.Transport, invalidReason = "http", "HTTP transport requires a non-empty string URL"
	default:
		invalidReason = "Codex MCP server requires either a command or URL transport"
	}
	findings := riskFindings(record, codexID, target)
	if len(server.sources) > 1 {
		findings = append(findings, finding("ACI094", "info", "Codex MCP server combines project layers", "The effective structural record merges matching server tables from multiple repository config layers; values remain hidden.", codexID, target, server.sources...))
	}
	if invalidReason != "" {
		record.Status, record.Reason = "invalid", invalidReason
		findings = append(findings, finding("ACI090", "error", "Codex MCP server declaration is invalid", invalidReason, codexID, target, server.fragment.source))
		finalizeDigest(&record)
		return record, false, findings
	}
	if !record.Enabled {
		record.Status, record.Reason = "disabled", "repository MCP server is configured but disabled by the effective project configuration"
		finalizeDigest(&record)
		return record, false, findings
	}
	record.Status, record.Reason = "configured", "repository MCP server is statically configured; trust, startup, authentication, and connectivity are unobserved"
	finalizeDigest(&record)
	return record, true, findings
}

func baseRecord(fragment serverFragment) agentconfig.MCPServerRecord {
	fields := make([]string, 0, len(fragment.fields))
	for field := range fragment.fields {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return agentconfig.MCPServerRecord{
		Name: fragment.name, DisplayPath: fragment.source, ScopeBase: fragment.scopeBase, Transport: "unknown", Status: "invalid",
		Enabled: true, DeclaredFields: fields, SourceBytes: fragment.sourceSize, Reason: "server declaration has not been classified",
	}
}

func finalizeDigest(record *agentconfig.MCPServerRecord) {
	projection := struct {
		Name, DisplayPath, ScopeBase, Transport, Status string
		ContributingSources                             []string
		Enabled, Required, Executable, Credentials      bool
		DeclaredFields                                  []string
	}{record.Name, record.DisplayPath, record.ScopeBase, record.Transport, record.Status, record.ContributingSources, record.Enabled, record.Required, record.Executable, record.CredentialFieldsPresent, record.DeclaredFields}
	encoded, _ := json.Marshal(projection)
	prefixed := append([]byte("agent-config-inspector/mcp-metadata/v1\x00"), encoded...)
	sum := sha256.Sum256(prefixed)
	record.MetadataDigest = &agentconfig.Digest{Algorithm: "sha256", Value: hex.EncodeToString(sum[:])}
}

func riskFindings(record agentconfig.MCPServerRecord, providerID, target string) []agentconfig.Finding {
	var findings []agentconfig.Finding
	if record.CredentialFieldsPresent {
		findings = append(findings, finding("ACI092", "warning", "MCP credential-bearing fields are present", "The declaration contains environment, header, authentication, or OAuth-related fields; their names and values are hidden.", providerID, target, record.DisplayPath))
	}
	if record.Executable {
		findings = append(findings, finding("ACI093", "warning", "Repository MCP configuration may execute a local process", "The declaration uses stdio or a command helper. Inventory never executes it; review the hidden command before approval or trust.", providerID, target, record.DisplayPath))
	}
	return findings
}

func sourceReadFailure(result *agentconfig.MCPInventoryResolution, providerID, target, source string, err error) error {
	if errors.Is(err, workspace.ErrOutsideWorkspace) {
		return err
	}
	if errors.Is(err, workspace.ErrSymlink) {
		return err
	}
	reason := "MCP source could not be read safely"
	if errors.Is(err, workspace.ErrTooLarge) {
		reason = "MCP source exceeds the configured byte limit"
	}
	result.State = "partial"
	result.Findings = append(result.Findings, finding("ACI095", "error", "MCP source could not be inspected", reason, providerID, target, source))
	return nil
}

func finding(code, severity, title, summary, providerID, target string, sources ...string) agentconfig.Finding {
	return agentconfig.Finding{Code: code, Severity: severity, Title: title, Summary: summary, Providers: []string{providerID}, Targets: []string{target}, Sources: sources, Confidence: "high"}
}

func evidence(providerID string) []agentconfig.EvidenceRecord {
	if providerID == claudeID {
		return []agentconfig.EvidenceRecord{{
			ID: "claude-project-mcp-2026-07-24", Kind: "official-documentation", Claim: "Claude Code reads team-shared project MCP servers from the repository-root .mcp.json and requires runtime approval.",
			Provider: claudeID, Surface: "cli", VersionRange: "current documentation", SourceURL: "https://code.claude.com/docs/en/mcp", CheckedOn: "2026-07-24", Status: "verified",
		}}
	}
	return []agentconfig.EvidenceRecord{{
		ID: "codex-project-mcp-2026-07-24", Kind: "official-documentation-and-source", Claim: "Codex reads [mcp_servers] from trusted project .codex/config.toml layers and recursively merges TOML tables.",
		Provider: codexID, Surface: "cli", VersionRange: "current documentation", SourceURL: "https://learn.chatgpt.com/docs/extend/mcp", CheckedOn: "2026-07-24", Status: "verified",
	}}
}

func normalizedTargets(view *workspace.View, requested []string) ([]string, error) {
	if len(requested) == 0 {
		requested = []string{"."}
	}
	seen := make(map[string]struct{})
	targets := make([]string, 0, len(requested))
	for _, target := range requested {
		normalized, err := view.NormalizeTarget(target)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[normalized]; duplicate {
			continue
		}
		seen[normalized] = struct{}{}
		targets = append(targets, normalized)
	}
	sort.Strings(targets)
	return targets, nil
}

func sortRecords(records []agentconfig.MCPServerRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].Name != records[j].Name {
			return records[i].Name < records[j].Name
		}
		return records[i].DisplayPath < records[j].DisplayPath
	})
}

func sortFindings(findings []agentconfig.Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Code != findings[j].Code {
			return findings[i].Code < findings[j].Code
		}
		left, right := strings.Join(findings[i].Sources, "\x00"), strings.Join(findings[j].Sources, "\x00")
		return left < right
	})
}

func hasAny(values map[string]struct{}, keys ...string) bool {
	for _, key := range keys {
		if _, exists := values[key]; exists {
			return true
		}
	}
	return false
}

func copySet(source map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{}, len(source))
	for key := range source {
		result[key] = struct{}{}
	}
	return result
}
