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
| `build82 --version` | Prints the version (`dev` for a build without release ldflags) and exits with code 0. |

Argument resolution for the first argument:

| First argument | Behaviour |
| :--- | :--- |
| None | stdio server. |
| `--http`, `install`, `uninstall`, `self-update`, `--help`, `-h`, `--version` | As listed above. |
| Any other value starting with `-` | Treated as server flags (see below). Without `--http` among the arguments, the stdio server starts and every server flag is ignored silently. |
| Any other value | `Error: unknown command "<value>"`, followed by the help text, on stderr. Exit code 1. |

## Server flags

Parsed by a manual walk of the arguments, in any order (`build82 --port 8080 --http` works). Unrecognised arguments are ignored. `--help`, `-h` and `--version` are also honoured inside this walk (print and exit 0).

| Flag | Type | Default | Applies to | Description |
| :--- | :--- | :--- | :--- | :--- |
| `--http` | bool | off | — | Selects the HTTP transport. Without it, all other server flags have no effect. |
| `--port <n>` | integer 1–65535 | `3000` | `--http` | TCP port. An invalid value prints `warning: invalid --port value "<v>", keeping <n>` to stderr and keeps the previous value. A missing value prints `warning: --port requires a value, ignoring`. The next argument is always consumed as the value. |
| `--host <host>` | string | `127.0.0.1` | `--http` | Bind address. Also added to the Host allow-list. Missing value: warning, ignored. |
| `--token <token>` | string | none (auth disabled) | `--http` | Bearer token required on `/mcp` and `/sse`. Last occurrence wins. Missing value: warning, ignored. |
| `--allowed-host <host>` | string, repeatable | none | `--http` | Extra value accepted in the `Host` header. Missing value: warning, ignored. |

### Token resolution

Applies with `--http`, evaluated in this order:

| # | Condition | Result |
| :--- | :--- | :--- |
| 1 | `--token <value>` passed, value not empty | Use the value. `BUILD82_TOKEN` is ignored. |
| 2 | `--token ""` passed | Error: `--token was passed with an empty value; omit --token entirely to run --http without authentication`. Exit code 1. |
| 3 | `--token` passed without a following argument | Warning; treated as if not passed, so continues to 4. |
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
| Flags | None. `argv[2]` is taken as the target, whatever its value. |
| `target` | `claude` (Claude Code), `claude-desktop`, `antigravity`, `codex`, `opencode`, `cursor`, `zed`, `cline`. Omitted: all detected tools. |
| Prompts | Moodle root path (offers the current directory when it is a Moodle root; validated as a Moodle root), and `Install build82 into all N detected tool(s)? [y/N]` when no target is given. |
| Writes | The `build82` entry in each tool's configuration, with `BUILD82_MOODLE_PATH` in its environment and the resolved absolute path of the running binary as the command. `claude` runs `claude mcp add --scope user build82 -e BUILD82_MOODLE_PATH=<path> -- <binary>`; `codex` runs `codex mcp add build82 --env BUILD82_MOODLE_PATH=<path> -- <binary>`. For both, an existing registration reported by `<tool> mcp get build82` is removed first (for `claude`: every `user` and `local` registration; a `project` registration in `.mcp.json` is left unchanged and reported with a warning, see [Claude Code](../guides/clients/claude-code.md)). The other targets merge the entry into their JSON file (see [client guides](../guides/clients/claude-code.md)); `cline` writes every Cline settings file that exists (the CLI one under `CLINE_DATA_DIR` when set), and a file with comments or trailing commas is never rewritten (the command prints the snippet to paste by hand and fails). |
| Output | `<tool>... configured.` for a new registration, `<tool>... updated.` when an existing one was replaced, `<tool>... failed: <error>` on error; each warning follows on its own line as `  Warning: <text>`. |
| Exit code | 0 when finished, including "No supported AI tools detected", `Skipped: <tool> not detected.` and `Skipped: <tool> (<reason>).` for a tool unavailable on the current OS; 1 on an unknown target or on an error while installing into an explicit target. Without a target, a per-tool failure is printed (`<tool>... failed: <error>`) and the exit code stays 0. |

## `uninstall`

```bash
build82 uninstall [target] [--purge]
```

| Item | Value |
| :--- | :--- |
| `target` | Same values as `install`. First argument not starting with `-`. |
| `--purge` | bool, default off. Deletes generated files and `~/.build82` after a separate confirmation. |
| Output | `<tool>... removed.`, `<tool>... not registered.`, `<tool>... nothing removed.` (only a Claude Code `project` registration was found) or `<tool>... failed: <error>`; each warning follows as `  Warning: <text>`. |
| Exit code | 0 on success, cancellation, or nothing found (including a Claude Code `project` registration that was left in place); 1 on an unknown target or on a failure with an explicit target. |

Full behaviour: [Uninstallation](../getting-started/uninstallation.md).

## `self-update`

```bash
build82 self-update [--check] [--channel <name>] [--yes | -y] [--rollback]
```

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--check` | bool | off | Reports whether a newer release exists; installs nothing. Exit code 0 either way. |
| `--channel <name>` | string | `stable` | Only `stable` is accepted; any other value fails with `unsupported channel "<name>": only "stable" is currently supported` (exit 1). A missing value is ignored silently. |
| `--yes`, `-y` | bool | off | Skips the `Replace the running binary with <tag>? [y/N]` prompt. Only `y` (any case) confirms. |
| `--rollback` | bool | off | Renames `<binary>.bak` over the binary, then runs `--version` on it as a diagnostic. If it appears anywhere in the arguments, all other self-update flags are ignored. Fails with `no backup found at <path> — nothing to roll back` if the backup is missing. On success prints `Rolled back to previous version at <path>.` |

Update sequence:

| Step | Detail |
| :--- | :--- |
| 1 | Queries `https://api.github.com/repos/oito2/mcp-build82/releases/latest` (30 s timeout). A 404 prints `No releases found.` and exits 0. |
| 2 | Compares the tag with the current version as `X.Y.Z` (leading `v` and any `-`/`+` suffix ignored). A `dev` build is always considered outdated; an unparsable tag never triggers an update. Not newer: prints `Already on the latest version (<tag>).`, exit 0. |
| 3 | Confirmation (unless `--yes`), or stop after reporting with `--check`. |
| 4 | Downloads `checksums.txt` and the asset `build82_<GOOS>_<GOARCH>` (`.exe` on Windows) into the binary's directory. URLs must be `https` on `github.com` or `*.githubusercontent.com`; redirects to non-https are refused; each download is limited to 200 MiB and 5 minutes. |
| 5 | Verifies the SHA-256 against `checksums.txt`; a mismatch aborts before anything is replaced. |
| 6 | Sets mode `0755`, runs `<new binary> --version` (10 s timeout), renames the current binary to `<binary>.bak`, then moves the new binary into place. |
| Exit code | 0 on success, cancel, already latest; 1 on any error. |

The binary path is resolved from the running executable with symlinks followed.

## Exit codes

| Code | Situation |
| :--- | :--- |
| 0 | Normal completion, `--help`, `--version`, cancelled prompts, graceful HTTP shutdown (also when the 10 s shutdown timeout is exceeded). |
| 1 | Unknown command; any error printed as `Error: <message>` by `install`, `uninstall`, `self-update`; token configuration errors; `Fatal error: <message>` from the stdio server, or from the HTTP server failing to bind (`listen on <host>:<port>: <cause>`). |

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
| `XDG_CONFIG_HOME` | Linux only, base of the Claude Desktop configuration directory (default `~/.config`) | [Uninstallation](../getting-started/uninstallation.md#build82-uninstall) |
