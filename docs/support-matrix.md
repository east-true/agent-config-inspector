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
| Gemini, Kimi, Grok, Copilot, others | All | Unsupported | Planned as independent later adapters |

Evidence:

- [Claude Code memory documentation](https://code.claude.com/docs/en/memory)
- [Codex `AGENTS.md` documentation](https://developers.openai.com/codex/guides/agents-md)
