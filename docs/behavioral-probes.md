# Behavioral probes

Behavioral probes are opt-in checks that ask an installed provider CLI whether it actually loaded one synthetic repository instruction. They are separate from the offline scanner: `scan`, `explain`, `diff`, `pin`, and `verify` never execute this path.

## Safety contract

Running `probe` without execution flags only inspects whether the named executable is on `PATH` and prints a plan. It does not create a fixture, read credentials, start a provider process, use the network, or consume model quota.

```bash
agent-config-inspector probe codex
agent-config-inspector probe kimi --format json
```

Actual execution requires two independent flags:

```bash
agent-config-inspector probe codex \
  --execute \
  --acknowledge-quota
```

The CLI prints the plan to standard error before it checks the acknowledgement. Review it before adding `--acknowledge-quota`. An execution:

- creates a new temporary home and a minimal temporary Git workspace;
- writes one provider-specific instruction containing a synthetic marker;
- makes the fixture workspace read-only before starting the provider;
- uses a provider-documented non-interactive mode;
- disables tools, or combines a tool allowlist with non-interactive denial and a read-only workspace;
- disables extensions or MCP configuration where the provider exposes a supported switch;
- passes only a small environment allowlist and the documented process-scoped credential variables;
- expects one model response and stops at the configured timeout;
- bounds captured stdout and stderr to 1 MiB each, uses them only for local classification, and discards them;
- emits the terminal status, marker match, detected CLI version, and failure stage—not the response body or credential values.

The generated fixture is the only repository the probe opens intentionally. Provider-managed policy can still change startup behavior, and the provider itself needs network access to its API. Run probes in a clean environment when system policy, wrappers, or managed hooks are not understood.

## Evidence registry

The only implemented case is `root-instruction-discovery`. A success confirms that single claim for the detected installed CLI version; it does not confirm nested precedence, imports, overrides, tool behavior, or model compliance in general.

| Provider | Synthetic source | Non-interactive boundary | Process credential | Tool/config boundary | Contract evidence |
|---|---|---|---|---|---|
| Claude Code | `CLAUDE.md` | `claude -p`, JSON output, `dontAsk` | `ANTHROPIC_API_KEY` | empty tool list, project-only settings, strict empty MCP config, no session persistence, nonessential traffic disabled, isolated config dir | [Headless mode](https://code.claude.com/docs/en/headless), [CLI reference](https://code.claude.com/docs/en/cli-usage), [environment variables](https://code.claude.com/docs/en/env-vars) |
| Codex CLI | `AGENTS.md` | `codex exec --ephemeral`, approval `never` | `CODEX_API_KEY` | read-only sandbox, ignored user config, isolated `CODEX_HOME` | [Non-interactive mode](https://developers.openai.com/codex/noninteractive), [`AGENTS.md` guide](https://developers.openai.com/codex/guides/agents-md) |
| Gemini CLI | `GEMINI.md` | headless prompt, JSON output, default approval | `GEMINI_API_KEY` | empty built-in-tool allowlist, extensions disabled, isolated `GEMINI_CLI_HOME` | [Headless mode](https://geminicli.com/docs/cli/headless/), [Configuration](https://geminicli.com/docs/reference/configuration/) |
| Kimi Code CLI | `AGENTS.md` | print mode, stream JSON | `KIMI_MODEL_NAME`, `KIMI_MODEL_API_KEY` | documented non-matching tool allowlist, telemetry/update disabled, isolated `KIMI_CODE_HOME` | [`kimi` command](https://www.kimi.com/code/docs/en/kimi-code-cli/reference/kimi-command), [Configuration files](https://www.kimi.com/code/docs/en/kimi-code-cli/configuration/config-files), [Environment variables](https://www.kimi.com/code/docs/en/kimi-code-cli/configuration/env-vars.html) |

Grok is deliberately skipped and remains unsupported. Adding a static adapter and adding a behavioral probe are independent decisions.

## Credentials

The probe does not copy a provider login directory, cached OAuth session, shell environment, or full home directory. It accepts only these documented ephemeral channels:

```text
Claude: ANTHROPIC_API_KEY
Codex:  CODEX_API_KEY
Gemini: GEMINI_API_KEY
Kimi:   KIMI_MODEL_NAME + KIMI_MODEL_API_KEY
```

Supply values through a local secret manager or a protected CI environment. Do not put keys in command arguments, fixture files, committed `.env` files, workflow inputs, issue comments, or probe artifacts. A missing variable returns `auth-unavailable` before the model command starts. The local `--version` command may already have run, but it does not make the probe model request.

## Results and exit codes

| Status | Meaning | Exit code |
|---|---|---:|
| `confirmed` | The provider stdout contained the case marker | 0 |
| `not-observed` | The command completed without the marker | 1 |
| `blocked-before-model-call` | Binary or fixture preparation failed | 3 |
| `auth-unavailable` | Required process credential was missing or rejected | 3 |
| `quota-exhausted` | Output indicated quota, rate-limit, credit, or billing exhaustion | 3 |
| `provider-error` | The provider failed, fixture integrity changed, or safe fixture cleanup failed | 3 |
| `inconclusive` | The run timed out or output exceeded its bound | 3 |

Authentication and quota failures are not evidence against instruction discovery. `not-observed` is also deliberately narrow: it applies only to the named case and detected runtime.

Use JSON when recording a sanitized result:

```bash
agent-config-inspector probe claude \
  --execute \
  --acknowledge-quota \
  --timeout 2m \
  --format json
```

The output contract is [schemas/probe.schema.json](../schemas/probe.schema.json). Results contain no timestamps or host identity. Compare the detected version and fixture digest before combining results from different machines.

## Manual conformance workflow

The repository includes a `workflow_dispatch`-only workflow named **Behavioral probe conformance**. Its default `execute=false` mode builds the current revision and prints the plan without secrets. Actual execution selects exactly one provider, requires the protected `behavioral-probes` GitHub Environment, and exposes only that provider's repository secret to the command.

Configure only the secret needed by the selected provider:

- `ANTHROPIC_API_KEY`
- `CODEX_API_KEY`
- `GEMINI_API_KEY`
- `KIMI_MODEL_API_KEY`, plus non-secret environment variable `KIMI_MODEL_NAME`

Do not enable the execution job for pull requests or unreviewed revisions. The workflow does not upload probe output as an artifact.
