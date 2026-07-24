package snapshot_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/east-true/agent-config-inspector/internal/parser"
	"github.com/east-true/agent-config-inspector/internal/snapshot"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

func TestRepositorySnapshot(t *testing.T) {
	t.Run("round trip is canonical", func(t *testing.T) {
		lock := buildLock(t, repositoryReport("Run tests"))
		var encoded bytes.Buffer
		if err := snapshot.Write(&encoded, lock); err != nil {
			t.Fatal(err)
		}
		decoded, err := snapshot.Decode(bytes.NewReader(encoded.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		if !snapshot.Equivalent(lock, decoded) {
			t.Fatalf("digest changed: %#v != %#v", lock.LockDigest, decoded.LockDigest)
		}
	})

	t.Run("build is deterministic", func(t *testing.T) {
		first := buildLock(t, repositoryReport("Run tests"))
		second := buildLock(t, repositoryReport("Run tests"))
		firstBytes, _ := snapshot.CanonicalBytes(first)
		secondBytes, _ := snapshot.CanonicalBytes(second)
		if !bytes.Equal(firstBytes, secondBytes) {
			t.Fatalf("canonical bytes differ\n%s\n%s", firstBytes, secondBytes)
		}
	})

	t.Run("user context existence cannot affect lockfile", func(t *testing.T) {
		withoutUser := repositoryReport("Run tests")
		withUser := repositoryReport("Run tests")
		withUser.Privacy.UserContextScanned = true
		withUser.Results[0].IncludedSources[0].Order = 2
		withUser.Results[0].IncludedSources = append([]agentconfig.Source{{
			ID: "private", Origin: "user", DisplayPath: "<user-instruction-1>", Kind: "claude-user-memory",
			ContentVisibility: "hidden", Content: "PRIVATE", NormalizedContent: "PRIVATE",
			Scope: agentconfig.Scope{Type: "always", Status: "applies", Reason: "user"}, Order: 1,
		}}, withUser.Results[0].IncludedSources...)
		first := buildLock(t, withoutUser)
		second := buildLock(t, withUser)
		firstBytes, _ := snapshot.CanonicalBytes(first)
		secondBytes, _ := snapshot.CanonicalBytes(second)
		if !bytes.Equal(firstBytes, secondBytes) || strings.Contains(string(secondBytes), "PRIVATE") || strings.Contains(string(secondBytes), "user-instruction") {
			t.Fatalf("user context affected lockfile\n%s\n%s", firstBytes, secondBytes)
		}
	})

	t.Run("workspace label cannot affect lockfile", func(t *testing.T) {
		withoutLabel := repositoryReport("Run tests")
		withLabel := repositoryReport("Run tests")
		withLabel.Request.WorkspaceLabel = "private-repository-label"
		first := buildLock(t, withoutLabel)
		second := buildLock(t, withLabel)
		firstBytes, _ := snapshot.CanonicalBytes(first)
		secondBytes, _ := snapshot.CanonicalBytes(second)
		if !bytes.Equal(firstBytes, secondBytes) || strings.Contains(string(secondBytes), "private-repository-label") {
			t.Fatalf("workspace label affected lockfile\n%s\n%s", firstBytes, secondBytes)
		}
	})

	t.Run("tampered digest is rejected", func(t *testing.T) {
		lock := buildLock(t, repositoryReport("Run tests"))
		lock.LockDigest.Value = strings.Repeat("0", 64)
		encoded, _ := json.Marshal(lock)
		_, err := snapshot.Decode(bytes.NewReader(encoded))
		if !errors.Is(err, snapshot.ErrInvalidSnapshot) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("entry drift is described", func(t *testing.T) {
		pinned := buildLock(t, repositoryReport("Run tests"))
		current := buildLock(t, repositoryReport("Run all tests"))
		deltas := snapshot.Diff(pinned, current)
		if len(deltas) != 1 || deltas[0].Kind != "changed" {
			t.Fatalf("deltas = %#v", deltas)
		}
	})

	t.Run("file round trip stays inside workspace", func(t *testing.T) {
		root := t.TempDir()
		lock := buildLock(t, repositoryReport("Run tests"))
		if err := snapshot.WriteFile(root, "agent-config-inspector.lock.json", lock); err != nil {
			t.Fatal(err)
		}
		decoded, err := snapshot.ReadFile(root, "agent-config-inspector.lock.json", 1<<20)
		if err != nil || !snapshot.Equivalent(lock, decoded) {
			t.Fatalf("decoded = %#v, err = %v", decoded, err)
		}
	})

	t.Run("valid existing snapshot can be replaced", func(t *testing.T) {
		root := t.TempDir()
		first := buildLock(t, repositoryReport("Run tests"))
		second := buildLock(t, repositoryReport("Run all tests"))
		if err := snapshot.WriteFile(root, "agent-config-inspector.lock.json", first); err != nil {
			t.Fatal(err)
		}
		if err := snapshot.WriteFile(root, "agent-config-inspector.lock.json", second); err != nil {
			t.Fatal(err)
		}
		decoded, err := snapshot.ReadFile(root, "agent-config-inspector.lock.json", 1<<20)
		if err != nil || !snapshot.Equivalent(second, decoded) {
			t.Fatalf("decoded = %#v, err = %v", decoded, err)
		}
	})

	t.Run("request matrix mismatch is rejected", func(t *testing.T) {
		lock := buildLock(t, repositoryReport("Run tests"))
		lock.Request.Targets = append(lock.Request.Targets, "nested")
		lock.LockDigest = agentconfig.Digest{}
		_, err := snapshot.CanonicalBytes(lock)
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		err = snapshot.Write(&output, lock)
		if !errors.Is(err, snapshot.ErrInvalidSnapshot) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("output escape is rejected", func(t *testing.T) {
		err := snapshot.WriteFile(t.TempDir(), "../outside.lock.json", buildLock(t, repositoryReport("Run tests")))
		if !errors.Is(err, snapshot.ErrUnsafePath) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("symlink output is rejected", func(t *testing.T) {
		root := t.TempDir()
		outside := filepath.Join(t.TempDir(), "outside.json")
		if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, "lock.json")); err != nil {
			t.Fatal(err)
		}
		err := snapshot.WriteFile(root, "lock.json", buildLock(t, repositoryReport("Run tests")))
		if !errors.Is(err, snapshot.ErrUnsafePath) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("unrelated existing output is not overwritten", func(t *testing.T) {
		root := t.TempDir()
		output := filepath.Join(root, "package.json")
		if err := os.WriteFile(output, []byte(`{"name":"keep-me"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		err := snapshot.WriteFile(root, "package.json", buildLock(t, repositoryReport("Run tests")))
		if !errors.Is(err, snapshot.ErrUnsafePath) {
			t.Fatalf("err = %v", err)
		}
		content, readErr := os.ReadFile(output)
		if readErr != nil || string(content) != `{"name":"keep-me"}` {
			t.Fatalf("content = %q, err = %v", content, readErr)
		}
	})

	t.Run("control character output is rejected", func(t *testing.T) {
		err := snapshot.WriteFile(t.TempDir(), "bad\nname.json", buildLock(t, repositoryReport("Run tests")))
		if !errors.Is(err, snapshot.ErrUnsafePath) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("Windows absolute paths are rejected on every host", func(t *testing.T) {
		err := snapshot.WriteFile(t.TempDir(), `C:\Users\alice\lock.json`, buildLock(t, repositoryReport("Run tests")))
		if !errors.Is(err, snapshot.ErrUnsafePath) {
			t.Fatalf("write err = %v", err)
		}
		report := repositoryReport("Run tests")
		report.Results[0].IncludedSources[0].LogicalPath = `C:\Users\alice\CLAUDE.md`
		if _, err := snapshot.Build(report); !errors.Is(err, snapshot.ErrInvalidSnapshot) {
			t.Fatalf("build err = %v", err)
		}
	})
}

func buildLock(t *testing.T, report agentconfig.Report) snapshot.Lockfile {
	t.Helper()
	lock, err := snapshot.Build(report)
	if err != nil {
		t.Fatal(err)
	}
	return lock
}

func emptyReport() agentconfig.Report {
	emptyDigest := parser.EffectiveDigest(nil)
	return agentconfig.Report{
		SchemaVersion: 1,
		Tool:          agentconfig.ToolInfo{Name: "agent-config-inspector", Version: "test", AdapterRegistry: "test-registry"},
		Request:       agentconfig.RequestInfo{Workspace: "<workspace>", Targets: []string{"."}, Providers: []string{"anthropic-claude-code/cli"}},
		Privacy:       agentconfig.PrivacyInfo{Redaction: "safe"},
		Results: []agentconfig.Resolution{{
			Provider: agentconfig.ProviderIdentity{
				ID: "anthropic-claude-code/cli", AdapterID: "builtin/claude/v1", ReportedVersion: "test", Support: "preview", Depth: "repository-instructions",
			},
			Target: ".", State: "complete", Prediction: "predicted-empty",
			IncludedSources: []agentconfig.Source{}, ExcludedSources: []agentconfig.Source{}, EffectiveDigest: emptyDigest,
		}},
		Complete: true,
	}
}

func repositoryReport(content string) agentconfig.Report {
	report := emptyReport()
	raw := parser.RawContentDigest([]byte(content))
	normalized := parser.ContentDigest(content)
	report.Results[0].Prediction = "predicted-effective"
	report.Results[0].IncludedSources = []agentconfig.Source{{
		ID: "anthropic-claude-code/cli:claude-memory:CLAUDE.md", Origin: "repository", LogicalPath: "CLAUDE.md",
		DisplayPath: "CLAUDE.md", Kind: "claude-memory", ContentVisibility: "metadata-only",
		RawDigest: &raw, NormalizedDigest: &normalized,
		Scope:  agentconfig.Scope{Type: "hierarchical", Base: ".", Status: "applies", Reason: "test"},
		Reason: "test", Order: 1, Content: content, NormalizedContent: content,
	}}
	return report
}
