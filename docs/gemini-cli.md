# Gemini CLI adapter

Status: preview

Provider ID: `google-gemini/cli`

Aliases: `gemini`, `gemini-cli`

Semantics target: Gemini CLI v0.50.0, checked 2026-07-24

This adapter statically predicts repository context for a selected target. It does not run Gemini CLI, observe tool calls, or claim that a model will follow the discovered text.

## Inputs modeled

The repository scan considers:

- `GEMINI.md` from the nearest memory boundary through the selected target directory;
- safe project settings in `<boundary>/.gemini/settings.json`;
- `context.fileName` as either a string or an ordered string array;
- `context.memoryBoundaryMarkers` as a string array;
- Markdown `@path` imports outside inline code and fenced code blocks;
- non-empty user context directly below `~/.gemini/` only when `--include-user-context` is supplied.

Configured context filenames are checked first and `GEMINI.md` remains a fallback candidate, matching v0.50.0 filename registration order. With default settings, the nearest `.git` entry between the workspace and target is the memory boundary. With an empty marker list, the workspace is the boundary.

## Resolution algorithm

For a target such as `packages/api/src/main.go`, the adapter:

1. normalizes the target inside the selected workspace;
2. locates the nearest default `.git` boundary;
3. reads project `.gemini/settings.json` at that boundary, if present;
4. recalculates the boundary with configured markers;
5. visits directories from that boundary to the target directory;
6. checks configured context filenames in order at each directory;
7. includes each non-empty context source and expands its imports depth first;
8. emits provenance, digests, scope, findings, and an effective digest without exposing source text.

The selected target is deliberately treated as a path Gemini accessed through its just-in-time context discovery. Finding `ACI064` records this assumption because a static scan cannot know which paths a live session will actually access.

## Import safety

Imports are capped at five hops, or a lower `--max-import-depth` value. Relative imports resolve from the importing file. Absolute imports are accepted only when they map back into the selected workspace and remain inside the calculated Gemini project boundary.

The adapter does not read imports that:

- escape the workspace or Gemini project boundary;
- use a home-relative path;
- traverse a disallowed symlink;
- exceed the configured depth;
- repeat an active source and form a cycle.

External import targets are represented only as `<external-import>`. Their original path and content are not read or emitted.

## User-context privacy

User context is disabled by default. With `--include-user-context`, the loader may read `~/.gemini/settings.json` to determine safe direct filenames and then inventory matching files under `~/.gemini/`. Reports use opaque labels such as `<user-instruction-1>` and omit the filename, path, content, digest, size, and token estimate. Imports from these private files are detected but never followed.

`pin` and `verify` reject user context entirely, so a committed snapshot cannot encode whether private Gemini files exist.

## Known gaps

The preview does not merge system settings, environment overrides, extension settings, include directories, experimental memory state, or the installed CLI version. It represents imports as a deterministic source graph rather than reproducing Gemini's exact inline comment/separator text. It also cannot observe runtime tool-access order or model compliance.

See [Limitations](limitations.md), [Privacy](privacy.md), and the [support matrix](support-matrix.md) for the cross-provider contract.

## Evidence

- [Gemini CLI context files](https://geminicli.com/docs/cli/gemini-md/)
- [Gemini CLI memory import processor](https://geminicli.com/docs/reference/memport/)
- [Gemini CLI v0.50.0 settings schema](https://raw.githubusercontent.com/google-gemini/gemini-cli/v0.50.0/schemas/settings.schema.json)
- [Gemini CLI v0.50.0 changelog](https://geminicli.com/docs/changelogs/latest/)
