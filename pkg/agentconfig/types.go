package agentconfig

var Version = "0.6.0-dev"

const (
	SchemaVersion          = 1
	AdapterRegistryVersion = "2026-07-24.4"
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
	Workspace string   `json:"workspace"`
	Targets   []string `json:"targets"`
	Providers []string `json:"providers"`
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
	IncludeUserContext bool
	FollowSymlinks     bool
	MaxSourceBytes     int64
	MaxImportDepth     int
}
