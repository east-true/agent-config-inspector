# Custom agents inventory

Status: Phase 8b developer preview (`v0.8.0-dev`)

Checked on: 2026-07-24

Agent Config Inspector inventories repository-owned custom agents that Claude Code or Codex CLI can discover for a selected path. This is a separate configuration surface from instruction resolution and Agent Skills. Discovery does not mean that an agent will be selected, delegated to, or allowed to run.

## Command

```bash
agent-config-inspector inventory agents [workspace] \
  --providers claude,codex \
  --target path/to/file \
  --format text
```

The default providers are `claude,codex`, the default target is `.`, and the supported formats are `text` and `json`. The command also accepts `--fail-on error|warning|never`, `--max-source-bytes`, and `--follow-workspace-symlinks`.

The target represents the launch or accessed path. For a file target, its parent directory is used. Only agent roots on the workspace-root-to-target directory chain are examined. Unrelated sibling directories are excluded.

## Discovery contract

| Provider | Repository roots examined | Standalone file contract |
|---|---|---|
| Claude Code | Recursive `*.md` files below `.claude/agents/` at each root-to-target directory | YAML frontmatter requires non-empty `name` and `description`; the Markdown body is the agent prompt. The name uses lowercase letters separated by single hyphens. |
| Codex CLI | Recursive `*.toml` files below `.codex/agents/` at each enabled project configuration layer on the root-to-target chain | Top-level `name`, `description`, and `developer_instructions` strings are required. The `name` field, not the filename, is the identity. |

The Codex hierarchy model follows its project configuration layers. Project `.codex` layers are a runtime trust decision; static inventory cannot observe whether the selected installation has trusted the project. Every Codex result therefore includes `ACI021` and states the trust assumption.

The inventory intentionally excludes personal agents from `~/.claude/agents/` and `~/.codex/agents/`. It also excludes Claude CLI `--agents` definitions, plugin agents, managed agents, Codex roles declared only through `[agents.*]` configuration, built-in agents, and session-provided definitions.

## Name collisions and precedence

Claude Code scans every `.claude/agents` directory in the working-directory hierarchy. Since Claude Code 2.1.178, a definition from the closest project directory wins when names collide across directories. If multiple files below the same `.claude/agents` root declare the same name, documented filesystem selection is not deterministic. The inventory preserves those closest candidates as `ambiguous`, returns a partial result, and emits `ACI082`.

Codex loads agent directories as configuration layers from lower to higher precedence. A closer project layer replaces a lower definition with the same name. Inside one layer, current Codex source recursively collects TOML files, sorts their paths, keeps the first definition for a name, and warns about later duplicates. The inventory mirrors that deterministic selection and emits an `ACI082` warning.

Neither collision rule predicts which available agent a parent model will choose for a task.

## Output and privacy

Reports may contain:

- provider ID and selected target
- repository-relative source path and scope base
- declared agent name and source format
- valid, invalid, unread, or ambiguous metadata status
- source byte count and domain-separated SHA-256 digest
- description and instruction presence, byte counts, and domain-separated digests
- allowlisted capability field names such as `tools`, `model`, `sandbox_mode`, `mcp_servers`, or `skills`
- evidence and bounded findings

Reports never contain:

- description text, Markdown prompt bodies, or `developer_instructions`
- tool allowlists, model identifiers, MCP server definitions, commands, URLs, environment values, or permission values
- arbitrary unknown metadata keys or values
- absolute workspace paths or resolved symlink targets
- personal, admin, managed, plugin, built-in, or session-agent metadata
- credentials, provider output, or environment variables

Agent names and repository-relative filenames remain visible because they are the inventory identifiers. Treat a JSON or text report as repository metadata: review it before publishing a report produced from a private repository.

Description and instruction digests are unkeyed fingerprints, not encryption. They support deterministic drift comparison without printing prompt text, but a party that already knows a candidate value can test it. Do not publish an inventory report from a private repository when even source fingerprints or agent names are confidential.

The JSON contract is [agents.schema.json](../schemas/agents.schema.json). It is independent from both the instruction report and Skills inventory schemas.

## Parser and resource bounds

The inventory reads at most 1 MiB per source by default and at most 512 matching agent files per provider and target. The closest scopes are retained first when the file-count bound is reached. A read or count limit makes the result partial and emits `ACI084`.

The bounded metadata parser extracts only the fields needed for inventory and privacy-safe capability names. It is not a general YAML or TOML editing library. A malformed required scalar, repeated required field, missing frontmatter terminator, or unterminated string is excluded with a finding. Declared names are limited to 128 UTF-8 bytes and may not contain control characters so hostile metadata cannot produce unbounded or terminal-active output. Runtime implementations can reject additional provider-specific shapes that this preview does not fully validate.

## Symlink policy

A direct symlinked agent source is recorded as excluded by default with `ACI083`. `--follow-workspace-symlinks` reads it only when the resolved target remains inside the selected workspace. An external target is a safety error. Symlinked directories are not recursively traversed.

## Findings

| Code | Meaning |
|---|---|
| `ACI021` | Codex project trust is a runtime condition that static inventory cannot observe. |
| `ACI080` | Required agent metadata is malformed, missing, or empty. |
| `ACI081` | An agent name violates the Claude identifier contract or the inventory's safe-output bound. |
| `ACI082` | A closer definition shadows another agent, or a same-layer name collision requires provider-specific selection. |
| `ACI083` | A direct symlinked agent source was skipped by the safe default. |
| `ACI084` | A source could not be read within the configured bound or the 512-file inventory limit was reached. |

## Explicit limitations

- Only repository-owned Claude Code and Codex CLI standalone agent files are supported.
- The command does not load user, local-only, admin, managed, plugin, built-in, or CLI/session agent definitions.
- The Codex trust decision, installed version, feature flags, project-root marker overrides, and disabled configuration layers are not observed.
- Claude settings-source selection, safe mode, CLI `--agents`, plugin precedence, live reload behavior, and agent disablement are not modeled.
- Codex `[agents.*]` declarations and explicit `config_file` references are not inventoried in this phase.
- Capability values are deliberately hidden, so the report does not assess whether tools, hooks, MCP servers, sandbox settings, or skills are safe or reachable.
- The command never invokes an agent, executes its prompt, starts a provider CLI, launches tools, or predicts task matching and delegation.
- Gemini, Kimi, Copilot, Grok, and other providers remain unsupported for this surface even when they offer similarly named agent features.

## Evidence

- [Claude Code subagents](https://code.claude.com/docs/en/sub-agents)
- [OpenAI Codex subagents and custom agents](https://learn.chatgpt.com/docs/agent-configuration/subagents.md)
- [OpenAI Codex custom-agent loader](https://github.com/openai/codex/blob/main/codex-rs/core/src/config/agent_roles.rs)
- [OpenAI Codex configuration precedence and project trust](https://learn.chatgpt.com/docs/config-file/config-basic.md)
