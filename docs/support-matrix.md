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
| `moonshotai-kimi-code/cli` | Project instruction hierarchy | Preview | 0.29.0 nearest Git root to selected target |
| `moonshotai-kimi-code/cli` | `.kimi-code/AGENTS.md` | Preview | Loaded before the generic file at each directory |
| `moonshotai-kimi-code/cli` | `AGENTS.md` / `agents.md` | Preview | First non-empty generic candidate; uppercase first |
| `moonshotai-kimi-code/cli` | User instructions | Preview | Explicit opt-in; `KIMI_CODE_HOME` and generic `.agents` scope; opaque output |
| `moonshotai-kimi-code/cli` | 32 KiB guidance | Preview | Full content retained; warning threshold modeled conservatively |
| `moonshotai-kimi-code/cli` | Custom agents and `SYSTEM.md` | Unsupported | Runtime selection can replace default prompt injection |
| `github-copilot/cli` | Standard repository and agent instructions | Preview | v1.0.73; root-to-target `.github/copilot-instructions.md`, `AGENTS.md`, `CLAUDE.md`, `.claude/CLAUDE.md`, and `GEMINI.md` |
| `github-copilot/cli` | Path-specific instructions | Preview | Recursive `*.instructions.md` at project-root and target-nested standard locations |
| `github-copilot/cli` | `applyTo` glob matching | Preview | Comma-separated documented glob forms; target-relative evaluation |
| `github-copilot/cli` | Supported `@imports` | Preview | Repository-contained relative imports for Copilot, AGENTS, and Claude instruction families; five-hop cap |
| `github-copilot/cli` | Identical general-source deduplication | Preview | Exact-byte duplicates removed; deterministic output order is not precedence |
| `github-copilot/cli` | User instructions | Preview | Explicit opt-in under `COPILOT_HOME`; recursive modular instructions; opaque output; imports not expanded |
| `github-copilot/cli` | `COPILOT_CUSTOM_INSTRUCTIONS_DIRS` and `/instructions` state | Unsupported | Arbitrary external directories and runtime enable/disable state are not read |
| `github-copilot/cloud-agent` | All | Unsupported | Independent cloud execution surface; CLI behavior is not inferred |
| `github-copilot/code-review` | All | Unsupported | Independent review surface; CLI behavior is not inferred |
| `github-copilot/vscode` | All | Unsupported | Independent IDE surface; CLI behavior is not inferred |
| Grok | All | Unsupported | Deliberately skipped; no compatibility is inferred |
| Other providers | All | Unsupported | Planned only as independent later adapters |

Behavioral evidence capabilities:

| Provider ID | Case | Status | Notes |
|---|---|---|---|
| `anthropic-claude-code/cli` | Root `CLAUDE.md` discovery | Opt-in probe | Generated fixture; process API key; tools and MCP disabled |
| `openai-codex/cli` | Root `AGENTS.md` discovery | Opt-in probe | Generated fixture; ephemeral exec; read-only sandbox |
| `google-gemini/cli` | Root `GEMINI.md` discovery | Opt-in probe | Generated fixture; empty core-tool allowlist; extensions disabled |
| `moonshotai-kimi-code/cli` | Root `AGENTS.md` discovery | Opt-in probe | Generated fixture; isolated config; tools disabled |
| `github-copilot/cli` | Any behavioral case | Unsupported | Static adapter only; no Copilot process is started |
| Grok, other providers | Any behavioral case | Unsupported | No provider process is started |

Configuration inventory capabilities:

| Provider ID | Surface | Status | Notes |
|---|---|---|---|
| `anthropic-claude-code/cli` | Repository Agent Skills | Preview | Target hierarchy `.claude/skills/*/SKILL.md`; directory invocation name; descriptions and bodies hidden |
| `openai-codex/cli` | Repository Agent Skills | Preview | Target hierarchy `.agents/skills/*/SKILL.md`; Agent Skills name/description baseline validation; descriptions and bodies hidden |
| Claude Code and Codex CLI | User, admin, system, and plugin skills | Unsupported | Repository-only Phase 8a boundary |
| Gemini, Kimi, Copilot, Grok, other providers | Agent Skills | Unsupported | No cross-provider compatibility is inferred |
| All providers | Skill activation and script execution | Unsupported | Inventory never predicts task matching or runs bundled resources |

An opt-in probe is an available measurement mechanism, not a claim that a case has been confirmed for every installed version. Only a successful result with its detected version and fixture digest is `confirmed`.

Output and CI capabilities:

| Capability | Status | Notes |
|---|---|---|
| Text and JSON reports | Preview | Safe redaction by default |
| SARIF 2.1.0 | Preview | Repository-relative locations only |
| Repository `pin` and `verify` | Preview | Canonical repository-only lockfile |
| Composite GitHub Action | Preview | Verified release download; optional SARIF upload |
| Behavioral probe plan/result JSON | Preview | Schema v1; raw provider response and credentials omitted |
| Skills inventory text/JSON | Preview | Independent schema v1; repository-relative paths; metadata content hidden |

Evidence:

- [Agent Config Inspector Gemini adapter contract](gemini-cli.md)
- [Claude Code memory documentation](https://code.claude.com/docs/en/memory)
- [Codex `AGENTS.md` documentation](https://developers.openai.com/codex/guides/agents-md)
- [Gemini CLI context-file documentation](https://geminicli.com/docs/cli/gemini-md/)
- [Gemini CLI memory-import documentation](https://geminicli.com/docs/reference/memport/)
- [Gemini CLI v0.50.0 settings schema](https://raw.githubusercontent.com/google-gemini/gemini-cli/v0.50.0/schemas/settings.schema.json)
- [Agent Config Inspector Kimi adapter contract](kimi-code-cli.md)
- [Kimi Code CLI 0.29.0 instruction loader](https://github.com/MoonshotAI/kimi-code/blob/%40moonshot-ai%2Fkimi-code%400.29.0/packages/agent-core/src/profile/context.ts)
- [Kimi Code CLI agents and instruction files](https://www.kimi.com/code/docs/en/kimi-code-cli/customization/agents.html)
- [Agent Config Inspector Copilot CLI adapter contract](copilot-cli.md)
- [GitHub Copilot CLI custom instructions](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-custom-instructions)
- [GitHub Copilot custom-instruction surface support](https://docs.github.com/en/copilot/reference/custom-instructions-support)
- [Behavioral probe contract and evidence registry](behavioral-probes.md)
- [Agent Skills inventory contract](skills-inventory.md)
- [Agent Skills specification](https://agentskills.io/specification)
- [Claude Code skills](https://code.claude.com/docs/en/slash-commands)
- [OpenAI Build skills](https://learn.chatgpt.com/docs/build-skills.md)
