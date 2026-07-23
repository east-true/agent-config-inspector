package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeExecutor struct {
	available bool
	runs      []fakeRun
	run       func(args []string, directory string, environment []string, maxOutput int) commandOutput
}

type fakeRun struct {
	args        []string
	directory   string
	environment []string
	maxOutput   int
}

func (f *fakeExecutor) LookPath(binary string) (string, error) {
	if !f.available {
		return "", errors.New("not found")
	}
	return "/synthetic/bin/" + binary, nil
}

func (f *fakeExecutor) Run(_ context.Context, _ string, args []string, directory string, environment []string, maxOutput int) commandOutput {
	f.runs = append(f.runs, fakeRun{
		args: append([]string(nil), args...), directory: directory,
		environment: append([]string(nil), environment...), maxOutput: maxOutput,
	})
	if f.run != nil {
		return f.run(args, directory, environment, maxOutput)
	}
	return commandOutput{started: true}
}

func TestPlansAreBoundedAndCredentialFree(t *testing.T) {
	service := New()
	service.executor = &fakeExecutor{available: true}
	for providerID, definition := range cases() {
		t.Run(providerID, func(t *testing.T) {
			plan, err := service.Plan(providerID, DefaultCaseID, 90*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if !plan.BinaryAvailable || plan.ExpectedModelCalls != 1 || !plan.MayConsumeQuota || plan.TimeoutSeconds != 90 {
				t.Fatalf("unexpected plan: %+v", plan)
			}
			if plan.FixtureDigest.Algorithm != "sha256" || len(plan.FixtureDigest.Value) != 64 {
				t.Fatalf("unexpected fixture digest: %+v", plan.FixtureDigest)
			}
			encoded, err := json.Marshal(plan)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(encoded, []byte(definition.marker)) || bytes.Contains(encoded, []byte("secret-value")) {
				t.Fatalf("plan exposes runtime-only data: %s", encoded)
			}
		})
	}
}

func TestExecuteConfirmsOnlyExactCaseMarker(t *testing.T) {
	for providerID, definition := range cases() {
		t.Run(providerID, func(t *testing.T) {
			executor := &fakeExecutor{available: true}
			executor.run = func(args []string, _ string, _ []string, _ int) commandOutput {
				if len(args) == 1 && args[0] == "--version" {
					return commandOutput{stdout: []byte("synthetic-cli 1.2.3\n"), started: true}
				}
				exitCode := 0
				return commandOutput{stdout: []byte(`{"response":"` + definition.marker + `"}`), exitCode: &exitCode, started: true}
			}
			service := testService(executor, credentialEnvironment(definition))
			result := service.Execute(context.Background(), providerID, DefaultCaseID, time.Minute)
			if result.Status != StatusConfirmed || !result.MarkerObserved || !result.ProviderProcessStarted {
				t.Fatalf("unexpected result: %+v", result)
			}
			if result.DetectedVersion != "1.2.3" || len(executor.runs) != 2 {
				t.Fatalf("version/runs mismatch: %+v, runs=%d", result, len(executor.runs))
			}
		})
	}
}

func TestExecuteDoesNotInheritUnrelatedEnvironment(t *testing.T) {
	definition := cases()["openai-codex/cli"]
	executor := &fakeExecutor{available: true}
	executor.run = func(args []string, _ string, environment []string, _ int) commandOutput {
		if len(args) == 1 && args[0] == "--version" {
			if strings.Contains(strings.Join(environment, "\n"), "CODEX_API_KEY=") {
				t.Fatal("version detection received a model credential")
			}
			return commandOutput{stdout: []byte("codex-cli 1.0.0"), started: true}
		}
		joined := strings.Join(environment, "\n")
		if strings.Contains(joined, "UNRELATED_SECRET=") || !strings.Contains(joined, "CODEX_API_KEY=synthetic-credential") {
			t.Fatalf("unsafe environment: %s", joined)
		}
		return commandOutput{stdout: []byte(definition.marker), started: true}
	}
	values := credentialEnvironment(definition)
	values["UNRELATED_SECRET"] = "must-not-cross-boundary"
	service := testService(executor, values)
	if result := service.Execute(context.Background(), definition.providerID, DefaultCaseID, time.Minute); result.Status != StatusConfirmed {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExecuteClassifiesPreflightAndProviderFailures(t *testing.T) {
	t.Run("binary unavailable", func(t *testing.T) {
		executor := &fakeExecutor{}
		service := testService(executor, nil)
		result := service.Execute(context.Background(), "openai-codex/cli", DefaultCaseID, time.Minute)
		if result.Status != StatusBlockedBeforeModelCall || result.FailureStage != "binary-detection" || len(executor.runs) != 0 {
			t.Fatalf("unexpected result: %+v", result)
		}
	})

	t.Run("credential unavailable", func(t *testing.T) {
		executor := &fakeExecutor{available: true, run: func(_ []string, _ string, _ []string, _ int) commandOutput {
			return commandOutput{stdout: []byte("codex-cli 1.0.0"), started: true}
		}}
		service := testService(executor, nil)
		result := service.Execute(context.Background(), "openai-codex/cli", DefaultCaseID, time.Minute)
		if result.Status != StatusAuthUnavailable || result.FailureStage != "credential-check" || result.ProviderProcessStarted || len(executor.runs) != 1 {
			t.Fatalf("unexpected result: %+v, runs=%d", result, len(executor.runs))
		}
	})

	t.Run("quota response", func(t *testing.T) {
		definition := cases()["openai-codex/cli"]
		executor := &fakeExecutor{available: true}
		executor.run = func(args []string, _ string, _ []string, _ int) commandOutput {
			if len(args) == 1 {
				return commandOutput{stdout: []byte("codex-cli 1.0.0"), started: true}
			}
			exitCode := 1
			return commandOutput{stderr: []byte("quota exhausted: private-provider-response"), exitCode: &exitCode, err: errors.New("exit status 1"), started: true}
		}
		service := testService(executor, credentialEnvironment(definition))
		result := service.Execute(context.Background(), definition.providerID, DefaultCaseID, time.Minute)
		if result.Status != StatusQuotaExhausted || result.FailureStage != "provider-execution" {
			t.Fatalf("unexpected result: %+v", result)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(encoded, []byte("private-provider-response")) || bytes.Contains(encoded, []byte("synthetic-credential")) {
			t.Fatalf("result retained sensitive provider data: %s", encoded)
		}
	})

	t.Run("marker not observed", func(t *testing.T) {
		definition := cases()["openai-codex/cli"]
		executor := &fakeExecutor{available: true, run: func(args []string, _ string, _ []string, _ int) commandOutput {
			if len(args) == 1 {
				return commandOutput{stdout: []byte("codex-cli 1.0.0"), started: true}
			}
			return commandOutput{stdout: []byte("a different answer"), started: true}
		}}
		service := testService(executor, credentialEnvironment(definition))
		result := service.Execute(context.Background(), definition.providerID, DefaultCaseID, time.Minute)
		if result.Status != StatusNotObserved || result.FailureStage != "marker-evaluation" {
			t.Fatalf("unexpected result: %+v", result)
		}
	})

	t.Run("marker in failed output is not confirmation", func(t *testing.T) {
		definition := cases()["openai-codex/cli"]
		executor := &fakeExecutor{available: true, run: func(args []string, _ string, _ []string, _ int) commandOutput {
			if len(args) == 1 {
				return commandOutput{stdout: []byte("codex-cli 1.0.0"), started: true}
			}
			return commandOutput{stdout: []byte(definition.marker), err: errors.New("provider failed"), started: true}
		}}
		service := testService(executor, credentialEnvironment(definition))
		result := service.Execute(context.Background(), definition.providerID, DefaultCaseID, time.Minute)
		if result.Status != StatusProviderError || !result.MarkerObserved {
			t.Fatalf("unexpected result: %+v", result)
		}
	})
}

func TestBoundedBufferDiscardsExcess(t *testing.T) {
	buffer := &boundedBuffer{limit: 4}
	if count, err := buffer.Write([]byte("abcdef")); err != nil || count != 6 {
		t.Fatalf("write = %d, %v", count, err)
	}
	if string(buffer.Bytes()) != "abcd" || !buffer.truncated {
		t.Fatalf("unexpected buffer: %q truncated=%t", buffer.Bytes(), buffer.truncated)
	}
}

func TestSanitizedVersionRetainsOnlyVersionToken(t *testing.T) {
	value := sanitizedVersion([]byte("wrapper /home/example/private 2.1.217 credential-value\nextra"), nil)
	if value != "2.1.217" {
		t.Fatalf("version = %q", value)
	}
}

func TestGeneratedProviderConfigurationKeepsToolsUnavailable(t *testing.T) {
	for _, providerID := range []string{"google-gemini/cli", "moonshotai-kimi-code/cli"} {
		t.Run(providerID, func(t *testing.T) {
			definition := cases()[providerID]
			root := t.TempDir()
			defer cleanupFixture(root)
			prepared, err := prepareFixture(root, definition)
			if err != nil {
				t.Fatal(err)
			}
			var path, required string
			switch providerID {
			case "google-gemini/cli":
				path = filepath.Join(prepared.home, ".gemini", "settings.json")
				required = `"core":[]`
			case "moonshotai-kimi-code/cli":
				path = filepath.Join(prepared.home, ".kimi-code", "config.toml")
				required = `enabled = ["*"]`
			}
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(content), required) || strings.Contains(string(content), "synthetic-credential") {
				t.Fatalf("unsafe provider configuration: %s", content)
			}
		})
	}
}

func TestUnsupportedCase(t *testing.T) {
	service := New()
	if _, err := service.Plan("openai-codex/cli", "nested-precedence", time.Minute); err == nil {
		t.Fatal("expected unsupported case error")
	}
}

func TestExecuteCleansReadOnlyFixtureAndSurfacesCleanupFailure(t *testing.T) {
	definition := cases()["openai-codex/cli"]
	executor := &fakeExecutor{available: true, run: func(args []string, _ string, _ []string, _ int) commandOutput {
		if len(args) == 1 {
			return commandOutput{stdout: []byte("codex-cli 1.0.0"), started: true}
		}
		return commandOutput{stdout: []byte(definition.marker), started: true}
	}}

	t.Run("cleanup succeeds", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "probe-root")
		service := testService(executor, credentialEnvironment(definition))
		service.tempDir = func(string, string) (string, error) { return root, nil }
		result := service.Execute(context.Background(), definition.providerID, DefaultCaseID, time.Minute)
		if result.Status != StatusConfirmed {
			t.Fatalf("unexpected result: %+v", result)
		}
		if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("fixture remains after cleanup: %v", err)
		}
	})

	t.Run("cleanup failure overrides confirmation", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "probe-root")
		service := testService(executor, credentialEnvironment(definition))
		service.tempDir = func(string, string) (string, error) { return root, nil }
		service.removeAll = func(path string) error {
			_ = cleanupFixture(path)
			return errors.New("synthetic cleanup failure")
		}
		result := service.Execute(context.Background(), definition.providerID, DefaultCaseID, time.Minute)
		if result.Status != StatusProviderError || result.FailureStage != "fixture-cleanup" {
			t.Fatalf("unexpected result: %+v", result)
		}
	})
}

func testService(executor executor, environment map[string]string) *Service {
	return &Service{
		executor: executor,
		getenv: func(key string) string {
			return environment[key]
		},
		tempDir:   os.MkdirTemp,
		removeAll: cleanupFixture,
	}
}

func credentialEnvironment(definition caseDefinition) map[string]string {
	result := map[string]string{"PATH": "/synthetic/bin"}
	for _, variable := range definition.credentialVariables {
		result[variable] = "synthetic-credential"
	}
	return result
}
