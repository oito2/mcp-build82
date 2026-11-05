🌐 [Português](../../pt-br/reference/cli.md) | **English** | 🏠 [Index](../index.md)

---

# CLI Reference

Binary: `build82`. Source of truth: `cmd/build82/main.go`.

## Commands

The subcommand is read from the first argument only.

| Invocation | Description |
| :--- | :--- |
| `build82` | Runs the MCP server over stdio. |
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
| `install`, `uninstall`, `self-update`, `--help`, `-h`, `--version` | As listed above. |
| Any other value starting with `-` | Checked as a server argument (see [Server](#server)): the server takes none, so it is a usage error (exit code 2). |
| Any other value | `Error: unknown command "<value>"`, followed by the help text, on stderr. Exit code 1. |

### Subcommand help, `--` and interrupts

| Rule | Behaviour |
| :--- | :--- |
| Subcommand help | `-h` or `--help` anywhere among the arguments of `install`, `uninstall` or `self-update` — before a `--` — prints the subcommand's help and exits 0; nothing else is parsed or run (`build82 self-update --check --help` only prints help). |
| `--` | Ends the options. For `install` and `uninstall`, every argument after it is the target, even one starting with `-` (`build82 uninstall -- --help` looks up a target named `--help`). `self-update` and the server take no positional argument, so anything after `--` is a usage error; a trailing `--` alone is accepted. |
| Usage-error pointer | A usage error of `install`, `uninstall` or `self-update` ends with `Run 'build82 <subcommand> --help' for usage.`; an unknown top-level flag with `Run 'build82 --help' for usage.` |
| Ctrl-C at a prompt | The first SIGINT or SIGTERM while `install`, `uninstall` (including the `--purge` confirmation) or `self-update` waits for an answer prints `Interrupted; nothing was changed.` on stderr and exits with code 1. During `self-update` it also aborts a download in progress (before the binary is replaced). Between two targets of `install`/`uninstall` without a target, the remaining targets are skipped and the command exits with code 1. A second Ctrl-C ends the process at once. |

## Server

`build82` with no argument runs the MCP server over **stdio**: the AI client starts the binary and exchanges JSON-RPC messages over its stdin and stdout. It logs `build82 server running on stdio` to stderr and runs until the client disconnects. One inbound message may be up to 64 MiB.

The server takes no flags besides `--help`, `-h` and `--version` (print and exit 0) and a trailing `--`. Any other argument is a usage error: `Error: unknown argument "<arg>"` and `Run 'build82 --help' for usage.` on stderr, exit code 2.

The HTTP mode (`--http`, with `--port`, `--host`, `--token`, `--allowed-host` and the `BUILD82_TOKEN` variable) was removed in v1.2.0. Those flags are usage errors that add `the HTTP mode was removed in v1.2.0; build82 runs over stdio only`.

## `install`

```bash
build82 install [target]
```

| Item | Value |
| :--- | :--- |
| Flags | None. At most one positional target; any flag or second argument is a usage error (exit 2). |
| `target` | `claude` (Claude Code), `claude-desktop`, `antigravity`, `codex`, `opencode`, `cursor`, `zed`, `cline`. Omitted: all detected tools. |
| Prompts | Moodle root path (offers the current directory when it is a Moodle root; validated as a Moodle root), and `Install build82 into all N detected tool(s)? [y/N]` when no target is given. |
| Writes | The `build82` entry in each tool's configuration, with `BUILD82_MOODLE_PATH` in its environment and the resolved absolute path of the running binary as the command. `claude` runs `claude mcp add --scope user build82 -e BUILD82_MOODLE_PATH=<path> -- <binary>`; `codex` runs `codex mcp add build82 --env BUILD82_MOODLE_PATH=<path> -- <binary>`. For both, an existing registration reported by `<tool> mcp get build82` is removed first (for `claude`: every `user` and `local` registration; a `project` registration in `.mcp.json` is left unchanged and reported with a warning, see [Claude Code](../guides/clients/claude-code.md)). The other targets merge the entry into their JSON file (see [client guides](../guides/clients/claude-code.md)); `claude-desktop` writes every Claude Desktop configuration directory that exists (on Windows, also those of the MSIX package); `cline` writes every Cline settings file that exists (the CLI one under `CLINE_DATA_DIR` or `CLINE_DIR` when set). Every other key, the key order and each value are kept as written, as are the file's mode and a symbolic link at its path. When a `build82` entry already exists, only `command`, `args` and `BUILD82_MOODLE_PATH` are replaced: other keys of the entry and other environment variables in it are kept, and an entry that already holds those values leaves the file untouched. Each `claude`/`codex` command is given 60 s to finish. On Windows, when that CLI is a `.cmd`/`.bat` script, a Moodle path or binary path containing `& | < > ^ % ! "` or a line break is refused before anything runs. If the new registration fails after the old one was removed, the error says so and tells you to run `install` again. A file with comments or trailing commas is never rewritten: when it already holds the same entry the tool counts as installed, otherwise the command prints the snippet to paste by hand. Close the client while `install` runs, since it edits a file the client also writes. |
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
| Errors | When `<tool> mcp get build82` fails with anything other than a "not registered" answer, the tool is reported as failed instead of not registered. If the `--purge` confirmation is interrupted after registrations were removed, the command fails with `purge interrupted; the build82 registrations reported above were already removed (<n> failed): <cause>`. |
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
| `--rollback` | bool | off | Swaps the binary with `<binary>.bak`: the backup must pass `<backup> --version` first (otherwise nothing changes), then the current binary is renamed to `<binary>.bak` and the backup takes its place, so running `--rollback` again swaps them back. Renaming instead of replacing works on Windows, where a running binary cannot be overwritten. Cannot be combined with any other self-update flag (exit 2). Fails with `no backup found at <path> — nothing to roll back` if the backup is missing. On success prints `Rolled back to <version> at <path> (the replaced version is kept at <path>.bak; run --rollback again to undo).` If putting the backup in place fails and the current binary cannot be restored either, the error ends with `(the backup is now at <path> — rename it back to <path>: <cause>)`. |

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
| 0 | Normal completion, `--help`, `--version`, cancelled prompts. |
| 10 | `self-update --check` found a newer release. |
| 2 | Usage error: unknown flag or argument (including the flags of the removed HTTP mode), flag missing its value, an extra argument to `install`, `uninstall` or `self-update`, or an invalid self-update flag combination (`--rollback` with another flag, `--require-signature` with `--check`). Reported as `Error: <message>` followed by `Run 'build82 <subcommand> --help' for usage.` (or `Run 'build82 --help' for usage.` for a top-level flag) on stderr. |
| 1 | Unknown command; a Ctrl-C at a subcommand prompt (`Interrupted; nothing was changed.`); any error printed as `Error: <message>` by `install`, `uninstall`, `self-update`; `Fatal error: <message>` from the stdio server. |

## Environment variables read

| Variable | Read by | Reference |
| :--- | :--- | :--- |
| `BUILD82_MOODLE_PATH`, `BUILD82_MOODLE_VERSION`, `BUILD82_MOODLE_FULLVERSION` | Server | [Configuration Reference](./configuration.md#environment-variables) |
| `BUILD82_EXTRACTOR_BACKEND` | Server | [Configuration Reference](./configuration.md#environment-variables) |
| `HOME` / `USERPROFILE` | Locating `~/.build82` and client configuration files | [Configuration Reference](./configuration.md#the-configuration-file) |
| `APPDATA` | Windows only, configuration paths of Claude Desktop, Zed and the Cline VS Code extension | [Uninstallation](../getting-started/uninstallation.md#build82-uninstall) |
| `LOCALAPPDATA` | Windows only, Claude Desktop's MSIX package directories | [Configuration Reference](./configuration.md#environment-variables) |
| `CLINE_DIR`, `CLINE_DATA_DIR` | `install`/`uninstall`, Cline CLI directories | [Configuration Reference](./configuration.md#environment-variables) |
| `XDG_CONFIG_HOME` | Linux only, base of the Claude Desktop configuration directory (default `~/.config`) | [Uninstallation](../getting-started/uninstallation.md#build82-uninstall) |
