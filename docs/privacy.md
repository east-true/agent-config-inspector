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
- GitHub Copilot CLI: `copilot-instructions.md` and recursive `instructions/**/*.instructions.md` under `$COPILOT_HOME` (default `~/.copilot`). `COPILOT_CUSTOM_INSTRUCTIONS_DIRS` is not followed.

The scanner does not recursively explore arbitrary home directories. User instruction symlinks are not followed.

User-source output replaces real paths with opaque labels such as `<user-instruction-1>`. It withholds content, content digest, normalized-unit digest, size, effective digest, and token estimate. Findings are phrased without excerpts.

Even a redacted report can reveal that a local instruction exists or causes a difference. Review it before publication.

Imports referenced by opted-in Gemini or Copilot user context are detected but never followed. User settings content, configured filenames, `applyTo` patterns, import paths, and context-file paths are not emitted.

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

## Repository configuration inventories

`inventory skills`, `inventory agents`, and `inventory mcp` are repository-only surfaces. They do not offer `--include-user-context` and do not read personal, local, admin, managed, plugin, built-in, session, or CLI-added definitions.

Skills output hides descriptions and `SKILL.md` bodies. Custom-agent output hides descriptions, Markdown prompts, `developer_instructions`, and all model, tool, MCP, hook, permission, command, URL, and environment values. The agent parser emits only an allowlisted set of capability field names and ignores arbitrary unknown metadata in output.

Inventory reports can still contain repository-relative paths, names, byte counts, and unkeyed domain-separated SHA-256 digests. These are fingerprints and repository metadata, not encryption or anonymization. Review them before publishing output from a private repository; do not publish when even filenames, agent names, or equality testing against a known candidate value would be sensitive.

No repository inventory traverses supporting executable resources or starts a provider. Direct source symlinks are skipped or refused by default, depending on whether the source is a discovered entry or an exact configuration file. Opt-in following is limited to resolved files inside the selected workspace, and absolute targets are never printed.

MCP inventory output hides command and argument values, URLs, environment and header names and values, tokens, authentication settings, OAuth details, scopes, tool names, and approval values. It exposes only an allowlisted structural projection such as normalized transport, enabled/required state, local-execution potential, and credential-field-family presence. Its `metadata_digest` is computed from that public projection, never from raw configuration bytes or a hidden value. The inventory does not start a command helper or server, connect to an endpoint, authenticate, or read runtime trust and approval state.

## External imports

Repository instructions that import an absolute path, `~` path, or relative path outside the workspace are rejected by default. The external path is represented as `<external-import>` and its bytes are not read. Imports originating inside opted-in user instructions are inventoried but not expanded in the preview.

## Behavioral probe opt-in

`probe` does not share the scanner's offline execution path. Plan mode is the default and performs only provider registry resolution plus executable discovery on `PATH`; it does not start the executable, create a fixture, inspect credentials, or use the network.

`--execute --acknowledge-quota` authorizes one generated-fixture provider run. The probe creates a temporary home and repository, never copies the real provider home, and passes a new environment containing only basic process variables, documented proxy/TLS variables, provider-specific endpoint variables, and the required process-scoped credential. Unrelated parent variables do not cross this boundary.

Provider stdout and stderr are bounded in memory, checked for the synthetic marker and a small set of failure signals, and discarded. Text and JSON results can contain:

- the provider ID, case, synthetic fixture digest, and documented safe invocation shape;
- executable availability, detected CLI version, process/exit state, and output-truncation state;
- marker observation, terminal classification, and failure stage;
- credential variable names, never their values.

Results do not contain the marker, prompt, instruction text, response body, temporary path, real home path, account identity, credential value, hostname, or timestamp. The tool does not automatically write a result file or upload an artifact. Provider services may retain requests according to their own account and data policies; that external retention is outside this repository's control.

## Non-goals

Redaction is not a data-loss-prevention system and does not inspect arbitrary output produced by downstream tools. A caller can still mishandle the local workspace or report after this process exits. A substituted provider binary or local wrapper can behave differently from the documented CLI, so verify executable provenance before opting into a probe.
