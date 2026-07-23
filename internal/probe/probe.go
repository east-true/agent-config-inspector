package probe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

const (
	SchemaVersion    = 1
	DefaultCaseID    = "root-instruction-discovery"
	defaultMaxOutput = 1 << 20
)

type Status string

const (
	StatusConfirmed              Status = "confirmed"
	StatusNotObserved            Status = "not-observed"
	StatusBlockedBeforeModelCall Status = "blocked-before-model-call"
	StatusQuotaExhausted         Status = "quota-exhausted"
	StatusAuthUnavailable        Status = "auth-unavailable"
	StatusProviderError          Status = "provider-error"
	StatusInconclusive           Status = "inconclusive"
)

type UnsupportedCaseError struct {
	Provider string
	Case     string
}

func (e *UnsupportedCaseError) Error() string {
	return fmt.Sprintf("unsupported probe case %q for provider %q", e.Case, e.Provider)
}

type Plan struct {
	SchemaVersion        int                `json:"schema_version"`
	Kind                 string             `json:"kind"`
	Provider             string             `json:"provider"`
	Case                 string             `json:"case"`
	Claim                string             `json:"claim"`
	Binary               string             `json:"binary"`
	BinaryAvailable      bool               `json:"binary_available"`
	Invocation           []string           `json:"invocation"`
	Fixture              string             `json:"fixture"`
	FixtureDigest        agentconfig.Digest `json:"fixture_digest"`
	FilesystemMode       string             `json:"filesystem_mode"`
	Network              string             `json:"network"`
	TimeoutSeconds       int64              `json:"timeout_seconds"`
	ExpectedModelCalls   int                `json:"expected_model_calls"`
	MayConsumeQuota      bool               `json:"may_consume_quota"`
	RawResponseRetention string             `json:"raw_response_retention"`
	CredentialMode       string             `json:"credential_mode"`
	CredentialVariables  []string           `json:"credential_variables"`
	EvidenceURL          string             `json:"evidence_url"`
}

type Result struct {
	SchemaVersion          int    `json:"schema_version"`
	Kind                   string `json:"kind"`
	Plan                   Plan   `json:"plan"`
	Status                 Status `json:"status"`
	FailureStage           string `json:"failure_stage,omitempty"`
	DetectedVersion        string `json:"detected_version,omitempty"`
	MarkerObserved         bool   `json:"marker_observed"`
	ProviderProcessStarted bool   `json:"provider_process_started"`
	ProviderExitCode       *int   `json:"provider_exit_code,omitempty"`
	OutputTruncated        bool   `json:"output_truncated"`
}

type caseDefinition struct {
	providerID          string
	claim               string
	binary              string
	instructionFile     string
	marker              string
	evidenceURL         string
	credentialVariables []string
	credentialMode      string
	invocation          []string
	arguments           func(fixture fixture, prompt string) []string
	configure           func(fixture) error
	environment         func(fixture) map[string]string
}

type fixture struct {
	root      string
	workspace string
	home      string
}

type commandOutput struct {
	stdout    []byte
	stderr    []byte
	exitCode  *int
	err       error
	truncated bool
	started   bool
}

type executor interface {
	LookPath(string) (string, error)
	Run(context.Context, string, []string, string, []string, int) commandOutput
}

type systemExecutor struct{}

func (systemExecutor) LookPath(binary string) (string, error) { return exec.LookPath(binary) }

func (systemExecutor) Run(ctx context.Context, binary string, args []string, directory string, environment []string, maxOutput int) commandOutput {
	stdout := &boundedBuffer{limit: maxOutput}
	stderr := &boundedBuffer{limit: maxOutput}
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = directory
	command.Env = environment
	command.Stdin = strings.NewReader("")
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Start()
	if err != nil {
		return commandOutput{stdout: stdout.Bytes(), stderr: stderr.Bytes(), err: err}
	}
	err = command.Wait()
	var exitCode *int
	if command.ProcessState != nil {
		value := command.ProcessState.ExitCode()
		exitCode = &value
	}
	return commandOutput{
		stdout: stdout.Bytes(), stderr: stderr.Bytes(), exitCode: exitCode, err: err,
		truncated: stdout.truncated || stderr.truncated, started: true,
	}
}

type boundedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	original := len(value)
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return original, nil
	}
	if len(value) > remaining {
		value = value[:remaining]
		b.truncated = true
	}
	_, _ = b.buffer.Write(value)
	return original, nil
}

func (b *boundedBuffer) Bytes() []byte { return append([]byte(nil), b.buffer.Bytes()...) }

type Service struct {
	executor  executor
	getenv    func(string) string
	tempDir   func(string, string) (string, error)
	removeAll func(string) error
}

func New() *Service {
	return &Service{executor: systemExecutor{}, getenv: os.Getenv, tempDir: os.MkdirTemp, removeAll: cleanupFixture}
}

func (s *Service) Plan(providerID, caseID string, timeout time.Duration) (Plan, error) {
	definition, err := lookupCase(providerID, caseID)
	if err != nil {
		return Plan{}, err
	}
	_, lookupErr := s.executor.LookPath(definition.binary)
	return Plan{
		SchemaVersion: SchemaVersion, Kind: "probe-plan", Provider: definition.providerID, Case: DefaultCaseID,
		Claim: definition.claim, Binary: definition.binary, BinaryAvailable: lookupErr == nil,
		Invocation: append([]string(nil), definition.invocation...), Fixture: "generated-minimal-fixture-only",
		FixtureDigest: fixtureDigest(definition), FilesystemMode: "read-only generated workspace",
		Network: "provider-required access only; agent network tools disabled or denied", TimeoutSeconds: int64(timeout / time.Second), ExpectedModelCalls: 1,
		MayConsumeQuota: true, RawResponseRetention: "none", CredentialMode: definition.credentialMode,
		CredentialVariables: append([]string(nil), definition.credentialVariables...), EvidenceURL: definition.evidenceURL,
	}, nil
}

func (s *Service) Execute(ctx context.Context, providerID, caseID string, timeout time.Duration) (result Result) {
	plan, planErr := s.Plan(providerID, caseID, timeout)
	if planErr != nil {
		return Result{SchemaVersion: SchemaVersion, Kind: "probe-result", Status: StatusBlockedBeforeModelCall, FailureStage: "planning"}
	}
	result = Result{SchemaVersion: SchemaVersion, Kind: "probe-result", Plan: plan, Status: StatusInconclusive}
	definition, _ := lookupCase(providerID, caseID)
	binary, lookupErr := s.executor.LookPath(definition.binary)
	if lookupErr != nil {
		result.Status = StatusBlockedBeforeModelCall
		result.FailureStage = "binary-detection"
		return result
	}

	root, tempErr := s.tempDir("", "agent-config-inspector-probe-")
	if tempErr != nil {
		result.Status = StatusBlockedBeforeModelCall
		result.FailureStage = "fixture-preparation"
		return result
	}
	defer func() {
		if cleanupErr := s.removeAll(root); cleanupErr != nil {
			result.Status = StatusProviderError
			result.FailureStage = "fixture-cleanup"
		}
	}()
	prepared, prepareErr := prepareFixture(root, definition)
	if prepareErr != nil {
		result.Status = StatusBlockedBeforeModelCall
		result.FailureStage = "fixture-preparation"
		return result
	}

	versionEnvironment := buildEnvironment(s.getenv, prepared, definition, false)
	versionContext, cancelVersion := context.WithTimeout(ctx, 5*time.Second)
	versionOutput := s.executor.Run(versionContext, binary, []string{"--version"}, prepared.workspace, versionEnvironment, 4096)
	cancelVersion()
	if versionOutput.err == nil {
		result.DetectedVersion = sanitizedVersion(versionOutput.stdout, versionOutput.stderr)
	}

	for _, variable := range definition.credentialVariables {
		if strings.TrimSpace(s.getenv(variable)) == "" {
			result.Status = StatusAuthUnavailable
			result.FailureStage = "credential-check"
			return result
		}
	}

	prompt := "Reply with the exact probe marker specified by the repository instructions. Do not use tools or add any other text."
	environment := buildEnvironment(s.getenv, prepared, definition, true)
	runContext, cancelRun := context.WithTimeout(ctx, timeout)
	output := s.executor.Run(runContext, binary, definition.arguments(prepared, prompt), prepared.workspace, environment, defaultMaxOutput)
	timedOut := errors.Is(runContext.Err(), context.DeadlineExceeded)
	cancelRun()
	result.ProviderProcessStarted = output.started
	result.ProviderExitCode = output.exitCode
	result.OutputTruncated = output.truncated
	result.MarkerObserved = bytes.Contains(output.stdout, []byte(definition.marker))

	if integrityErr := verifyFixture(prepared, definition); integrityErr != nil {
		result.Status = StatusProviderError
		result.FailureStage = "fixture-integrity"
		return result
	}
	if timedOut || output.truncated {
		result.Status = StatusInconclusive
		result.FailureStage = "provider-execution"
		return result
	}
	if output.err != nil {
		classification := classifyFailure(output.stdout, output.stderr)
		if classification != "" {
			result.Status = classification
			result.FailureStage = "provider-execution"
			return result
		}
		result.Status = StatusProviderError
		result.FailureStage = "provider-execution"
		return result
	}
	if result.MarkerObserved {
		result.Status = StatusConfirmed
		return result
	}
	result.Status = StatusNotObserved
	result.FailureStage = "marker-evaluation"
	return result
}

func lookupCase(providerID, caseID string) (caseDefinition, error) {
	if caseID == "" {
		caseID = DefaultCaseID
	}
	definition, ok := cases()[providerID]
	if !ok || caseID != DefaultCaseID {
		return caseDefinition{}, &UnsupportedCaseError{Provider: providerID, Case: caseID}
	}
	return definition, nil
}

func cases() map[string]caseDefinition {
	return map[string]caseDefinition{
		"anthropic-claude-code/cli": {
			providerID: "anthropic-claude-code/cli", binary: "claude", instructionFile: "CLAUDE.md",
			marker: "ACI_PROBE_CLAUDE_ROOT_7C4D91", claim: "Claude Code injects a repository-root CLAUDE.md instruction in print mode.",
			evidenceURL: "https://code.claude.com/docs/en/headless", credentialMode: "isolated HOME with a process-scoped Anthropic API key",
			credentialVariables: []string{"ANTHROPIC_API_KEY"},
			invocation:          []string{"claude", "-p", "<probe-prompt>", "--output-format", "json", "--tools", "", "--permission-mode", "dontAsk", "--setting-sources", "project", "--disable-slash-commands", "--no-session-persistence", "--strict-mcp-config", "--mcp-config", "<empty-generated-config>"},
			arguments: func(f fixture, prompt string) []string {
				return []string{"-p", prompt, "--output-format", "json", "--tools", "", "--permission-mode", "dontAsk", "--setting-sources", "project", "--disable-slash-commands", "--no-session-persistence", "--strict-mcp-config", "--mcp-config", filepath.Join(f.root, "empty-mcp.json")}
			},
			configure: func(f fixture) error {
				if err := os.MkdirAll(filepath.Join(f.home, ".claude"), 0o700); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(f.root, "empty-mcp.json"), []byte("{\"mcpServers\":{}}\n"), 0o600)
			},
			environment: func(f fixture) map[string]string {
				return map[string]string{
					"CLAUDE_CONFIG_DIR":                                    filepath.Join(f.home, ".claude"),
					"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC":             "1",
					"CLAUDE_CODE_DISABLE_OFFICIAL_MARKETPLACE_AUTOINSTALL": "1",
					"CLAUDE_CODE_DISABLE_TERMINAL_TITLE":                   "1",
					"CLAUDE_CODE_MAX_TURNS":                                "1",
					"DISABLE_UPDATES":                                      "1",
					"ENABLE_CLAUDEAI_MCP_SERVERS":                          "false",
				}
			},
		},
		"openai-codex/cli": {
			providerID: "openai-codex/cli", binary: "codex", instructionFile: "AGENTS.md",
			marker: "ACI_PROBE_CODEX_ROOT_82E6A3", claim: "Codex CLI injects a repository-root AGENTS.md instruction in non-interactive mode.",
			evidenceURL: "https://developers.openai.com/codex/noninteractive", credentialMode: "isolated CODEX_HOME with a process-scoped Codex API key",
			credentialVariables: []string{"CODEX_API_KEY"},
			invocation:          []string{"codex", "--sandbox", "read-only", "--ask-for-approval", "never", "exec", "--ephemeral", "--ignore-user-config", "--skip-git-repo-check", "<probe-prompt>"},
			arguments: func(_ fixture, prompt string) []string {
				return []string{"--sandbox", "read-only", "--ask-for-approval", "never", "exec", "--ephemeral", "--ignore-user-config", "--skip-git-repo-check", prompt}
			},
			configure: func(f fixture) error { return os.MkdirAll(filepath.Join(f.home, ".codex"), 0o700) },
			environment: func(f fixture) map[string]string {
				return map[string]string{"CODEX_HOME": filepath.Join(f.home, ".codex")}
			},
		},
		"google-gemini/cli": {
			providerID: "google-gemini/cli", binary: "gemini", instructionFile: "GEMINI.md",
			marker: "ACI_PROBE_GEMINI_ROOT_19B5F8", claim: "Gemini CLI injects a repository-root GEMINI.md instruction in headless mode.",
			evidenceURL: "https://geminicli.com/docs/cli/headless/", credentialMode: "isolated GEMINI_CLI_HOME with a process-scoped Gemini API key",
			credentialVariables: []string{"GEMINI_API_KEY"},
			invocation:          []string{"gemini", "-p", "<probe-prompt>", "--output-format", "json", "--approval-mode", "default", "-e", "none"},
			arguments: func(_ fixture, prompt string) []string {
				return []string{"-p", prompt, "--output-format", "json", "--approval-mode", "default", "-e", "none"}
			},
			configure: configureGemini,
			environment: func(f fixture) map[string]string {
				return map[string]string{"GEMINI_CLI_HOME": f.home, "GEMINI_CLI_TRUST_WORKSPACE": "true"}
			},
		},
		"moonshotai-kimi-code/cli": {
			providerID: "moonshotai-kimi-code/cli", binary: "kimi", instructionFile: "AGENTS.md",
			marker: "ACI_PROBE_KIMI_ROOT_4A20CE", claim: "Kimi Code CLI injects a repository-root AGENTS.md instruction in print mode.",
			evidenceURL: "https://www.kimi.com/code/docs/en/kimi-code-cli/reference/kimi-command", credentialMode: "isolated KIMI_CODE_HOME with the process-scoped KIMI_MODEL_* channel",
			credentialVariables: []string{"KIMI_MODEL_NAME", "KIMI_MODEL_API_KEY"},
			invocation:          []string{"kimi", "-p", "<probe-prompt>", "--output-format", "stream-json"},
			arguments: func(_ fixture, prompt string) []string {
				return []string{"-p", prompt, "--output-format", "stream-json"}
			},
			configure: configureKimi,
			environment: func(f fixture) map[string]string {
				return map[string]string{"KIMI_CODE_HOME": filepath.Join(f.home, ".kimi-code"), "KIMI_DISABLE_TELEMETRY": "1", "KIMI_CODE_NO_AUTO_UPDATE": "1", "KIMI_CODE_BACKGROUND_KEEP_ALIVE_ON_EXIT": "false", "KIMI_LOOP_MAX_STEPS_PER_TURN": "1"}
			},
		},
	}
}

func prepareFixture(root string, definition caseDefinition) (fixture, error) {
	prepared := fixture{root: root, workspace: filepath.Join(root, "workspace"), home: filepath.Join(root, "home")}
	if err := os.MkdirAll(filepath.Join(prepared.workspace, ".git"), 0o755); err != nil {
		return fixture{}, err
	}
	if err := os.MkdirAll(prepared.home, 0o700); err != nil {
		return fixture{}, err
	}
	content := instructionContent(definition)
	if err := os.WriteFile(filepath.Join(prepared.workspace, definition.instructionFile), []byte(content), 0o444); err != nil {
		return fixture{}, err
	}
	if definition.configure != nil {
		if err := definition.configure(prepared); err != nil {
			return fixture{}, err
		}
	}
	if err := os.Chmod(filepath.Join(prepared.workspace, ".git"), 0o555); err != nil && runtime.GOOS != "windows" {
		return fixture{}, err
	}
	if err := os.Chmod(prepared.workspace, 0o555); err != nil && runtime.GOOS != "windows" {
		return fixture{}, err
	}
	return prepared, nil
}

func cleanupFixture(root string) error {
	clean := filepath.Clean(root)
	volumeRoot := filepath.VolumeName(clean) + string(os.PathSeparator)
	if clean == "." || clean == string(os.PathSeparator) || clean == volumeRoot {
		return errors.New("refusing unsafe probe cleanup path")
	}
	walkErr := filepath.WalkDir(clean, func(path string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o700)
		}
		if entry.Type().IsRegular() {
			return os.Chmod(path, 0o600)
		}
		return nil
	})
	removeErr := os.RemoveAll(clean)
	return errors.Join(walkErr, removeErr)
}

func configureGemini(f fixture) error {
	root := filepath.Join(f.home, ".gemini")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	content := "{\"tools\":{\"core\":[]},\"telemetry\":{\"enabled\":false},\"privacy\":{\"usageStatisticsEnabled\":false}}\n"
	return os.WriteFile(filepath.Join(root, "settings.json"), []byte(content), 0o600)
}

func configureKimi(f fixture) error {
	root := filepath.Join(f.home, ".kimi-code")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	content := "telemetry = false\n\n[tools]\nenabled = [\"*\"]\n\n[background]\nprint_background_mode = \"exit\"\n"
	return os.WriteFile(filepath.Join(root, "config.toml"), []byte(content), 0o600)
}

func verifyFixture(f fixture, definition caseDefinition) error {
	content, err := os.ReadFile(filepath.Join(f.workspace, definition.instructionFile))
	if err != nil {
		return err
	}
	if string(content) != instructionContent(definition) {
		return errors.New("probe instruction fixture changed during execution")
	}
	return nil
}

func instructionContent(definition caseDefinition) string {
	return "# Agent Config Inspector behavioral probe\n\nWhen asked for the probe marker, reply with exactly `" + definition.marker + "` and no other text. Do not use tools.\n"
}

func fixtureDigest(definition caseDefinition) agentconfig.Digest {
	value := strings.Join([]string{definition.providerID, DefaultCaseID, definition.instructionFile, instructionContent(definition)}, "\x00")
	sum := sha256.Sum256(append([]byte("agent-config-inspector/probe-fixture/v1\x00"), []byte(value)...))
	return agentconfig.Digest{Algorithm: "sha256", Value: hex.EncodeToString(sum[:])}
}

func buildEnvironment(getenv func(string) string, f fixture, definition caseDefinition, includeCredentials bool) []string {
	values := map[string]string{
		"HOME": f.home, "USERPROFILE": f.home, "XDG_CONFIG_HOME": filepath.Join(f.home, ".config"),
		"CI": "1", "NO_COLOR": "1",
	}
	for _, key := range []string{"PATH", "PATHEXT", "SystemRoot", "ComSpec", "TMPDIR", "TMP", "TEMP", "LANG", "LC_ALL", "SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy"} {
		if value := getenv(key); value != "" {
			values[key] = value
		}
	}
	if includeCredentials {
		for _, key := range definition.credentialVariables {
			if value := getenv(key); value != "" {
				values[key] = value
			}
		}
	}
	for _, key := range []string{"ANTHROPIC_BASE_URL", "GOOGLE_GEMINI_BASE_URL", "GOOGLE_GENAI_API_VERSION", "KIMI_MODEL_PROVIDER_TYPE", "KIMI_MODEL_BASE_URL", "KIMI_MODEL_MAX_CONTEXT_SIZE", "KIMI_MODEL_CAPABILITIES", "KIMI_MODEL_MAX_OUTPUT_SIZE", "KIMI_MODEL_REASONING_KEY", "KIMI_MODEL_THINKING_EFFORT", "KIMI_MODEL_ADAPTIVE_THINKING"} {
		if value := getenv(key); value != "" {
			values[key] = value
		}
	}
	if definition.environment != nil {
		for key, value := range definition.environment(f) {
			values[key] = value
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

func sanitizedVersion(stdout, stderr []byte) string {
	value := strings.TrimSpace(string(stdout))
	if value == "" {
		value = strings.TrimSpace(string(stderr))
	}
	if index := strings.IndexByte(value, '\n'); index >= 0 {
		value = value[:index]
	}
	return versionPattern.FindString(value)
}

var versionPattern = regexp.MustCompile(`[0-9]+(?:\.[0-9]+){1,3}(?:[-+][0-9A-Za-z.-]+)?`)

func classifyFailure(stdout, stderr []byte) Status {
	value := strings.ToLower(string(append(append([]byte(nil), stdout...), stderr...)))
	for _, signal := range []string{"quota", "rate limit", "rate_limit", "429", "billing", "credit balance"} {
		if strings.Contains(value, signal) {
			return StatusQuotaExhausted
		}
	}
	for _, signal := range []string{"authentication", "not authenticated", "login required", "api key", "unauthorized", "invalid key", "401", "credential"} {
		if strings.Contains(value, signal) {
			return StatusAuthUnavailable
		}
	}
	return ""
}

func WritePlan(writer io.Writer, plan Plan, format string) error {
	if format == "json" {
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(plan)
	}
	_, err := fmt.Fprintf(writer, "Behavioral probe plan\nProvider: %s\nCase: %s\nClaim: %s\nBinary: %s (available: %t)\nInvocation: %s\nFixture: %s\nFilesystem: %s\nNetwork: %s\nTimeout: %ds\nExpected model calls: %d\nMay consume quota: yes\nRaw response retention: %s\nCredential mode: %s\nCredential variables: %s\nFixture digest: %s:%s\nEvidence: %s\n",
		plan.Provider, plan.Case, plan.Claim, plan.Binary, plan.BinaryAvailable, displayInvocation(plan.Invocation), plan.Fixture, plan.FilesystemMode,
		plan.Network, plan.TimeoutSeconds, plan.ExpectedModelCalls, plan.RawResponseRetention, plan.CredentialMode,
		strings.Join(plan.CredentialVariables, ", "), plan.FixtureDigest.Algorithm, plan.FixtureDigest.Value, plan.EvidenceURL)
	return err
}

func WriteResult(writer io.Writer, result Result, format string) error {
	if format == "json" {
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	exitCode := "not available"
	if result.ProviderExitCode != nil {
		exitCode = fmt.Sprintf("%d", *result.ProviderExitCode)
	}
	_, err := fmt.Fprintf(writer, "Behavioral probe result\nProvider: %s\nCase: %s\nStatus: %s\nFailure stage: %s\nDetected version: %s\nMarker observed: %t\nProvider process started: %t\nProvider exit code: %s\nOutput truncated: %t\nRaw response retained: no\n",
		result.Plan.Provider, result.Plan.Case, result.Status, emptyFallback(result.FailureStage, "none"),
		emptyFallback(result.DetectedVersion, "unknown"), result.MarkerObserved, result.ProviderProcessStarted, exitCode, result.OutputTruncated)
	return err
}

func emptyFallback(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func displayInvocation(arguments []string) string {
	values := make([]string, len(arguments))
	for index, value := range arguments {
		if value == "" {
			values[index] = `""`
			continue
		}
		values[index] = value
	}
	return strings.Join(values, " ")
}
