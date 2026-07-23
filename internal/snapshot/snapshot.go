package snapshot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/east-true/agent-config-inspector/internal/parser"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

const SchemaVersion = 1

var (
	ErrInvalidSnapshot = errors.New("invalid repository snapshot")
	ErrUnsafePath      = errors.New("snapshot path escapes workspace")
)

type Lockfile struct {
	SchemaVersion   int                `json:"schema_version"`
	Tool            Tool               `json:"tool"`
	AdapterRegistry string             `json:"adapter_registry"`
	Request         Request            `json:"request"`
	Entries         []Entry            `json:"entries"`
	LockDigest      agentconfig.Digest `json:"lock_digest"`
}

type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Request struct {
	Targets   []string `json:"targets"`
	Providers []string `json:"providers"`
}

type Entry struct {
	Provider                  ProviderIdentity   `json:"provider"`
	Target                    string             `json:"target"`
	State                     string             `json:"state"`
	Prediction                string             `json:"prediction"`
	IncludedSources           []Source           `json:"included_sources"`
	ExcludedSources           []Source           `json:"excluded_sources"`
	EffectiveRepositoryDigest agentconfig.Digest `json:"effective_repository_digest"`
}

type ProviderIdentity struct {
	ID                 string `json:"id"`
	AdapterID          string `json:"adapter_id"`
	VersionRequirement string `json:"version_requirement"`
	Support            string `json:"support"`
	Depth              string `json:"depth"`
}

type Source struct {
	ID               string              `json:"id"`
	LogicalPath      string              `json:"logical_path"`
	Kind             string              `json:"kind"`
	RawDigest        *agentconfig.Digest `json:"raw_digest,omitempty"`
	NormalizedDigest *agentconfig.Digest `json:"normalized_digest,omitempty"`
	Scope            agentconfig.Scope   `json:"scope"`
	Order            int                 `json:"order,omitempty"`
}

type Delta struct {
	Kind     string `json:"kind"`
	Provider string `json:"provider"`
	Target   string `json:"target"`
}

func Build(report agentconfig.Report) (Lockfile, error) {
	lock := Lockfile{
		SchemaVersion:   SchemaVersion,
		Tool:            Tool{Name: report.Tool.Name, Version: report.Tool.Version},
		AdapterRegistry: report.Tool.AdapterRegistry,
		Request: Request{
			Targets:   append([]string(nil), report.Request.Targets...),
			Providers: append([]string(nil), report.Request.Providers...),
		},
		Entries: []Entry{},
	}
	sort.Strings(lock.Request.Targets)
	sort.Strings(lock.Request.Providers)
	for _, result := range report.Results {
		entry := Entry{
			Provider: ProviderIdentity{
				ID: result.Provider.ID, AdapterID: result.Provider.AdapterID,
				VersionRequirement: result.Provider.ReportedVersion, Support: result.Provider.Support, Depth: result.Provider.Depth,
			},
			Target: result.Target, State: result.State, Prediction: result.Prediction,
			IncludedSources: []Source{}, ExcludedSources: []Source{},
		}
		var effectiveValues []string
		for _, source := range result.IncludedSources {
			if source.Origin != "repository" || source.LogicalPath == "" {
				continue
			}
			lockedSource := sourceFromReport(source)
			lockedSource.Order = len(entry.IncludedSources) + 1
			entry.IncludedSources = append(entry.IncludedSources, lockedSource)
			effectiveValues = append(effectiveValues, source.NormalizedContent)
		}
		for _, source := range result.ExcludedSources {
			if source.Origin != "repository" || source.LogicalPath == "" {
				continue
			}
			entry.ExcludedSources = append(entry.ExcludedSources, sourceFromReport(source))
		}
		if len(entry.IncludedSources) == 0 {
			entry.Prediction = "predicted-empty"
		} else {
			entry.Prediction = "predicted-effective"
		}
		sort.Slice(entry.ExcludedSources, func(i, j int) bool {
			if entry.ExcludedSources[i].LogicalPath != entry.ExcludedSources[j].LogicalPath {
				return entry.ExcludedSources[i].LogicalPath < entry.ExcludedSources[j].LogicalPath
			}
			return entry.ExcludedSources[i].ID < entry.ExcludedSources[j].ID
		})
		entry.EffectiveRepositoryDigest = parser.EffectiveDigest(effectiveValues)
		lock.Entries = append(lock.Entries, entry)
	}
	sort.Slice(lock.Entries, func(i, j int) bool {
		if lock.Entries[i].Target != lock.Entries[j].Target {
			return lock.Entries[i].Target < lock.Entries[j].Target
		}
		return lock.Entries[i].Provider.ID < lock.Entries[j].Provider.ID
	})
	digest, err := calculateDigest(lock)
	if err != nil {
		return Lockfile{}, err
	}
	lock.LockDigest = digest
	if err := Validate(lock); err != nil {
		return Lockfile{}, err
	}
	return lock, nil
}

func sourceFromReport(source agentconfig.Source) Source {
	return Source{
		ID: source.ID, LogicalPath: source.LogicalPath, Kind: source.Kind,
		RawDigest: cloneDigest(source.RawDigest), NormalizedDigest: cloneDigest(source.NormalizedDigest),
		Scope: agentconfig.Scope{
			Type: source.Scope.Type, Base: source.Scope.Base, Patterns: append([]string(nil), source.Scope.Patterns...),
			Status: source.Scope.Status, Reason: source.Scope.Reason,
		},
		Order: source.Order,
	}
}

func cloneDigest(value *agentconfig.Digest) *agentconfig.Digest {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func calculateDigest(lock Lockfile) (agentconfig.Digest, error) {
	lock.LockDigest = agentconfig.Digest{}
	canonical, err := CanonicalBytes(lock)
	if err != nil {
		return agentconfig.Digest{}, err
	}
	sum := sha256.Sum256(append([]byte("agent-config-inspector/repository-lock/v1\x00"), canonical...))
	return agentconfig.Digest{Algorithm: "sha256", Value: hex.EncodeToString(sum[:])}, nil
}

func CanonicalBytes(lock Lockfile) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(lock); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(output.Bytes(), []byte("\n")), nil
}

func Validate(lock Lockfile) error {
	if lock.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: unsupported schema_version %d", ErrInvalidSnapshot, lock.SchemaVersion)
	}
	if lock.Tool.Name != "agent-config-inspector" || lock.Tool.Version == "" || lock.AdapterRegistry == "" {
		return fmt.Errorf("%w: incomplete tool identity", ErrInvalidSnapshot)
	}
	if err := validateCanonicalStrings("targets", lock.Request.Targets); err != nil {
		return err
	}
	for _, target := range lock.Request.Targets {
		if err := validateLogicalPath(target); err != nil {
			return fmt.Errorf("%w: invalid request target", err)
		}
	}
	if err := validateCanonicalStrings("providers", lock.Request.Providers); err != nil {
		return err
	}
	for _, providerID := range lock.Request.Providers {
		if providerID != "anthropic-claude-code/cli" && providerID != "github-copilot/cli" && providerID != "google-gemini/cli" && providerID != "moonshotai-kimi-code/cli" && providerID != "openai-codex/cli" {
			return fmt.Errorf("%w: unsupported provider identity", ErrInvalidSnapshot)
		}
	}
	if len(lock.Entries) != len(lock.Request.Targets)*len(lock.Request.Providers) {
		return fmt.Errorf("%w: entries do not cover the request matrix", ErrInvalidSnapshot)
	}
	targets := stringSet(lock.Request.Targets)
	providers := stringSet(lock.Request.Providers)
	previousKey := ""
	for _, entry := range lock.Entries {
		key := entry.Target + "\x00" + entry.Provider.ID
		if entry.Target == "" || entry.Provider.ID == "" || entry.Provider.AdapterID == "" || key <= previousKey {
			return fmt.Errorf("%w: entries are missing identity, duplicated, or unsorted", ErrInvalidSnapshot)
		}
		if _, ok := targets[entry.Target]; !ok {
			return fmt.Errorf("%w: entry target is absent from the request", ErrInvalidSnapshot)
		}
		if _, ok := providers[entry.Provider.ID]; !ok {
			return fmt.Errorf("%w: entry provider is absent from the request", ErrInvalidSnapshot)
		}
		if entry.Provider.VersionRequirement == "" || entry.Provider.Support == "" || entry.Provider.Depth == "" {
			return fmt.Errorf("%w: incomplete provider identity", ErrInvalidSnapshot)
		}
		if entry.State != "complete" && entry.State != "partial" && entry.State != "unknown" {
			return fmt.Errorf("%w: unsupported entry state", ErrInvalidSnapshot)
		}
		if entry.Prediction != "predicted-effective" && entry.Prediction != "predicted-empty" {
			return fmt.Errorf("%w: unsupported entry prediction", ErrInvalidSnapshot)
		}
		if err := validateDigest("effective repository", entry.EffectiveRepositoryDigest); err != nil {
			return err
		}
		previousKey = key
		for index, source := range entry.IncludedSources {
			if source.Order != index+1 {
				return fmt.Errorf("%w: included source order is not canonical", ErrInvalidSnapshot)
			}
			if err := validateSource(source); err != nil {
				return err
			}
		}
		previousSourceKey := ""
		for _, source := range entry.ExcludedSources {
			if source.Order != 0 {
				return fmt.Errorf("%w: excluded source must not have precedence order", ErrInvalidSnapshot)
			}
			sourceKey := source.LogicalPath + "\x00" + source.ID
			if sourceKey <= previousSourceKey {
				return fmt.Errorf("%w: excluded sources are duplicated or unsorted", ErrInvalidSnapshot)
			}
			previousSourceKey = sourceKey
			if err := validateSource(source); err != nil {
				return err
			}
		}
		if (len(entry.IncludedSources) == 0) != (entry.Prediction == "predicted-empty") {
			return fmt.Errorf("%w: prediction contradicts included sources", ErrInvalidSnapshot)
		}
	}
	if err := validateDigest("lock", lock.LockDigest); err != nil {
		return err
	}
	want, err := calculateDigest(lock)
	if err != nil {
		return err
	}
	if lock.LockDigest.Algorithm != want.Algorithm || lock.LockDigest.Value != want.Value {
		return fmt.Errorf("%w: lock digest mismatch", ErrInvalidSnapshot)
	}
	return nil
}

func validateCanonicalStrings(name string, values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("%w: request %s are empty", ErrInvalidSnapshot, name)
	}
	previous := ""
	for _, value := range values {
		if value == "" || (previous != "" && value <= previous) {
			return fmt.Errorf("%w: request %s are empty, duplicated, or unsorted", ErrInvalidSnapshot, name)
		}
		previous = value
	}
	return nil
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func validateSource(source Source) error {
	if err := validateLogicalPath(source.LogicalPath); err != nil {
		return err
	}
	if source.ID == "" || source.Kind == "" || source.Scope.Type == "" || source.Scope.Status == "" || source.Scope.Reason == "" {
		return fmt.Errorf("%w: incomplete source identity", ErrInvalidSnapshot)
	}
	if source.Scope.Base != "" {
		if err := validateLogicalPath(source.Scope.Base); err != nil {
			return fmt.Errorf("%w: invalid source scope base", err)
		}
	}
	if source.RawDigest != nil {
		if err := validateDigest("raw source", *source.RawDigest); err != nil {
			return err
		}
	}
	if source.NormalizedDigest != nil {
		if err := validateDigest("normalized source", *source.NormalizedDigest); err != nil {
			return err
		}
	}
	return nil
}

func validateDigest(name string, digest agentconfig.Digest) error {
	if digest.Algorithm != "sha256" || len(digest.Value) != sha256.Size*2 {
		return fmt.Errorf("%w: invalid %s digest", ErrInvalidSnapshot, name)
	}
	if _, err := hex.DecodeString(digest.Value); err != nil || digest.Value != strings.ToLower(digest.Value) {
		return fmt.Errorf("%w: invalid %s digest", ErrInvalidSnapshot, name)
	}
	return nil
}

func validateLogicalPath(logical string) error {
	normalized := strings.ReplaceAll(logical, "\\", "/")
	if logical == "" || filepath.IsAbs(logical) || path.IsAbs(normalized) || hasWindowsDrivePrefix(normalized) {
		return fmt.Errorf("%w: non-relative source path", ErrInvalidSnapshot)
	}
	cleaned := path.Clean(normalized)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || cleaned != normalized {
		return fmt.Errorf("%w: non-canonical source path", ErrInvalidSnapshot)
	}
	for _, character := range logical {
		if character == 0 || unicode.IsControl(character) {
			return fmt.Errorf("%w: control character in source path", ErrInvalidSnapshot)
		}
	}
	return nil
}

func hasWindowsDrivePrefix(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':'
}

func Decode(reader io.Reader) (Lockfile, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var lock Lockfile
	if err := decoder.Decode(&lock); err != nil {
		return Lockfile{}, fmt.Errorf("%w: %v", ErrInvalidSnapshot, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Lockfile{}, fmt.Errorf("%w: trailing JSON value", ErrInvalidSnapshot)
	}
	if err := Validate(lock); err != nil {
		return Lockfile{}, err
	}
	return lock, nil
}

func Write(writer io.Writer, lock Lockfile) error {
	if err := Validate(lock); err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(lock)
}

func Equivalent(pinned, current Lockfile) bool {
	return pinned.LockDigest.Algorithm == current.LockDigest.Algorithm && pinned.LockDigest.Value == current.LockDigest.Value
}

func Diff(pinned, current Lockfile) []Delta {
	pinnedEntries := make(map[string]Entry, len(pinned.Entries))
	currentEntries := make(map[string]Entry, len(current.Entries))
	for _, entry := range pinned.Entries {
		pinnedEntries[entryKey(entry)] = entry
	}
	for _, entry := range current.Entries {
		currentEntries[entryKey(entry)] = entry
	}
	var deltas []Delta
	for key, entry := range pinnedEntries {
		currentEntry, ok := currentEntries[key]
		if !ok {
			deltas = append(deltas, Delta{Kind: "removed", Provider: entry.Provider.ID, Target: entry.Target})
			continue
		}
		pinnedBytes, _ := json.Marshal(entry)
		currentBytes, _ := json.Marshal(currentEntry)
		if !bytes.Equal(pinnedBytes, currentBytes) {
			deltas = append(deltas, Delta{Kind: "changed", Provider: entry.Provider.ID, Target: entry.Target})
		}
	}
	for key, entry := range currentEntries {
		if _, ok := pinnedEntries[key]; !ok {
			deltas = append(deltas, Delta{Kind: "added", Provider: entry.Provider.ID, Target: entry.Target})
		}
	}
	sort.Slice(deltas, func(i, j int) bool {
		if deltas[i].Target != deltas[j].Target {
			return deltas[i].Target < deltas[j].Target
		}
		if deltas[i].Provider != deltas[j].Provider {
			return deltas[i].Provider < deltas[j].Provider
		}
		return deltas[i].Kind < deltas[j].Kind
	})
	return deltas
}

func entryKey(entry Entry) string { return entry.Target + "\x00" + entry.Provider.ID }
