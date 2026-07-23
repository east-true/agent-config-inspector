# Provider support matrix

Checked on: 2026-07-24

Support labels describe only the listed capability. They do not imply model-behavior parity.

| Provider ID | Capability | Status | Notes |
|---|---|---|---|
| `anthropic-claude-code/cli` | Root and nested `CLAUDE.md` | Preview | Root-to-target order and local files |
| `anthropic-claude-code/cli` | `.claude/CLAUDE.md` | Preview | Project alternative location |
| `anthropic-claude-code/cli` | `@imports` | Preview | Relative in-workspace imports; four-hop cap; cycle protection |
| `anthropic-claude-code/cli` | `.claude/rules/**/*.md` | Preview | Recursive Markdown discovery and common path globs |
| `anthropic-claude-code/cli` | User instructions | Preview | Explicit opt-in; safe opaque output; external imports not expanded |
| `openai-codex/cli` | Root and nested `AGENTS.md` | Preview | One non-empty source per directory |
| `openai-codex/cli` | `AGENTS.override.md` | Preview | Filename precedence modeled |
| `openai-codex/cli` | Project fallback filenames | Preview | One-line TOML string arrays |
| `openai-codex/cli` | Project instruction byte budget | Preview | Conservative whole-source exclusion at boundary |
| `openai-codex/cli` | User instruction | Preview | Explicit opt-in; `CODEX_HOME` respected; safe opaque output |
| `google-gemini/cli` | Root and target hierarchy | Preview | v0.50.0 memory boundary to selected target; JIT activation predicted |
| `google-gemini/cli` | Configured context filenames | Preview | Project `context.fileName`; configured order plus default |
| `google-gemini/cli` | Memory boundary markers | Preview | Project `context.memoryBoundaryMarkers`; nearest marker wins |
| `google-gemini/cli` | `@imports` | Preview | In-project relative/absolute imports; five-hop cap; cycle protection |
| `google-gemini/cli` | User context | Preview | Explicit opt-in; configured filename inventory; imports not expanded |
| Kimi, Grok, Copilot, others | All | Unsupported | Planned as independent later adapters |

Output and CI capabilities:

| Capability | Status | Notes |
|---|---|---|
| Text and JSON reports | Preview | Safe redaction by default |
| SARIF 2.1.0 | Preview | Repository-relative locations only |
| Repository `pin` and `verify` | Preview | Canonical repository-only lockfile |
| Composite GitHub Action | Preview | Verified release download; optional SARIF upload |

Evidence:

- [Agent Config Inspector Gemini adapter contract](gemini-cli.md)
- [Claude Code memory documentation](https://code.claude.com/docs/en/memory)
- [Codex `AGENTS.md` documentation](https://developers.openai.com/codex/guides/agents-md)
- [Gemini CLI context-file documentation](https://geminicli.com/docs/cli/gemini-md/)
- [Gemini CLI memory-import documentation](https://geminicli.com/docs/reference/memport/)
- [Gemini CLI v0.50.0 settings schema](https://raw.githubusercontent.com/google-gemini/gemini-cli/v0.50.0/schemas/settings.schema.json)
