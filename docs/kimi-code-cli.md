# Kimi Code CLI adapter

Status: preview

Provider ID: `moonshotai-kimi-code/cli`

Aliases: `kimi`, `kimi-code`

Semantics target: Kimi Code CLI 0.29.0, checked 2026-07-24

This baseline adapter statically predicts the instruction files injected by the default Kimi Code CLI prompt for a selected target. It does not run Kimi, select a custom agent, or claim that a model will follow the discovered text.

## Product boundary

The adapter targets the current TypeScript product in [`MoonshotAI/kimi-code`](https://github.com/MoonshotAI/kimi-code), not the legacy Python implementation in `MoonshotAI/kimi-cli`. The products have different data directories and instruction behavior. Evidence is pinned to the current product's 0.29.0 release rather than borrowing the legacy CLI's `.kimi/` rules.

## Inputs modeled

With user context disabled, the repository scan considers these files from the nearest Git root through the selected target directory:

1. `.kimi-code/AGENTS.md`, when non-empty;
2. the first non-empty generic candidate in the same directory: `AGENTS.md`, then `agents.md`.

Both the Kimi-specific and generic project files can apply at one directory. A non-empty uppercase generic filename prevents the lowercase fallback from being checked. With no `.git` entry in the target's ancestry, Kimi treats its working directory as the project root; the adapter therefore searches only the selected target directory.

With `--include-user-context`, two earlier user scopes are added:

1. `$KIMI_CODE_HOME/AGENTS.md`, or `~/.kimi-code/AGENTS.md` when the environment variable is unset;
2. the first non-empty generic candidate under `~/.agents/`: `AGENTS.md`, then `agents.md`.

The final order is user Kimi-specific, user generic, then project directories from root to target. Markdown frontmatter has no special instruction-file meaning and remains part of the content.

## Resolution algorithm

For a target such as `packages/api/src/main.ts`, the adapter:

1. normalizes the target inside the selected workspace and uses its parent directory as Kimi's working directory;
2. searches upward for the nearest `.git` file or directory;
3. falls back to the target directory when no marker exists;
4. visits every directory from that project root to the target;
5. checks the Kimi-specific file and generic filename fallback in documented order;
6. skips empty files and preserves every included file in root-to-target order;
7. emits provenance, source digests, scope, findings, and an effective digest without exposing source text.

Kimi does not expand `@path` syntax in these instruction files. Such text remains ordinary instruction content.

## Size guidance

Kimi Code CLI 0.29.0 injects the complete merged instruction content. When the rendered content exceeds 32 KiB, the runtime presents a warning instead of truncating it. Agent Config Inspector emits `ACI045` while retaining every source. Because user paths are deliberately opaque, a scan containing user context can differ slightly from the runtime at the exact annotation-byte boundary.

## User-context privacy

User context is disabled by default. When enabled, reports replace source paths with opaque labels such as `<user-instruction-1>` and omit content, digest, size, token estimate, and normalized units. User instruction symlinks are never followed. `pin` and `verify` reject user context entirely.

## Known gaps

The baseline predicts the built-in default prompt. It does not model `SYSTEM.md`, `--agent`, `--agent-file`, project custom-agent files, the experimental v2 engine selection, or agent templates that omit `${base_prompt}` or `${agents_md}`. It does not load instructions from additional workspace directories. The installed CLI version and runtime prompt selection are not observed.

Kimi 0.29.0 can follow instruction-file symlinks. The scanner is intentionally stricter: symlinks are rejected by default and can be followed only with `--follow-workspace-symlinks` when the resolved target remains inside the selected workspace.

See [Limitations](limitations.md), [Privacy](privacy.md), and the [support matrix](support-matrix.md) for the cross-provider contract.

## Evidence

- [Kimi Code CLI 0.29.0 release](https://github.com/MoonshotAI/kimi-code/releases/tag/%40moonshot-ai%2Fkimi-code%400.29.0)
- [Kimi 0.29.0 instruction loader source](https://github.com/MoonshotAI/kimi-code/blob/%40moonshot-ai%2Fkimi-code%400.29.0/packages/agent-core/src/profile/context.ts)
- [Kimi Code agents and instruction files](https://www.kimi.com/code/docs/en/kimi-code-cli/customization/agents.html)
- [Kimi Code data locations](https://www.kimi.com/code/docs/en/kimi-code-cli/configuration/data-locations.html)
