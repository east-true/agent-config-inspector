# Privacy model

## Default mode

The default scan reads repository-owned instruction sources inside the selected workspace. It does not read home-directory instructions and does not send data over the network.

Text and JSON output may contain:

- workspace-relative repository paths;
- source kind, precedence, scope, and inclusion reason;
- repository content and normalized-unit digests;
- line numbers, counts, confidence, and evidence URLs.

It does not contain instruction text, an absolute workspace path, account name, home path, hostname, credential, provider session output, or repository command output.

## User-context opt-in

`--include-user-context` explicitly permits reads from documented user instruction locations:

- Claude Code: `~/.claude/CLAUDE.md` and Markdown files below `~/.claude/rules/`;
- Codex CLI: the first non-empty `AGENTS.override.md` or `AGENTS.md` in `$CODEX_HOME` (default `~/.codex`);
- Gemini CLI: `~/.gemini/settings.json` only to determine safe context filenames, followed by matching non-empty files directly under `~/.gemini/`.
- Kimi Code CLI: `$KIMI_CODE_HOME/AGENTS.md` (default `~/.kimi-code/AGENTS.md`) and the first non-empty `~/.agents/AGENTS.md` or `~/.agents/agents.md`.

The scanner does not recursively explore arbitrary home directories. User instruction symlinks are not followed.

User-source output replaces real paths with opaque labels such as `<user-instruction-1>`. It withholds content, content digest, normalized-unit digest, size, effective digest, and token estimate. Findings are phrased without excerpts.

Even a redacted report can reveal that a local instruction exists or causes a difference. Review it before publication.

Imports referenced by opted-in Gemini user context are detected but never followed. User settings content, configured filenames, import paths, and context-file paths are not emitted.

## Repository snapshots

`pin` and `verify` refuse `--include-user-context`. The snapshot model is separate from the scan-report model and has no field for source origin, user labels, source content, findings, absolute workspace paths, token estimates, or external state.

Repository snapshots contain only:

- selected provider and target identities;
- adapter and documented provider-version identity;
- repository-relative included and excluded source identities;
- repository source digests and scope metadata;
- repository-only effective graph digests;
- a domain-separated lock digest.

Building a lockfile from an internal report filters user sources before calculating prediction and effective digests. Tests require otherwise-identical reports with and without user context to produce byte-identical lockfiles.

SARIF uses the same boundary. Only validated repository-relative paths can become SARIF locations; external and opaque user labels are omitted.

## External imports

Repository instructions that import an absolute path, `~` path, or relative path outside the workspace are rejected by default. The external path is represented as `<external-import>` and its bytes are not read. Imports originating inside opted-in user instructions are inventoried but not expanded in the preview.

## Non-goals

Redaction is not a data-loss-prevention system and does not inspect arbitrary output produced by downstream tools. A caller can still mishandle the local workspace or report after this process exits.
