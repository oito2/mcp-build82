# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.1.0] - 2026-10-09

### Added

- `self-update` verifies the Sigstore signature of the release's `checksums.txt` with
  [cosign](https://docs.sigstore.dev/cosign/system_config/installation/) v3 or later when it is on
  `PATH`, requiring the exact identity of this repository's release workflow for the tag. Without
  a usable cosign it prints a warning and relies on the checksum alone.
- `self-update --require-signature` refuses to update, before downloading anything, when no usable
  cosign is available.
- `install claude-desktop` also configures Claude Desktop's Windows MSIX package (the claude.ai
  download and the Microsoft Store), which reads
  `%LOCALAPPDATA%\Packages\Claude_<id>\LocalCache\Roaming\Claude\claude_desktop_config.json`.
- The `cline` target honors `CLINE_DIR`, the Cline CLI's configuration directory, in place of
  `~/.cline`.
- `build82 install --help`, `build82 uninstall --help` and `build82 self-update --help` (or `-h`)
  print that subcommand's usage and options without running it.
- `--` ends the options of `install` and `uninstall`, so a target starting with `-` can be passed.
- A Ctrl-C while `install`, `uninstall` or `self-update` waits at a prompt exits at once with
  `Interrupted; nothing was changed.` and code 1. During `self-update` it also aborts a download in
  progress.
- The 9 tools with a `format` parameter (`init_moodle_context`, `generate_plugin_context`,
  `plugin_batch`, `update_indexes`, `search_plugins`, `search_api`, `get_plugin_info`,
  `list_dev_plugins`, `doctor`) declare an `outputSchema` and return the matching
  `structuredContent` on success, in either format. Clients that show `structuredContent` to the
  model (such as Claude Code) now get the real data instead of an empty object.
- Every tool declares explicit annotations (title, `readOnlyHint`, `destructiveHint`,
  `idempotentHint`, `openWorldHint`).
- A native Windows on Arm binary, `build82_windows_arm64.exe`, is published with every release
  (`self-update` installs it on that platform).
- Releases are signed: `checksums.txt.sigstore.json` is a keyless Sigstore signature of
  `checksums.txt` made by the release workflow, and every binary and `build82.mcpb` carries a GitHub
  build provenance attestation (`gh attestation verify`).

### Changed

- `self-update --check` exits with code **10** when a newer release is available (0 when up to
  date), so scripts can tell the two apart.
- `self-update --rollback` combined with any other self-update flag, and `--require-signature`
  combined with `--check`, are usage errors (exit 2). Previously the other flags were ignored.
- The updated binary keeps the permission bits of the binary it replaces instead of always getting
  `0755`.
- `install` and `uninstall` keep the order of the keys in a client's JSON configuration file and
  every value as written (only the indentation is normalized to 2 spaces); a new `build82` entry
  goes at the end. A symbolic link at the configuration path is kept, and a UTF-8 byte order mark
  is tolerated.
- A configuration file with comments or trailing commas is reported as `manual step needed`
  instead of `failed`, and counts as installed when it already holds the same entry.
- `install` and `uninstall` without a target exit with code 1 when any client failed or needs a
  manual step. Previously they exited with 0.
- Declining the `uninstall` confirmation prints `Nothing removed.`, and with `--purge` says that
  the purge was skipped too.
- Usage errors of `install`, `uninstall` and `self-update` now point to the subcommand's help
  (`Run 'build82 <subcommand> --help' for usage.`).
- The JSON format of `plugin_batch` is now an object `{mode, plugins[]}` with lower-case keys and
  plugin paths relative to the Moodle root; `list_dev_plugins` returns `{plugins[]}` with
  `component`, `path`, `has_ai_context`; `get_plugin_info` returns one object with the plugin's
  metadata plus `has_ai_context` and, when generated, `ai_context`; `update_indexes` adds
  `moodle_version`. `plugin_batch` in `dev` mode and `list_dev_plugins` with no marked plugin
  return an empty list instead of a text-only answer.
- Failed tool calls return only the error message (no JSON in the `json` format, no
  `structuredContent`); `get_plugin_info` reports "possible matches" for an unknown plugin as an
  error result. `doctor` keeps returning its full report, also when the verdict is `fail`.
- The stdio transport accepts inbound messages of up to 64 MiB.
- `self-update --rollback` swaps the binary with its `.bak` backup instead of discarding the
  current binary: the replaced version becomes the new `.bak`, so running it again undoes the
  rollback. The backup is smoke-tested before anything is renamed.
- Release binaries are built reproducibly (`-trimpath`, empty build ID, pinned build environment):
  the same Go release produces byte-identical binaries locally and in CI.

### Security

- Files read from plugin directories and from the generated `.build82/` files are opened only when
  they are regular files: a symbolic link at the file is not followed, and a FIFO or device is
  refused without blocking, so a planted special file can no longer hang the server or
  `release_plugin`.
- `plugin_batch`'s JSON output no longer includes the absolute path of each plugin.
- `self-update` downloads only from this repository's release of the new tag
  (`https://github.com/oito2/mcp-build82/releases/download/<tag>/<asset>`), and follows at most 10
  redirects, each restricted to `github.com`, `objects.githubusercontent.com` and
  `release-assets.githubusercontent.com`. Other `*.githubusercontent.com` hosts, which serve user
  content, are refused. Release tags that aren't `vX.Y.Z` (optionally with a pre-release suffix)
  are refused.
- Downloads are flushed to disk before verification, and the directory is synced after the binary
  swap.

### Fixed

- `self-update` on Windows: the downloaded binary is staged with an `.exe` name, so its smoke test
  can run it, and `--rollback` renames the running binary instead of overwriting it, which Windows
  refuses.
- The `tags` file is skipped, instead of being reported as a failed generator, when the `ctags` on
  `PATH` is not Universal Ctags (such as macOS's BSD `ctags`); `doctor` warns about it.
- A client configuration file that cannot be read or parsed is reported as `failed` by `install`
  and `uninstall` instead of being treated as having no build82 registration.
- A `mcpServers`/`mcp`/`context_servers` key that holds something other than an object is no longer
  replaced; the file is left unchanged and the client is reported as `failed`.
- A failing `codex mcp get` counts as "not registered" only when Codex says so (`No MCP server
  named ...`); any other failure is reported instead of being ignored.
- On Windows, `self-update` no longer fails when the previous `.bak` is still running: it is moved
  aside as `<binary>.bak.old-<n>` and removed by a later update. A rename briefly blocked by
  another process (for example an antivirus scan) is retried for up to 1 second.

## [1.0.1] - 2026-10-06

### Security

- `release_plugin` no longer follows symbolic links when packaging. Symlinks inside the plugin are
  skipped, so a link that points outside the plugin can't put the file it points to into the
  release ZIP. The report lists every skipped link as a warning.
- `self-update` now refuses a downloaded binary if its `--version` output doesn't match the release
  tag. The installed binary is left untouched.

### Changed

- The CLI now treats usage errors as errors and exits with code **2** instead of ignoring them.
  This covers unknown flags, a flag with a missing value (e.g. `--channel` with no argument), extra
  positional arguments, and server flags (`--port`, `--host`, `--token`, `--allowed-host`) without
  `--http`. Unknown subcommands and runtime errors still exit with code 1.
- `self-update` follows full semantic-version precedence, including pre-releases: `v1.2.3-rc1` is
  older than `v1.2.3`.
- `watch_plugins` watches each plugin's directories instead of individual files. Changes saved by
  editors that write a temporary file and rename it are now detected every time. A change made
  while that plugin is being regenerated triggers one more regeneration instead of being lost.
- An existing `db/subplugins.json` is authoritative. If it is invalid, it no longer silently falls
  back to `db/subplugins.php`.

### Fixed

- `build82 install` with a closed stdin (e.g. `</dev/null`) now fails with a clear error instead of
  prompting for the Moodle path forever.
- A `.build82/.cache.json` with `"entries": null`, or with no `entries` key, no longer crashes the
  next generation.
- `MOODLE_AI_INDEX.md` and `MOODLE_AI_WORKSPACE.md` are now regenerated when a plugin's
  `PLUGIN_AI_CONTEXT.md` or `.indevelopment` marker is created or deleted.
- `search_plugins` no longer hides plugins whose name contains "Component".
- `create_plugin_skeleton` writes the correct `xmldb.xsd` schema path in `db/install.xml` for plugin
  types nested deeper than `type/name` (e.g. `admin/tool`).
- PHP extraction:
  - Commented-out `admin_setting_*` declarations are no longer reported as settings.
  - PHP 8 attributes (`#[...]`) no longer break class scanning.
  - Single-line docblocks (`/** text */`) are parsed.
  - Quoted values that contain escaped or mixed quotes are no longer cut short.
  - Methods inside classes are no longer reported as legacy `lib.php` callbacks.
- Upgrade steps that share the same version, and observers of the same event in
  `MOODLE_EVENTS_INDEX.md`, now always appear in the same order.
- Truncated descriptions and prompt arguments never cut a multi-byte (UTF-8) character in half.
- Legacy-file migration no longer rejects filenames that contain `..` as part of the name
  (e.g. `a..b`).
- Directories created for private files (mode 0600) are no longer world-readable: they now get
  mode 0700 instead of 0755.
- A whitespace-only `BUILD82_MOODLE_PATH` is treated as unset everywhere, consistently.
- `update_indexes` reports plugins by their Moodle-relative path when one fails unexpectedly.
- `self-update` checks for errors when writing the downloaded file to disk.

### Development

- `golangci-lint` v2.14.0 (`.golangci.yml`) is now a required check in CI and in the release
  workflow, and must report zero issues.
- CI and release jobs are pinned to `ubuntu-24.04`.
- Every function and type in the Go source now has an English doc comment. The license holder in
  the source headers is now `OITO2`.

## [1.0.0] - 2026-10-05

### Added

- First public release: the `build82` MCP server for Moodle plugin development. It provides 13
  tools, 13 global resources plus 12 per-plugin resource templates, and 3 prompts, over stdio or
  Streamable HTTP/SSE.
- Regex PHP extraction backend (the default), plus an optional pure-Go tree-sitter backend.
- `install` / `uninstall` for 8 MCP clients, and `self-update` with `--rollback`.
- Release artifacts: 5 binaries, the `build82.mcpb` Claude Desktop extension, `checksums.txt`, and
  the MCP Registry entry `io.github.oito2/mcp-build82`.

[Unreleased]: https://github.com/oito2/mcp-build82/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/oito2/mcp-build82/compare/v1.0.1...v1.1.0
[1.0.1]: https://github.com/oito2/mcp-build82/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/oito2/mcp-build82/releases/tag/v1.0.0
