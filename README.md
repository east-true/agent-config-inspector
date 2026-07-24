# Agent Config Inspector

[![CI](https://github.com/east-true/agent-config-inspector/actions/workflows/ci.yml/badge.svg)](https://github.com/east-true/agent-config-inspector/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/east-true/agent-config-inspector)](https://github.com/east-true/agent-config-inspector/releases/latest)
[![License](https://img.shields.io/github/license/east-true/agent-config-inspector)](LICENSE)

See which repository instructions coding agents are predicted to receive—for a specific file, with provenance—before configuration drift reaches an agent session or CI.

Agent Config Inspector is a local, offline CLI for Claude Code, Codex CLI, Gemini CLI, Kimi Code CLI, and GitHub Copilot CLI. It discovers each provider's applicable instruction files, explains why they apply, and compares the resulting instruction graphs without executing repository code or exposing instruction text.

> **Developer preview:** results are static predictions of instruction discovery, not proof that a model will follow an instruction.

## Why use it?

A repository can contain `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, imports, nested rules, and path-specific instructions. Each coding agent resolves them differently, so agents working on the same file may receive different guidance.

Agent Config Inspector makes those differences visible:

- discover which instruction sources apply to a target path;
- explain why each source was included, excluded, or overridden;
- compare the predicted instruction graphs of multiple providers;
- detect repository instruction drift in pull requests;
- inventory repository-owned skills, custom agents, and MCP declarations.

## Quick start

Go 1.25 or newer is required. The core currently has no third-party Go module dependencies.

```bash
git clone https://github.com/east-true/agent-config-inspector.git
cd agent-config-inspector
go build -o bin/agent-config-inspector ./cmd/agent-config-inspector
```

Scan a repository for Claude Code and Codex CLI instructions that apply to one file:

```bash
./bin/agent-config-inspector scan /path/to/repository \
  --providers claude,codex \
  --target backend/src/users.go
```

Omit `--providers` and `--target` to scan every supported provider at the repository root.

If the result is `predicted-empty`, the report lists the provider-specific instruction locations it checked. README files, source code, and application runtime directories are intentionally outside this instruction scan.

Continue with the [Getting started guide](docs/getting-started.md) for result interpretation, `explain`, `diff`, and workspace labels. Prebuilt Linux and macOS archives with SHA-256 checksums are available on the [Releases page](https://github.com/east-true/agent-config-inspector/releases/latest).

## Supported providers

All current adapters are preview-quality and model only the named CLI surface. Similar filenames in another product do not imply compatibility.

| Alias | Provider ID | Surface | Repository instruction baseline |
|---|---|---|---|
| `claude` | `anthropic-claude-code/cli` | Claude Code | `CLAUDE.md`, local memory, rules, imports |
| `codex` | `openai-codex/cli` | Codex CLI | `AGENTS.md`, overrides, fallbacks, byte budget |
| `gemini` | `google-gemini/cli` | Gemini CLI | `GEMINI.md`, configured context files, boundaries, imports |
| `kimi` | `moonshotai-kimi-code/cli` | Kimi Code CLI | branded and generic instruction hierarchy, size guidance |
| `copilot` | `github-copilot/cli` | GitHub Copilot CLI | standard, compatible, and path-specific instructions |

Grok is deliberately unsupported. Copilot coding agent, code review, IDE integrations, and other adjacent products are separate surfaces and are not inferred from Copilot CLI behavior. See the capability-level [support matrix](docs/support-matrix.md) for exact versions, limits, and evidence.

## Safe by default

- Local, offline, read-only repository scans.
- No provider CLI, hook, build script, MCP server, or repository command execution.
- No instruction text or absolute workspace paths in reports.
- No user-level instructions unless explicitly requested, with opaque output when enabled.
- No workspace escape or external import traversal by default.
- Deterministic repository-only snapshots and GitHub-compatible SARIF.

Read [Privacy](docs/privacy.md), [Limitations](docs/limitations.md), and the [Security policy](SECURITY.md) before expanding scan authority or publishing a report that includes user context.

## Documentation

The [`docs/` index](docs/index.md) is the entry point for versioned project documentation.

| Start here | Purpose |
|---|---|
| [Getting started](docs/getting-started.md) | Build, scan, explain, compare, and interpret results |
| [CLI reference](docs/cli-reference.md) | Commands, shared options, formats, and exit codes |
| [CI integration](docs/ci-integration.md) | Snapshots, SARIF, and the GitHub Action |
| [Support matrix](docs/support-matrix.md) | Provider capabilities, versions, limits, and evidence |
| [Initial design](docs/initial-design.md) | Architecture, boundaries, and roadmap |

## Contributing

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) before adding a provider, changing a schema, or modifying discovery behavior.

```bash
go test ./...
go vet ./...
go build ./cmd/agent-config-inspector
go run ./cmd/agent-config-inspector verify .
git diff --check
```

## License

Apache License 2.0. See [LICENSE](LICENSE).
