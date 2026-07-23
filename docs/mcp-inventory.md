# Repository MCP inventory

Status: Phase 8c developer preview (`v0.9.0-dev`)

`inventory mcp` reports the repository-owned MCP server declarations that Claude Code or Codex CLI can discover for a selected target. It is a static, offline inventory. It does not start a provider, execute a command, connect to a server, read credentials, perform OAuth, or decide whether a server should be trusted.

```bash
agent-config-inspector inventory mcp [workspace] \
  --providers claude,codex \
  --target path/to/file \
  --format text
```

Only Claude Code and Codex CLI are supported on this surface. Gemini, Kimi, Copilot, Grok, and other providers return unsupported; similar MCP terminology does not imply compatible storage, merging, trust, or transport rules.

## Provider contracts

### Claude Code

The inventory reads only the repository-root `.mcp.json` and its `mcpServers` object. The selected target does not add nested `.mcp.json` layers.

Recognized transports are:

- `stdio`, requiring a non-empty string `command`; an omitted `type` is treated as `stdio` only when `command` is present;
- `http` and its `streamable-http` alias, requiring a non-empty string `url`;
- deprecated `sse`, requiring a non-empty string `url`;
- `ws`, requiring a non-empty string `url`.

A remote URL without an explicit type is invalid. An empty remote URL is classified as `unconfigured`. Claude-reserved server names are excluded. JSON with duplicate object fields is rejected instead of silently selecting a value.

Project-scoped servers require a runtime approval decision. Approval state is intentionally not read or predicted, so `ACI091` is emitted when a project MCP source is inventoried.

### Codex CLI

The inventory reads `.codex/config.toml` along the workspace-root-to-selected-target directory chain. It projects `[mcp_servers.<name>]` tables and applies Codex's recursive TOML-table merge model from the root layer toward the closest layer. A closer field replaces the same field while unrelated parent fields remain effective. `ACI094` identifies a server whose structural record has more than one contributing project source.

Recognized transports are:

- `stdio`, selected by a non-empty string `command`;
- streamable HTTP, reported as `http`, selected by a non-empty string `url`.

Declaring both transports or neither is invalid. Effective `enabled = false` servers are reported as excluded and `disabled`. `required` is reported as a boolean, but the runtime consequence of a startup failure is not tested.

Codex project configuration is applied only for trusted projects. Trust state is runtime context and remains unobserved (`ACI021`). The bounded parser supports ordinary and quoted TOML table/key forms; a top-level inline `mcp_servers = {...}` declaration is rejected conservatively rather than partially decoded.

## Privacy-safe output

The report may contain:

- server name;
- repository-relative source path and scope base;
- contributing repository paths for a merged Codex definition;
- normalized transport, enabled/required state, and configured/disabled/unconfigured/invalid status;
- whether a declaration can execute a local process;
- whether credential-bearing field families are present;
- an allowlisted set of top-level field names;
- source byte count and a structural SHA-256 digest.

The report never contains:

- command or argument values;
- URL values;
- environment-variable names or values;
- header names or values;
- tokens, authentication settings, OAuth details, scopes, or resource values;
- enabled/disabled tool names or per-tool approval values;
- arbitrary unknown field names or values;
- an absolute workspace path or resolved symlink target.

`metadata_digest` is computed only from the already-public structural projection. It is not a digest of `.mcp.json`, `config.toml`, or any hidden value. This prevents the digest itself from becoming a stable fingerprint of a token, URL, or command.

Server names, repository-relative paths, byte counts, and structural digests can still be private repository metadata. Review reports before publishing them.

## Findings

| Code | Severity | Meaning |
|---|---|---|
| `ACI021` | info | Codex project trust is a runtime condition |
| `ACI090` | error | MCP source, name, transport, or declaration is invalid |
| `ACI091` | info | Claude project approval state is not statically observable |
| `ACI092` | warning | Credential-bearing field families are present; values remain hidden |
| `ACI093` | warning | Repository MCP configuration can execute a local process or command helper |
| `ACI094` | info | A Codex server combines multiple project configuration layers |
| `ACI095` | error | A source read, byte bound, or server-count bound prevented complete inspection |

Warnings do not fail by default. Use `--fail-on warning` for a policy gate or `--fail-on never` for an informational inventory.

## Safety and limits

- The default maximum source size is 1 MiB.
- At most 512 effective server definitions are reported per provider and target.
- Exact configuration-file symlinks are refused by default. `--follow-workspace-symlinks` permits only targets that resolve inside the selected workspace.
- User, local, managed, plugin, session, and CLI-added MCP sources are not read.
- Server startup, reachability, protocol negotiation, authentication, tool discovery, approval, and actual provider availability are not tested.

JSON output uses the independent [`schemas/mcp.schema.json`](../schemas/mcp.schema.json) schema, currently version 1.

Primary references checked on 2026-07-24:

- [Claude Code MCP configuration](https://code.claude.com/docs/en/mcp)
- [Codex MCP configuration](https://learn.chatgpt.com/docs/extend/mcp)
- [Codex recursive configuration merge](https://github.com/openai/codex/blob/main/codex-rs/config/src/merge.rs)
