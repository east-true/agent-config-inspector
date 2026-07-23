# GitHub Copilot CLI adapter contract

Checked on: 2026-07-24  
Adapter: `github-copilot/cli`  
Semantics baseline: stable Copilot CLI `v1.0.73` and current GitHub documentation

This adapter models GitHub Copilot CLI only. Copilot coding agent, Copilot code review, and Copilot integrations in VS Code or other IDEs are separate surfaces and remain unsupported.

## Modeled repository sources

For a selected target, the adapter finds the nearest `.git` ancestor inside the selected workspace. If no marker exists, it uses the workspace root. It then inspects documented standard locations from that project root through the target directory for:

- `.github/copilot-instructions.md`;
- `AGENTS.md`;
- `CLAUDE.md` and `.claude/CLAUDE.md`;
- `GEMINI.md`.

All applicable general instruction files are combined. Identical bytes across general user, repository-wide, and agent instruction files are represented once; later identical sources are listed as excluded with `ACI041`. GitHub documents no general precedence among these files, so the adapter's source order is a deterministic inventory order, not an override claim.

The adapter recursively discovers `*.instructions.md` below `.github/instructions` at the project root and each directory nested along the selected target path. The static request treats the calculated project root as Copilot's initial working directory. It therefore has no distinct repository-root-to-cwd segment whose intermediate modular locations would be excluded by the CLI documentation.

## `applyTo` compatibility

A modular instruction must have an inline `applyTo` value in YAML frontmatter. Comma-separated patterns are supported, including all documented examples:

- `*`, `**`, and `**/*`;
- `*.py` and `**/*.py`;
- `src/*.py` and `src/**/*.py`;
- `**/subdir/**/*.py`.

The adapter also accepts `?` and brace alternatives such as `*.{ts,tsx}`. A target is included when at least one pattern matches. Missing, malformed, or unsupported `applyTo` metadata is excluded with an `ACI027` finding rather than treated as an unconditional rule.

Patterns are evaluated relative to the standard-location base that contains the `.github/instructions` directory. The selected target is treated as the file Copilot CLI is working with; runtime access to other files can activate a different set.

The `excludeAgent` frontmatter key controls the cloud-agent and code-review surfaces. It does not cause this CLI adapter to infer behavior for those surfaces.

## Imports

The adapter expands line-prefixed `@relative/path` references in `.github/copilot-instructions.md`, `AGENTS.md`, `CLAUDE.md`, `.claude/CLAUDE.md`, and recursively imported files. Relative paths resolve from the importing file. It deliberately does not expand references in `GEMINI.md` or `*.instructions.md` files.

Imports must remain inside the calculated repository boundary. Absolute paths, home-relative paths, URLs, Windows drive paths, and relative escapes are excluded with the external path redacted. Expansion has cycle protection and is capped at five hops or a lower `--max-import-depth` value.

The general CLI command reference currently says imports may be absolute, while the dedicated custom-instructions guide says absolute and `~/` paths are not loaded. This adapter follows the narrower dedicated guide and the scanner's repository privacy boundary instead of reading an absolute path.

## User context

User context is not read by default. With `--include-user-context`, the loader reads only:

- `$COPILOT_HOME/copilot-instructions.md`;
- `$COPILOT_HOME/instructions/**/*.instructions.md`;

where `COPILOT_HOME` defaults to `~/.copilot`. User-source paths, contents, digests, sizes, `applyTo` patterns, effective digest, and token estimate are withheld from output. Imports found in user instructions are reported but never traversed.

`COPILOT_CUSTOM_INSTRUCTIONS_DIRS` is not followed in this baseline because it can name arbitrary external locations. Session enable/disable state from `/instructions` is also not available to an offline scan.

## Surface status

| Surface ID | Status | Boundary |
|---|---|---|
| `github-copilot/cli` | Preview | Static repository and opted-in user custom instructions described above |
| `github-copilot/cloud-agent` | Unsupported | No cloud execution, environment, or surface-specific precedence model |
| `github-copilot/code-review` | Unsupported | No review-event or changed-file context model |
| `github-copilot/vscode` | Unsupported | No editor setting, workspace, or extension-version model |

## Evidence and limitations

Primary evidence:

- [Adding custom instructions for GitHub Copilot CLI](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-custom-instructions)
- [Copilot CLI command reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference)
- [Custom-instruction surface support](https://docs.github.com/en/copilot/reference/custom-instructions-support)
- [Copilot CLI v1.0.73 release](https://github.com/github/copilot-cli/releases/tag/v1.0.73)

The result remains a static prediction. It does not execute Copilot CLI, inspect `/instructions`, observe runtime file access, detect the installed version, or prove that a model followed an instruction. There is no Copilot behavioral probe in this phase.
