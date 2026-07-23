# Limitations

Agent Config Inspector predicts static instruction discovery. It does not claim to observe model compliance or reproduce every runtime context layer.

Current limitations:

- Only Claude Code and Codex CLI repository instruction discovery is enabled.
- Claude imports are expanded only inside the workspace, with at most four hops. Opted-in user-memory imports remain unexpanded.
- Claude path matching implements the common documented `*`, `**`, `?`, and brace-alternative forms. Bracket expressions and the documented expansion budgets are not yet modeled.
- Claude `claudeMdExcludes`, managed policy, additional directories, auto memory, settings-source flags, and lazy subdirectory discovery after arbitrary tool reads are not fully modeled.
- Codex project trust is not observable statically. Project `.codex` settings are analyzed under a documented trust assumption.
- Codex user `config.toml`, profiles, CLI `-c` overrides, and custom project root markers are not yet merged into the prediction. The explicitly selected workspace is treated as the analysis root.
- A Codex source that would cross the combined instruction byte limit is conservatively excluded as a whole; runtime versions may truncate at a different byte boundary.
- Token estimates use UTF-8 bytes divided by four, not a provider tokenizer.
- Lexical command and prohibition findings are conservative signals, not semantic equivalence judgments.
- Symlinked rule directories are not recursively walked in the preview. Direct source-file symlinks can be enabled only when they resolve inside the workspace.
- `matrix`, `pin`, `verify`, `probe`, SARIF, and the reusable GitHub Action belong to later phases.

When accuracy matters, compare a report with the provider's own runtime context inspection and record the exact provider version.
