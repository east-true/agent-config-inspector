package agentconfig

var Version = "0.9.0-dev"

const (
	SchemaVersion               = 1
	AdapterRegistryVersion      = "2026-07-24.8"
	SkillInventorySchemaVersion = 1
	AgentInventorySchemaVersion = 1
	MCPInventorySchemaVersion   = 1
)

type ProviderIdentity struct {
	ID              string `json:"id"`
	Provider        string `json:"provider"`
	Surface         string `json:"surface"`
	ReportedVersion string `json:"reported_version"`
	AdapterID       string `json:"adapter_id"`
	Support         string `json:"support"`
	Depth           string `json:"depth"`
}

type Digest struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

type Scope struct {
	Type     string   `json:"type"`
	Base     string   `json:"base,omitempty"`
	Patterns []string `json:"patterns,omitempty"`
	Status   string   `json:"status"`
	Reason   string   `json:"reason"`
}

type Unit struct {
	Kind        string `json:"kind"`
	SourceID    string `json:"source_id"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	Digest      Digest `json:"digest"`
	Text        string `json:"-"`
	Normalized  string `json:"-"`
	Command     string `json:"-"`
	Prohibition bool   `json:"-"`
}

type Source struct {
	ID                string  `json:"id"`
	Origin            string  `json:"origin"`
	LogicalPath       string  `json:"logical_path,omitempty"`
	DisplayPath       string  `json:"display_path"`
	Kind              string  `json:"kind"`
	ContentVisibility string  `json:"content_visibility"`
	SizeBytes         int64   `json:"size_bytes,omitempty"`
	RawDigest         *Digest `json:"raw_digest,omitempty"`
	NormalizedDigest  *Digest `json:"normalized_digest,omitempty"`
	Scope             Scope   `json:"scope"`
	Reason            string  `json:"reason"`
	Order             int     `json:"order,omitempty"`
	Units             []Unit  `json:"units,omitempty"`
	Content           string  `json:"-"`
	NormalizedContent string  `json:"-"`
}

type EvidenceRecord struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Claim        string `json:"claim"`
	Provider     string `json:"provider"`
	Surface      string `json:"surface"`
	VersionRange string `json:"version_range"`
	SourceURL    string `json:"source_url"`
	CheckedOn    string `json:"checked_on"`
	Status       string `json:"status"`
}

type Finding struct {
	Code        string   `json:"code"`
	Severity    string   `json:"severity"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Providers   []string `json:"providers,omitempty"`
	Targets     []string `json:"targets,omitempty"`
	Sources     []string `json:"sources,omitempty"`
	Confidence  string   `json:"confidence"`
	Remediation []string `json:"remediation,omitempty"`
}

type TokenEstimate struct {
	Method string `json:"method"`
	Value  int    `json:"value"`
}

type Resolution struct {
	Provider        ProviderIdentity `json:"provider"`
	Target          string           `json:"target"`
	ProjectRoot     string           `json:"project_root"`
	State           string           `json:"state"`
	Prediction      string           `json:"prediction"`
	IncludedSources []Source         `json:"included_sources"`
	ExcludedSources []Source         `json:"excluded_sources"`
	EffectiveDigest Digest           `json:"effective_digest"`
	TokenEstimate   TokenEstimate    `json:"token_estimate"`
	Evidence        []EvidenceRecord `json:"evidence"`
	Findings        []Finding        `json:"findings,omitempty"`
}

type Comparison struct {
	Target          string   `json:"target"`
	Providers       []string `json:"providers"`
	OnlyInFirst     []string `json:"only_in_first,omitempty"`
	OnlyInSecond    []string `json:"only_in_second,omitempty"`
	SharedUnitCount int      `json:"shared_unit_count"`
	Equivalent      bool     `json:"equivalent"`
}

type ToolInfo struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	AdapterRegistry string `json:"adapter_registry"`
}

type RequestInfo struct {
	Workspace      string   `json:"workspace"`
	WorkspaceLabel string   `json:"workspace_label,omitempty"`
	Targets        []string `json:"targets"`
	Providers      []string `json:"providers"`
}

type PrivacyInfo struct {
	Redaction          string `json:"redaction"`
	UserContextScanned bool   `json:"user_context_scanned"`
	SensitiveOutput    bool   `json:"sensitive_output"`
}

type Report struct {
	SchemaVersion int          `json:"schema_version"`
	Tool          ToolInfo     `json:"tool"`
	Request       RequestInfo  `json:"request"`
	Privacy       PrivacyInfo  `json:"privacy"`
	Results       []Resolution `json:"results"`
	Comparisons   []Comparison `json:"comparisons,omitempty"`
	Findings      []Finding    `json:"findings,omitempty"`
	Complete      bool         `json:"complete"`
}

type ScanOptions struct {
	Targets            []string
	Providers          []string
	WorkspaceLabel     string
	IncludeUserContext bool
	FollowSymlinks     bool
	MaxSourceBytes     int64
	MaxImportDepth     int
}

type SkillInventoryRequest struct {
	Workspace string   `json:"workspace"`
	Targets   []string `json:"targets"`
	Providers []string `json:"providers"`
}

type SkillRecord struct {
	Name               string  `json:"name"`
	DeclaredName       string  `json:"declared_name,omitempty"`
	DisplayPath        string  `json:"display_path"`
	ScopeBase          string  `json:"scope_base"`
	SourceDigest       *Digest `json:"source_digest,omitempty"`
	SourceBytes        int64   `json:"source_bytes,omitempty"`
	MetadataStatus     string  `json:"metadata_status"`
	DescriptionPresent bool    `json:"description_present"`
	DescriptionBytes   int     `json:"description_bytes,omitempty"`
	DescriptionDigest  *Digest `json:"description_digest,omitempty"`
	Reason             string  `json:"reason"`
}

type SkillInventoryResolution struct {
	Provider        ProviderIdentity `json:"provider"`
	Capability      string           `json:"capability"`
	Target          string           `json:"target"`
	ProjectRoot     string           `json:"project_root"`
	State           string           `json:"state"`
	AvailableSkills []SkillRecord    `json:"available_skills"`
	ExcludedSkills  []SkillRecord    `json:"excluded_skills"`
	Evidence        []EvidenceRecord `json:"evidence"`
	Findings        []Finding        `json:"findings,omitempty"`
}

type SkillInventoryReport struct {
	SchemaVersion int                        `json:"schema_version"`
	Tool          ToolInfo                   `json:"tool"`
	Request       SkillInventoryRequest      `json:"request"`
	Privacy       PrivacyInfo                `json:"privacy"`
	Results       []SkillInventoryResolution `json:"results"`
	Findings      []Finding                  `json:"findings,omitempty"`
	Complete      bool                       `json:"complete"`
}

type SkillInventoryOptions struct {
	Targets        []string
	Providers      []string
	FollowSymlinks bool
	MaxSourceBytes int64
}

type AgentInventoryRequest struct {
	Workspace string   `json:"workspace"`
	Targets   []string `json:"targets"`
	Providers []string `json:"providers"`
}

type AgentRecord struct {
	Name                 string   `json:"name"`
	DisplayPath          string   `json:"display_path"`
	ScopeBase            string   `json:"scope_base"`
	Format               string   `json:"format"`
	SourceDigest         *Digest  `json:"source_digest,omitempty"`
	SourceBytes          int64    `json:"source_bytes,omitempty"`
	MetadataStatus       string   `json:"metadata_status"`
	DescriptionPresent   bool     `json:"description_present"`
	DescriptionBytes     int      `json:"description_bytes,omitempty"`
	DescriptionDigest    *Digest  `json:"description_digest,omitempty"`
	InstructionsPresent  bool     `json:"instructions_present"`
	InstructionsBytes    int      `json:"instructions_bytes,omitempty"`
	InstructionsDigest   *Digest  `json:"instructions_digest,omitempty"`
	DeclaredCapabilities []string `json:"declared_capabilities,omitempty"`
	Reason               string   `json:"reason"`
}

type AgentInventoryResolution struct {
	Provider        ProviderIdentity `json:"provider"`
	Capability      string           `json:"capability"`
	Target          string           `json:"target"`
	ProjectRoot     string           `json:"project_root"`
	State           string           `json:"state"`
	AvailableAgents []AgentRecord    `json:"available_agents"`
	ExcludedAgents  []AgentRecord    `json:"excluded_agents"`
	Evidence        []EvidenceRecord `json:"evidence"`
	Findings        []Finding        `json:"findings,omitempty"`
}

type AgentInventoryReport struct {
	SchemaVersion int                        `json:"schema_version"`
	Tool          ToolInfo                   `json:"tool"`
	Request       AgentInventoryRequest      `json:"request"`
	Privacy       PrivacyInfo                `json:"privacy"`
	Results       []AgentInventoryResolution `json:"results"`
	Findings      []Finding                  `json:"findings,omitempty"`
	Complete      bool                       `json:"complete"`
}

type AgentInventoryOptions struct {
	Targets        []string
	Providers      []string
	FollowSymlinks bool
	MaxSourceBytes int64
}

type MCPInventoryRequest struct {
	Workspace string   `json:"workspace"`
	Targets   []string `json:"targets"`
	Providers []string `json:"providers"`
}

// MCPServerRecord is a deliberately lossy, privacy-safe projection. It never
// carries command arguments, URLs, header or environment names or values,
// authentication material, OAuth details, tool names, or approval values.
type MCPServerRecord struct {
	Name                    string   `json:"name"`
	DisplayPath             string   `json:"display_path"`
	ContributingSources     []string `json:"contributing_sources,omitempty"`
	ScopeBase               string   `json:"scope_base"`
	Transport               string   `json:"transport"`
	Status                  string   `json:"status"`
	Enabled                 bool     `json:"enabled"`
	Required                bool     `json:"required"`
	Executable              bool     `json:"executable"`
	CredentialFieldsPresent bool     `json:"credential_fields_present"`
	DeclaredFields          []string `json:"declared_fields,omitempty"`
	MetadataDigest          *Digest  `json:"metadata_digest,omitempty"`
	SourceBytes             int64    `json:"source_bytes,omitempty"`
	Reason                  string   `json:"reason"`
}

type MCPInventoryResolution struct {
	Provider         ProviderIdentity  `json:"provider"`
	Capability       string            `json:"capability"`
	Target           string            `json:"target"`
	ProjectRoot      string            `json:"project_root"`
	State            string            `json:"state"`
	AvailableServers []MCPServerRecord `json:"available_servers"`
	ExcludedServers  []MCPServerRecord `json:"excluded_servers"`
	Evidence         []EvidenceRecord  `json:"evidence"`
	Findings         []Finding         `json:"findings,omitempty"`
}

type MCPInventoryReport struct {
	SchemaVersion int                      `json:"schema_version"`
	Tool          ToolInfo                 `json:"tool"`
	Request       MCPInventoryRequest      `json:"request"`
	Privacy       PrivacyInfo              `json:"privacy"`
	Results       []MCPInventoryResolution `json:"results"`
	Findings      []Finding                `json:"findings,omitempty"`
	Complete      bool                     `json:"complete"`
}

type MCPInventoryOptions struct {
	Targets        []string
	Providers      []string
	FollowSymlinks bool
	MaxSourceBytes int64
}
