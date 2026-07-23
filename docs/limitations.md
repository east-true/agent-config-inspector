# Limitations

Agent Config Inspector predicts static instruction discovery. It does not claim to observe model compliance or reproduce every runtime context layer.

Current limitations:

- Claude Code, Codex CLI, Gemini CLI, Kimi Code CLI, and GitHub Copilot CLI repository instruction discovery are enabled. Other providers remain unsupported.
- Claude imports are expanded only inside the workspace, with at most four hops. Opted-in user-memory imports remain unexpanded.
- Claude path matching implements the common documented `*`, `**`, `?`, and brace-alternative forms. Bracket expressions and the documented expansion budgets are not yet modeled.
- Claude `claudeMdExcludes`, managed policy, additional directories, auto memory, settings-source flags, and lazy subdirectory discovery after arbitrary tool reads are not fully modeled.
- Codex project trust is not observable statically. Project `.codex` settings are analyzed under a documented trust assumption.
- Codex user `config.toml`, profiles, CLI `-c` overrides, and custom project root markers are not yet merged into the prediction. The explicitly selected workspace is treated as the analysis root.
- A Codex source that would cross the combined instruction byte limit is conservatively excluded as a whole; runtime versions may truncate at a different byte boundary.
- Gemini semantics target stable v0.50.0. A selected target is treated as a path accessed by Gemini's JIT context discovery; the scanner does not observe which runtime tool calls actually trigger context activation.
- Gemini project `context.fileName` and `context.memoryBoundaryMarkers` are modeled. System settings, environment overrides, extensions, include directories, experimental memory state, and installed-version detection are not merged.
- Gemini imports are represented as a deterministic depth-first source graph. The preview does not reproduce the runtime's exact inline separator text or every legacy eager-discovery/file-filtering mode.
- Kimi support targets the current TypeScript `MoonshotAI/kimi-code` 0.29.0 product, not the legacy Python `kimi-cli`. It models the default prompt's user/project `AGENTS.md` hierarchy but not `SYSTEM.md`, custom agents, agent-file launch flags, or experimental engine selection.
- A selected Kimi target is treated as the CLI working path. Without a `.git` ancestor, only the target directory is searched. Additional workspace directories do not contribute instruction files.
- Kimi `@path` text is not expanded. The 32 KiB threshold produces a warning without truncation. Exact warning-byte parity can differ when user path annotations are redacted.
- Kimi can follow instruction symlinks at runtime; the scanner rejects them by default and only follows explicitly enabled links that stay inside the workspace.
- Copilot support targets the CLI surface and stable v1.0.73 documentation. Copilot coding agent, code review, VS Code, and other IDE surfaces remain independently unsupported.
- Copilot general instructions are inventoried deterministically because GitHub documents combination and exact duplicate removal but no general precedence. The output order must not be interpreted as an override order.
- Copilot modular discovery treats the calculated project root as the initial working directory and models target-nested standard locations. A runtime started from a deeper cwd can exclude modular locations in the repository-root-to-cwd intermediate segment. Later file access can activate sources not present for the selected target.
- Copilot `applyTo` supports documented `*`/`**` patterns plus `?` and brace alternatives. More expansive glob dialect features are not inferred; missing or malformed `applyTo` remains partial instead of becoming unconditional.
- Copilot imports are expanded only for documented Copilot, AGENTS, and Claude instruction families, inside the repository, with at most five hops. User imports, `GEMINI.md` imports, and modular-file imports are not expanded.
- Copilot `/instructions` enable/disable state, `COPILOT_CUSTOM_INSTRUCTIONS_DIRS`, installed-version detection, and provider-managed policy are not modeled. No Copilot behavioral probe exists yet.
- Token estimates use UTF-8 bytes divided by four, not a provider tokenizer.
- Lexical command and prohibition findings are conservative signals, not semantic equivalence judgments.
- Symlinked rule directories are not recursively walked in the preview. Direct source-file symlinks can be enabled only when they resolve inside the workspace.
- `matrix` belongs to a later phase. `probe` currently measures only repository-root instruction discovery; nested precedence, imports, overrides, and other runtime behavior remain unmeasured.
- Probe execution requires an installed provider CLI and a documented process-scoped API credential. Cached login state is intentionally not copied into the isolated home.
- Provider-managed policy, wrappers, system configuration, CLI version drift, API retries, and service-side behavior can block or alter a probe. Authentication, quota, timeout, and provider failures remain inconclusive rather than becoming negative discovery evidence.
- Probe response matching is an exact synthetic marker observation in bounded stdout. Provider stdout/stderr are held only in bounded process memory for classification and are not emitted or persisted by the tool.
- Snapshot verification detects repository and adapter/tool metadata drift but does not semantically classify whether a changed instruction is equivalent.
- The composite GitHub Action requires a published release with matching platform tarballs and `checksums.txt`; branch-only revisions cannot be installed through the Action.
- Release archives currently target Linux and macOS on amd64 and arm64. Windows packaging is not yet available.

When accuracy matters, compare a report with the provider's own runtime context inspection and record the exact provider version.
