# Agent Config Inspector

Agent Config Inspector is an offline CLI that predicts which repository instructions Claude Code, Codex CLI, Gemini CLI, Kimi Code CLI, and GitHub Copilot CLI receive for a target path, then explains configuration drift without executing repository code.

> Status: developer preview. The output is a static prediction of instruction discovery, not proof that a model will follow an instruction.

## Why this exists

Coding agents use different filenames, hierarchy rules, imports, and path scopes. A repository can therefore look consistently configured while its agents receive different guidance. Agent Config Inspector makes that difference visible with provenance and conservative findings.

The current source registry contains:

- `anthropic-claude-code/cli` (`claude` alias)
- `github-copilot/cli` (`copilot` alias)
- `google-gemini/cli` (`gemini` alias)
- `moonshotai-kimi-code/cli` (`kimi` alias)
- `openai-codex/cli` (`codex` alias)

Grok has been deliberately skipped. Copilot coding agent, code review, VS Code, and other agents remain separate planned adapters; requests for those surfaces fail as unsupported instead of inheriting CLI behavior.

## Current capabilities

- Resolves root and nested `CLAUDE.md`, `CLAUDE.local.md`, `.claude/CLAUDE.md`, recursive `.claude/rules/**/*.md`, path globs, and bounded `@imports`.
- Resolves root and nested `AGENTS.override.md`, `AGENTS.md`, Codex fallback filenames, and the combined project instruction byte budget.
- Resolves Gemini CLI v0.50.0 hierarchical context, configured context filenames, memory boundaries, target-specific JIT context, and bounded `@imports`.
- Resolves Kimi Code CLI 0.29.0 user and project instruction hierarchy, branded files, lowercase fallback, and soft 32 KiB guidance.
- Resolves Copilot CLI v1.0.73 standard instruction locations, compatible agent files, path-specific `applyTo` globs, identical-source deduplication, and bounded supported imports.
- Inventories repository-owned Claude Code and Codex CLI Agent Skills for a selected path without exposing skill descriptions or bodies.
- Compares normalized instruction units without an LLM or network call.
- Explains included and excluded sources, precedence, evidence, token estimates, and confidence.
- Rejects workspace escapes and external imports by default.
- Hides instruction contents and absolute workspace paths in text and JSON output.
- Reads known user-level instruction locations only with `--include-user-context`, then redacts path, content, digest, size, and token estimates.
- Pins deterministic repository-only lockfiles and verifies pull-request drift.
- Emits GitHub-compatible SARIF 2.1.0 without external or user-source locations.
- Offers an opt-in, generated-fixture behavioral probe for one root-discovery claim for each of the four adapters released through v0.4.0; Copilot CLI probing is not yet supported.

## Build

Go 1.25 or newer is required.

```bash
git clone git@github.com:east-true/agent-config-inspector.git
cd agent-config-inspector
go build -o bin/agent-config-inspector ./cmd/agent-config-inspector
```

No third-party Go module is required for the current core.

## Quick start

Scan the current repository for all supported providers:

```bash
./bin/agent-config-inspector scan .
```

Explain Codex resolution for one file:

```bash
./bin/agent-config-inspector explain . \
  --provider codex \
  --target backend/src/users.go
```

Compare Claude Code and Codex CLI as JSON:

```bash
./bin/agent-config-inspector diff . \
  --providers claude,codex \
  --target backend/src/users.go \
  --format json
```

Inventory repository skills available to Claude Code and Codex CLI:

```bash
./bin/agent-config-inspector inventory skills . \
  --providers claude,codex \
  --target backend/src/users.go
```

This command inspects only repository-owned `.claude/skills` and `.agents/skills` directories. It does not read user skills or execute skill scripts. See [Skills inventory](docs/skills-inventory.md).

Fail CI when a warning or error is found:

```bash
./bin/agent-config-inspector scan . --fail-on warning
```

Pin the current repository-owned instruction state and verify it later:

```bash
./bin/agent-config-inspector pin .
./bin/agent-config-inspector verify .
```

The default lockfile is `agent-config-inspector.lock.json`. It contains repository-relative paths and digests only. See the [snapshot format](docs/snapshot-format.md).

Generate SARIF locally:

```bash
./bin/agent-config-inspector verify . --format sarif > agent-config-inspector.sarif
```

Use the composite GitHub Action after checking out the repository:

```yaml
permissions:
  contents: read
  security-events: write

steps:
  - uses: actions/checkout@v7.0.1
  - uses: east-true/agent-config-inspector@v0.6.0
    with:
      command: verify
      snapshot: agent-config-inspector.lock.json
      fail-on: warning
      version: v0.6.0
      upload-sarif: "true"
```

The Action downloads the selected GitHub Release, verifies its SHA-256 entry from `checksums.txt`, and then runs it. Set the `version` input to an exact release tag when a workflow requires a fixed binary.

Inspect the exact provider registry:

```bash
./bin/agent-config-inspector providers list
./bin/agent-config-inspector providers show kimi
```

Preview a behavioral probe without starting a provider CLI or making a model request:

```bash
./bin/agent-config-inspector probe codex
```

Actual execution requires both `--execute` and `--acknowledge-quota`, uses a documented process-scoped API credential, and may consume quota. Read [Behavioral probes](docs/behavioral-probes.md) before running one.

## Exit codes

| Code | Meaning |
|---:|---|
| 0 | Completed below the configured finding threshold |
| 1 | A finding reached `--fail-on` |
| 2 | Invalid CLI usage or configuration |
| 3 | Internal error or incomplete result |
| 4 | Unsupported provider, surface, version, or probe case |
| 5 | Safety policy refused the request |

Warnings do not fail by default. Use `--fail-on warning` for a stricter CI policy or `--fail-on never` for an informational run.

## Privacy model

The default scan is local, offline, read-only, repository-scoped, and does not execute provider CLIs, hooks, build scripts, MCP servers, or repository commands. Output contains metadata and content digests for repository-owned sources, never instruction text.

User-level instructions are excluded unless `--include-user-context` is supplied. In that mode, only documented user instruction locations are inventoried and output identifiers remain opaque. See [Privacy](docs/privacy.md) and [Security policy](SECURITY.md) before publishing a report produced with local context.

`pin` and `verify` deliberately refuse `--include-user-context`. A commit-ready lockfile cannot represent user-source existence, paths, content, fingerprints, or token counts.

`probe` is a separate opt-in network path. Its default plan mode does not start a provider or read credentials. Explicit execution uses a synthetic temporary workspace and isolated home, passes only an allowlisted environment, and discards bounded provider output after marker/failure classification. It never probes the selected repository.

## Accuracy boundary

Adapters are based on current official discovery documentation and deliberately report their checked date. Runtime flags, trust decisions, managed policy, version drift, and agent behavior can change the actual result. See the [support matrix](docs/support-matrix.md) and [limitations](docs/limitations.md).

Primary semantics references:

- [Claude Code memory and instruction discovery](https://code.claude.com/docs/en/memory)
- [Codex `AGENTS.md` guide](https://developers.openai.com/codex/guides/agents-md)
- [Gemini CLI adapter contract](docs/gemini-cli.md)
- [Gemini CLI context files](https://geminicli.com/docs/cli/gemini-md/)
- [Gemini CLI memory import processor](https://geminicli.com/docs/reference/memport/)
- [Kimi Code CLI adapter contract](docs/kimi-code-cli.md)
- [Kimi Code CLI 0.29.0 instruction loader](https://github.com/MoonshotAI/kimi-code/blob/%40moonshot-ai%2Fkimi-code%400.29.0/packages/agent-core/src/profile/context.ts)
- [Copilot CLI adapter contract](docs/copilot-cli.md)
- [Copilot CLI custom instructions](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-custom-instructions)
- [Behavioral probe contract and evidence registry](docs/behavioral-probes.md)
- [Agent Skills inventory contract](docs/skills-inventory.md)

## Development

```bash
go test ./...
go vet ./...
go build ./cmd/agent-config-inspector
git diff --check
```

The detailed architecture and roadmap are in [docs/initial-design.md](docs/initial-design.md). Contributions are welcome; read [CONTRIBUTING.md](CONTRIBUTING.md) first.

## License

Apache License 2.0. See [LICENSE](LICENSE).
