# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/oito2/mcp-build82/compare/v1.0.1...HEAD
[1.0.1]: https://github.com/oito2/mcp-build82/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/oito2/mcp-build82/releases/tag/v1.0.0
