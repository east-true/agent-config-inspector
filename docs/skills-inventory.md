# Agent Skills inventory

Status: Phase 8a developer preview (`v0.7.0`)

Checked on: 2026-07-24

Agent Config Inspector inventories repository-owned Agent Skills that Claude Code or Codex CLI can discover for a selected path. This is a separate configuration surface from instruction resolution: a discovered skill is not treated as an always-loaded instruction, and the tool never claims that a provider will activate or follow it.

## Command

```bash
agent-config-inspector inventory skills [workspace] \
  --providers claude,codex \
  --target path/to/file \
  --format text
```

The default providers are `claude,codex`, the default target is `.`, and the supported formats are `text` and `json`. `--fail-on error|warning|never`, `--max-source-bytes`, and `--follow-workspace-symlinks` use the same safety conventions as instruction scanning.

The target represents the launch or accessed path. For a file target, its parent directory is used. Results include only skill roots on the workspace-root-to-target directory chain.

## Discovery contract

| Provider | Repository roots examined | Metadata baseline |
|---|---|---|
| Claude Code | `.claude/skills/*/SKILL.md` at each root-to-target directory | Directory name is the invocation name. Frontmatter fields are optional; a missing or malformed description is reported because automatic matching is unreliable. |
| Codex CLI | `.agents/skills/*/SKILL.md` at each root-to-target directory | `name` and `description` are required. The name must match its directory and the Agent Skills naming constraints. |

Each skill root is listed non-recursively. The tool reads only the immediate `SKILL.md`; it does not walk supporting `scripts`, `references`, `assets`, or arbitrary files.

When multiple discoverable skills share a name, every source remains in the inventory and `ACI072` is emitted. The preview does not choose a winner or simulate a runtime selector.

## Output and privacy

Reports may contain:

- provider ID and selected target
- repository-relative `SKILL.md` path
- effective inventory name and optional declared name
- scope base and metadata status
- source byte count and domain-separated SHA-256 digest
- whether a description exists, its byte count, and a domain-separated digest
- evidence and bounded findings

Reports never contain:

- description text or `SKILL.md` body
- supporting-file content
- absolute workspace paths
- user, admin, system, bundled, or plugin skill paths
- provider output, credentials, or environment values

The JSON contract is [skills.schema.json](../schemas/skills.schema.json). It is independent from the instruction report schema so a new configuration surface does not silently change `scan`, `explain`, `diff`, `pin`, or `verify` output.

## Symlink policy

Symlinked skill directories are recorded as excluded by default. `--follow-workspace-symlinks` follows a skill directory only when its resolved target remains inside the selected workspace. An external target is a safety error. The report never exposes the resolved absolute target.

## Findings

| Code | Meaning |
|---|---|
| `ACI070` | Skill frontmatter is malformed or required metadata is missing or too large. |
| `ACI071` | A provider-specific name contract is not satisfied or Claude's display label differs from its directory invocation name. |
| `ACI072` | Multiple discoverable repository skills share the same inventory name. |
| `ACI073` | A symlinked skill directory was skipped by the safe default. |
| `ACI074` | A `SKILL.md` could not be read within the configured byte bound. |

## Explicit limitations

- No user-level opt-in exists in Phase 8a. This avoids exposing even the existence or names of personal skills.
- Claude `.claude/commands`, `--add-dir`, managed skills, bundled skills, and plugin skills are excluded.
- Codex user, admin, system, and plugin skills are excluded.
- Full YAML semantics are not implemented. The bounded parser supports the scalar and block forms needed for `name` and `description`; unsupported or malformed shapes produce findings.
- Runtime context budgets, settings overrides, disabled skills, live file watching, explicit invocation, automatic activation, and model compliance are not observed.
- Gemini, Kimi, Copilot, Grok, and other tools remain unsupported for the Skills surface even if they adopt a compatible directory format.

## Evidence

- [Agent Skills specification](https://agentskills.io/specification)
- [Claude Code: Extend Claude with skills](https://code.claude.com/docs/en/slash-commands)
- [OpenAI: Build skills](https://learn.chatgpt.com/docs/build-skills.md)
