🌐 [Português](../../pt-br/reference/cli.md) | **English** | 🏠 [Index](../index.md)

---

# CLI Reference

Binary: `build82`. Source of truth: `cmd/build82/main.go` and `internal/transport/http.go`.

## Commands

The subcommand is read from the first argument only.

| Invocation | Description |
| :--- | :--- |
| `build82` | Runs the MCP server over stdio. |
| `build82 --http [flags]` | Runs the MCP server over Streamable HTTP and SSE. |
| `build82 install [target]` | Registers build82 in an AI tool. |
| `build82 uninstall [target] [--purge]` | Removes the registration. See [Uninstallation](../getting-started/uninstallation.md). |
| `build82 self-update [flags]` | Updates the binary to the latest release. |
| `build82 -h`, `build82 --help` | Prints the help text to stdout and exits with code 0. |
| `build82 <subcommand> -h`, `build82 <subcommand> --help` | For `install`, `uninstall` and `self-update`: prints that subcommand's usage and options to stdout and exits with code 0 without running it. |
| `build82 --version` | Prints the version (`dev` for a build without release ldflags) and exits with code 0. |

Argument resolution for the first argument:

| First argument | Behaviour |
| :--- | :--- |
| None | stdio server. |
| `--http`, `install`, `uninstall`, `self-update`, `--help`, `-h`, `--version` | As listed above. |
| Any other value starting with `-` | Treated as server flags (see below). An unknown flag, or a server-only flag without `--http` among the arguments, is a usage error (exit code 2). |
| Any other value | `Error: unknown command "<value>"`, followed by the help text, on stderr. Exit code 1. |

### Subcommand help, `--` and interrupts

| Rule | Behaviour |
| :--- | :--- |
| Subcommand help | `-h` or `--help` anywhere among the arguments of `install`, `uninstall` or `self-update` — before a `--` — prints the subcommand's help and exits 0; nothing else is parsed or run (`build82 self-update --check --help` only prints help). |
| `--` | Ends the options. For `install` and `uninstall`, every argument after it is the target, even one starting with `-` (`build82 uninstall -- --help` looks up a target named `--help`). `self-update` and the server take no positional argument, so anything after `--` is a usage error; a trailing `--` alone is accepted. |
| Usage-error pointer | A usage error of `install`, `uninstall` or `self-update` ends with `Run 'build82 <subcommand> --help' for usage.`; a server-flag error with `Run 'build82 --help' for usage.` |
| Ctrl-C at a prompt | The first SIGINT or SIGTERM while `install`, `uninstall` (including the `--purge` confirmation) or `self-update` waits for an answer prints `Interrupted; nothing was changed.` on stderr and exits with code 1. During `self-update` it also aborts a download in progress (before the binary is replaced). Between two targets of `install`/`uninstall` without a target, the remaining targets are skipped and the command exits with code 1. A second Ctrl-C ends the process at once. |

## Server flags

Parsed by a manual walk of the arguments, in any order (`build82 --port 8080 --http` works). A `--` ends the flags (nothing may follow it). An unrecognised argument, a flag missing its value, or a server-only flag (`--port`, `--host`, `--token`, `--allowed-host`) without `--http` is a usage error: `Error: <message>` and `Run 'build82 --help' for usage.` on stderr, exit code 2. `--help`, `-h` and `--version` are also honoured inside this walk (print and exit 0).

| Flag | Type | Default | Applies to | Description |
| :--- | :--- | :--- | :--- | :--- |
| `--http` | bool | off | — | Selects the HTTP transport. Required by every other server flag. |
| `--port <n>` | integer 1–65535 | `3000` | `--http` | TCP port. An invalid value prints `warning: invalid --port value "<v>", keeping <n>` to stderr and keeps the previous value. A missing value is a usage error. The next argument is always consumed as the value. |
| `--host <host>` | string | `127.0.0.1` | `--http` | Bind address. Also added to the Host allow-list. Missing value: usage error. |
| `--token <token>` | string | none (auth disabled) | `--http` | Bearer token required on `/mcp` and `/sse`. Last occurrence wins. Missing value: usage error. |
| `--allowed-host <host>` | string, repeatable | none | `--http` | Extra value accepted in the `Host` header. Missing value: usage error. |

### Token resolution

Applies with `--http`, evaluated in this order:

| # | Condition | Result |
| :--- | :--- | :--- |
| 1 | `--token <value>` passed, value not empty | Use the value. `BUILD82_TOKEN` is ignored. |
| 2 | `--token ""` passed | Error: `--token was passed with an empty value; omit --token entirely to run --http without authentication`. Exit code 1. |
| 3 | `--token` passed without a following argument | Usage error: `--token requires a value`. Exit code 2. |
| 4 | `BUILD82_TOKEN` set, not empty | Use the value. |
| 5 | `BUILD82_TOKEN` set but empty | Error: `BUILD82_TOKEN is set but empty; unset it entirely to run --http without authentication`. Exit code 1. |
| 6 | Neither present (variable unset) | Authentication disabled. |

Details: [Configuration Reference](./configuration.md#environment-variables).

## `install`

```bash
build82 install [target]
```

| Item | Value |
| :--- | :--- |
| Flags | None. At most one positional target; any flag or second argument is a usage error (exit 2). |
| `target` | `claude` (Claude Code), `claude-desktop`, `antigravity`, `codex`, `opencode`, `cursor`, `zed`, `cline`. Omitted: all detected tools. |
| Prompts | Moodle root path (offers the current directory when it is a Moodle root; validated as a Moodle root), and `Install build82 into all N detected tool(s)? [y/N]` when no target is given. |
| Writes | The `build82` entry in each tool's configuration, with `BUILD82_MOODLE_PATH` in its environment and the resolved absolute path of the running binary as the command. `claude` runs `claude mcp add --scope user build82 -e BUILD82_MOODLE_PATH=<path> -- <binary>`; `codex` runs `codex mcp add build82 --env BUILD82_MOODLE_PATH=<path> -- <binary>`. For both, an existing registration reported by `<tool> mcp get build82` is removed first (for `claude`: every `user` and `local` registration; a `project` registration in `.mcp.json` is left unchanged and reported with a warning, see [Claude Code](../guides/clients/claude-code.md)). The other targets merge the entry into their JSON file (see [client guides](../guides/clients/claude-code.md)); `claude-desktop` writes every Claude Desktop configuration directory that exists (on Windows, also those of the MSIX package); `cline` writes every Cline settings file that exists (the CLI one under `CLINE_DATA_DIR` or `CLINE_DIR` when set). Every other key, the key order and each value are kept as written, as are the file's mode and a symbolic link at its path. A file with comments or trailing commas is never rewritten: when it already holds the same entry the tool counts as installed, otherwise the command prints the snippet to paste by hand. Close the client while `install` runs, since it edits a file the client also writes. |
| Output | `<tool>... configured.` for a new registration, `<tool>... updated.` when an existing one was replaced (or already matched), `<tool>... manual step needed: <instructions>` for a commented (JSONC) file, `<tool>... failed: <error>` on any other error (including a config file that cannot be read or parsed); each warning follows on its own line as `  Warning: <text>`. |
| Exit code | 0 when finished, including "No supported AI tools detected", `Skipped: <tool> not detected.` and `Skipped: <tool> (<reason>).` for a tool unavailable on the current OS; 1 on an unknown target, when any tool failed or needs a manual step (with or without an explicit target; without one, every other detected tool is still configured), or when stdin ends before a valid Moodle root is given (`no Moodle path provided (stdin closed)`). |

## `uninstall`

```bash
build82 uninstall [target] [--purge]
```

| Item | Value |
| :--- | :--- |
| `target` | Same values as `install`. First argument not starting with `-`. |
| `--purge` | bool, default off. Deletes generated files and `~/.build82` after a separate confirmation. |
| Candidates | Without a target: every tool with a registration, including file-based tools that are no longer detected but whose config file still holds the entry, and config files that cannot be read (reported as failed). Declining the confirmation prints `Nothing removed.` (with `--purge`: `Nothing removed; --purge was skipped too.`). |
| Output | `<tool>... removed.`, `<tool>... not registered.`, `<tool>... nothing removed.` (only a Claude Code `project` registration was found), `<tool>... manual step needed: <instructions>` (commented JSONC file) or `<tool>... failed: <error>`; each warning follows as `  Warning: <text>`. |
| Exit code | 0 on success, cancellation, or nothing found (including a Claude Code `project` registration that was left in place); 1 on an unknown target or when any tool failed or needs a manual step, with or without an explicit target. |

Full behaviour: [Uninstallation](../getting-started/uninstallation.md).

## `self-update`

```bash
build82 self-update [--check] [--channel <name>] [--yes | -y] [--require-signature]
build82 self-update --rollback
```

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--check` | bool | off | Reports whether a newer release exists; installs nothing. Exits **10** when an update is available, 0 when already up to date. |
| `--channel <name>` | string | `stable` | Only `stable` is accepted; any other value fails with `unsupported channel "<name>": only "stable" is currently supported` (exit 1). A missing value is a usage error (exit 2). |
| `--yes`, `-y` | bool | off | Skips the `Replace the running binary with <tag>? [y/N]` prompt. Only `y` (any case) confirms. |
| `--require-signature` | bool | off | Refuses to update, before downloading anything, when no usable cosign (v3 or later on `PATH`) can verify the release signature. Cannot be combined with `--check` (exit 2). |
| `--rollback` | bool | off | Swaps the binary with `<binary>.bak`: the backup must pass `<backup> --version` first (otherwise nothing changes), then the current binary is renamed to `<binary>.bak` and the backup takes its place, so running `--rollback` again swaps them back. Renaming instead of replacing works on Windows, where a running binary cannot be overwritten. Cannot be combined with any other self-update flag (exit 2). Fails with `no backup found at <path> — nothing to roll back` if the backup is missing. On success prints `Rolled back to <version> at <path> (the replaced version is kept at <path>.bak; run --rollback again to undo).` |

Update sequence:

| Step | Detail |
| :--- | :--- |
| 1 | Queries `https://api.github.com/repos/oito2/mcp-build82/releases/latest` (30 s timeout). A 404 prints `No releases found.` and exits 0. A tag that is not `vX.Y.Z` (optionally with a `-pre-release` suffix) is refused (exit 1). |
| 2 | Compares the tag with the current version as `X.Y.Z` (semver precedence: a release outranks the same version with a pre-release suffix such as `-rc1`). A `dev` build is always considered outdated. Not newer: prints `Already on the latest version (<tag>).`, exit 0. |
| 3 | With `--check`, stops after reporting (exit 10 when newer). |
| 4 | Looks for `cosign` on `PATH` and reads `cosign version --json`; only v3 or later is used. Without a usable cosign it prints a warning on stderr and continues with the checksum alone — or, with `--require-signature`, fails here. |
| 5 | Confirmation (unless `--yes`). |
| 6 | Every asset URL must be exactly `https://github.com/oito2/mcp-build82/releases/download/<tag>/<name>`. Downloads go into the binary's directory; each redirect (at most 10) must be `https` on `github.com`, `objects.githubusercontent.com` or `release-assets.githubusercontent.com`, with no port or credentials; each download is limited to 200 MiB and 5 minutes and flushed to disk. |
| 7 | With cosign: downloads `checksums.txt.sigstore.json` and runs `cosign verify-blob checksums.txt --bundle checksums.txt.sigstore.json --certificate-identity https://github.com/oito2/mcp-build82/.github/workflows/release.yml@refs/tags/<tag> --certificate-oidc-issuer https://token.actions.githubusercontent.com`. A failure aborts with cosign's output before the binary is downloaded. |
| 8 | Downloads `build82_<GOOS>_<GOARCH>` (`.exe` on Windows) and verifies its SHA-256 against `checksums.txt`; a mismatch aborts before anything is replaced. |
| 9 | Gives the new binary the current binary's permission bits (owner execute always set), runs `<new binary> --version` (10 s timeout; its output must equal the release tag, otherwise nothing is replaced), renames the current binary to `<binary>.bak`, moves the new binary into place and syncs the directory. If the move fails, the backup is renamed back. |
| Exit code | 0 on success, cancel, already latest; 10 with `--check` when an update is available; 1 on any error; 2 on a usage error. |

The binary path is resolved from the running executable with symlinks followed, so when `build82` is reached through a symlink the file it points to is replaced and the link is kept.

On Windows, a previous `.bak` that is still running (an MCP client started it) cannot be deleted; it is moved aside as `<binary>.bak.old-<n>` and removed by a later update. A rename blocked by another process (for example an antivirus scan) is retried for up to 1 second; if the backup can be neither removed nor moved aside, the error asks you to restart your MCP clients and try again.

Releases published before v1.1.0 carry no `checksums.txt.sigstore.json`. Updating to such a release with cosign installed fails with `release <tag> has no asset named checksums.txt.sigstore.json`; remove cosign from `PATH` for that update.

## Exit codes

| Code | Situation |
| :--- | :--- |
| 0 | Normal completion, `--help`, `--version`, cancelled prompts, graceful HTTP shutdown (also when the 10 s shutdown timeout is exceeded). |
| 10 | `self-update --check` found a newer release. |
| 2 | Usage error: unknown flag or argument, flag missing its value, server-only flag without `--http`, an extra argument to `install`, `uninstall` or `self-update`, or an invalid self-update flag combination (`--rollback` with another flag, `--require-signature` with `--check`). Reported as `Error: <message>` followed by `Run 'build82 <subcommand> --help' for usage.` (or `Run 'build82 --help' for usage.` for server flags) on stderr. |
| 1 | Unknown command; a Ctrl-C at a subcommand prompt (`Interrupted; nothing was changed.`); any error printed as `Error: <message>` by `install`, `uninstall`, `self-update`; token configuration errors; `Fatal error: <message>` from the stdio server, or from the HTTP server failing to bind (`listen on <host>:<port>: <cause>`). |

## Transports

| Transport | Selected by | Endpoints | Authentication | Notes |
| :--- | :--- | :--- | :--- | :--- |
| stdio | no `--http` | — | none | Logs `build82 server running on stdio` to stderr. Runs until the client disconnects. stdout carries the protocol. |
| Streamable HTTP | `--http` | `/mcp` (handled by the Go SDK's Streamable HTTP handler) | Bearer, if a token is set | One MCP server instance per session. |
| SSE | `--http` | `/sse` (message endpoint handled by the same handler) | Bearer, if a token is set | Legacy HTTP+SSE transport. |
| Health | `--http` | `GET /health` | none, exempt from Host check | Returns `200 {"status":"ok"}`. |

Other paths return `404` with `{"error":"Not Found","endpoints":["/mcp","/sse","/health"]}`.

### HTTP server behaviour

| Aspect | Value |
| :--- | :--- |
| Listen address | `<host>:<port>`; defaults `127.0.0.1:3000`. Bind happens before startup messages; failure exits 1. |
| Startup messages (stderr) | `[build82] HTTP server listening on http://<host>:<port>`, `MCP endpoint: http://<host>:<port>/mcp`, `SSE endpoint: http://<host>:<port>/sse`. |
| TLS | None. Plain HTTP only. Use a reverse proxy for HTTPS. |
| Bearer auth | Header `Authorization: Bearer <token>` (prefix is case-sensitive); constant-time comparison. Applies to `/mcp` and `/sse` only. Failure: `401` `{"error":"Unauthorized","message":"Valid Bearer token required. Set Authorization: Bearer <token> header."}`. |
| Host allow-list | `Host` header, port stripped, compared case-insensitively against: `localhost`, `127.0.0.1`, `::1`, the `--host` value, every `--allowed-host`. Not listed: `403` `{"error":"Forbidden","message":"Host \"<host>\" is not allowed. Pass --allowed-host to permit it."}`. `/health` is exempt. Evaluated before auth. |
| No-token warning | `[build82] WARNING: no token set — /mcp and /sse are UNAUTHENTICATED` on every start without a token. |
| Non-loopback warning | Additionally, when there is no token and `--host` is not `127.0.0.1`, `localhost` or `::1` (case-insensitive): `[build82] WARNING: host "<host>" is not loopback — the MCP server is exposed to the network with no authentication`. |
| `ReadHeaderTimeout` | 10 s |
| `IdleTimeout` | 120 s |
| `WriteTimeout` | Not set (long-lived streams). |
| Shutdown | On `SIGINT` or `SIGTERM`: prints `[build82] shutting down...`, waits up to 10 s for connections to end, then closes them. |

Example:

```bash
export BUILD82_TOKEN="$(openssl rand -hex 32)"
build82 --http --port 3000
curl http://127.0.0.1:3000/health
curl -H "Authorization: Bearer $BUILD82_TOKEN" http://127.0.0.1:3000/mcp
```

## Environment variables read

| Variable | Read by | Reference |
| :--- | :--- | :--- |
| `BUILD82_TOKEN` | `--http` | [Configuration Reference](./configuration.md#environment-variables) |
| `BUILD82_MOODLE_PATH`, `BUILD82_MOODLE_VERSION`, `BUILD82_MOODLE_FULLVERSION` | Server (all transports) | [Configuration Reference](./configuration.md#environment-variables) |
| `BUILD82_EXTRACTOR_BACKEND` | Server (all transports) | [Configuration Reference](./configuration.md#environment-variables) |
| `HOME` / `USERPROFILE` | Locating `~/.build82` and client configuration files | [Configuration Reference](./configuration.md#the-configuration-file) |
| `APPDATA` | Windows only, configuration paths of Claude Desktop, Zed and the Cline VS Code extension | [Uninstallation](../getting-started/uninstallation.md#build82-uninstall) |
| `LOCALAPPDATA` | Windows only, Claude Desktop's MSIX package directories | [Configuration Reference](./configuration.md#environment-variables) |
| `CLINE_DIR`, `CLINE_DATA_DIR` | `install`/`uninstall`, Cline CLI directories | [Configuration Reference](./configuration.md#environment-variables) |
| `XDG_CONFIG_HOME` | Linux only, base of the Claude Desktop configuration directory (default `~/.config`) | [Uninstallation](../getting-started/uninstallation.md#build82-uninstall) |
