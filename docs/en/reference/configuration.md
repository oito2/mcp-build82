🌐 [Português](../../pt-br/reference/configuration.md) | **English** | 🏠 [Index](../index.md)

---

# Configuration Reference

Source of truth: `internal/config/config.go`, `cmd/build82/main.go`, `internal/extractors/backend.go`, `internal/tools/release.go`, `internal/genutil/genutil.go`.

## The configuration file

| Property | Value |
| :--- | :--- |
| Path | `.build82` in the user's home directory (`os.UserHomeDir`): `~/.build82` on Linux and macOS, `%USERPROFILE%\.build82` on Windows. If the home directory cannot be resolved, commands that need it fail with `resolve home directory: <cause>`. |
| Written by | The `init_moodle_context` tool (only when not yet initialized, or with `force: true`) and `update_indexes` (only when the detected Moodle version differs from the stored one). Never written by the CLI subcommands. |
| Read by | Every tool, resource and prompt that needs the Moodle root; `build82 uninstall --purge`. |
| Deleted by | `build82 uninstall --purge`. See [Uninstallation](../getting-started/uninstallation.md). |
| Write method | Atomic replace, mode `0644`. No cross-process lock; with concurrent writers the last one wins. |

### Format

Plain text, one `KEY=VALUE` per line, no quotes, no sections. Exactly three lines are written, in this order:

```text
MOODLE_PATH=/var/www/moodle
MOODLE_VERSION=4.5
MOODLE_FULLVERSION=2024100700
```

| Key | Required | Description |
| :--- | :---: | :--- |
| `MOODLE_PATH` | Yes | Absolute path to the Moodle root. If missing or empty, the whole file is treated as absent. |
| `MOODLE_VERSION` | No | Moodle version, e.g. `4.5`, taken from the start of `$release` in the Moodle root's `version.php`. Empty string when not detected. |
| `MOODLE_FULLVERSION` | No | Numeric build number: the `$version` value of the Moodle root's `version.php`, e.g. `2024100700`. Empty string when not detected. |

Parsing rules when reading:

| Rule | Detail |
| :--- | :--- |
| Line selection | Lines without `=` are skipped; lines whose first non-blank character is `#` are skipped. |
| Split | On the first `=`; key and value are trimmed of surrounding whitespace. |
| Duplicates | The last occurrence of a key wins. |
| Unknown keys | Ignored. |
| Unreadable file | A read error other than "does not exist" prints `build82: warning: could not read config file <path>: <error>` to stderr and is treated as no configuration. |
| Writing | A value containing `\n` or `\r` is refused: `config value contains a newline, refusing to write`. |

## Environment variables

| Variable | Used by | Type | Default | Description |
| :--- | :--- | :--- | :--- | :--- |
| `BUILD82_MOODLE_PATH` | Server | string (absolute path) | unset | Moodle root. When set to a non-blank value (surrounding whitespace is trimmed), the configuration file is not read at all. `build82 install` writes it into each client entry's `env`. |
| `BUILD82_MOODLE_VERSION` | Server | string | empty | Moodle version. Read only when `BUILD82_MOODLE_PATH` is set. Whitespace trimmed. |
| `BUILD82_MOODLE_FULLVERSION` | Server | string | empty | Numeric build number (the `$version` of `version.php`, e.g. `2024100700`). Read only when `BUILD82_MOODLE_PATH` is set. Whitespace trimmed. |
| `BUILD82_EXTRACTOR_BACKEND` | Server | string | unset | Exactly `treesitter` selects the tree-sitter PHP extractor backend. Any other value, or unset, selects the regex backend. Case-sensitive; evaluated on every extraction call. See [Extractors](../architecture/extractors.md). |
| `BUILD82_TOKEN` | `--http` | string | unset | Bearer token for `/mcp` and `/sse`. See the semantics below. |
| `CLINE_DATA_DIR` | `install`, `uninstall` | string (absolute path) | unset | The Cline CLI's own data directory, which replaces `~/.cline/data`. When set to a non-empty value, the `cline` target detects the Cline CLI by this directory and writes/removes `$CLINE_DATA_DIR/settings/cline_mcp_settings.json` instead of `~/.cline/data/settings/cline_mcp_settings.json`. See [Cline](../guides/clients/cline.md). |

### `BUILD82_TOKEN` semantics

| State | Effect |
| :--- | :--- |
| Unset, and no `--token` flag | Authentication disabled (warning on stderr). |
| Set to a non-empty value, no `--token` flag | The value is the token. |
| Set but empty (`export BUILD82_TOKEN=`), no `--token` flag | Startup error, exit code 1: `BUILD82_TOKEN is set but empty; unset it entirely to run --http without authentication`. |
| Any value, with `--token <non-empty>` | The flag wins; the variable is ignored. |
| Any value, with `--token ""` | Startup error, exit code 1 (the flag is checked first). |

The value is not trimmed. Only read with `--http`; ignored for stdio. Full flow: [CLI Reference](./cli.md#token-resolution).

## Precedence

| Setting | Order (highest first) |
| :--- | :--- |
| Moodle root, version, full version | 1. `BUILD82_MOODLE_PATH` (with its `BUILD82_MOODLE_VERSION` and `BUILD82_MOODLE_FULLVERSION`) 2. `~/.build82`. |
| HTTP Bearer token | 1. `--token` 2. `BUILD82_TOKEN` 3. none (disabled). |
| Port, host, allowed hosts | Flags only (`--port`, `--host`, `--allowed-host`); no environment variable or file. |
| Extractor backend | `BUILD82_EXTRACTOR_BACKEND` only. |

Behaviour to be aware of:

| Case | Result |
| :--- | :--- |
| `BUILD82_MOODLE_PATH` set | The source is the environment as a whole. Values are never merged with the file: `MOODLE_VERSION` in the file is ignored, even if `BUILD82_MOODLE_VERSION` is unset. |
| Neither source present | No configuration; tools report that build82 is not initialized. |
| `init_moodle_context` with a configuration already present (from either source) | Returns "already initialized" and changes nothing, unless `force: true`. |
| `update_indexes` when the detected version differs | Rewrites `~/.build82` with the configured `MoodlePath` (the environment's path when environment-sourced) and the detected versions. |

## Output directory: `.build82/`

Everything build82 generates goes under a `.build82/` directory, at the Moodle root (global files, `tags`, `.cache.json`) and at each plugin root (per-plugin files, `.indevelopment`). Nothing is written directly at either root. The directory name is fixed and not configurable. File list and descriptions: [Generated Files](./generated-files.md).

## Marker and ignore files

| File | Location | Read by | Format and effect |
| :--- | :--- | :--- | :--- |
| `.indevelopment` | `<plugin_root>/.build82/.indevelopment` | Tools that list or batch dev plugins, `doctor`, `update_indexes` (`include_plugins`), the file watcher, and `uninstall --purge` | Marker written by `generate_plugin_context` (always) and by `plugin_batch` (mode `dev`, or with `mark_as_dev`). Content: a timestamp. A plugin counts as in development if the file exists inside a `.build82` directory; its content is not parsed. Excluded from `release_plugin` ZIPs. |
| `.buildignore` | `<plugin_root>/.buildignore` (optional) | `release_plugin` | One name per line; blank lines and lines starting with `#` are skipped; each line is trimmed. Names are matched exactly against file or directory basenames at any depth (no globs, no paths). Additive to the fixed exclusion set. A missing file is not an error. The file itself is never packaged. |

Fixed `release_plugin` exclusions (basenames, any depth): `.build82`, `CLAUDE.md`, `GEMINI.md`, `AGENTS.md`, `.claudeignore`, `.geminiignore`, `.aiexclude`, `node_modules`, `.buildignore`, `.git`, `.indevelopment`, and the 12 `PLUGIN_*.md` file names.

## Related

- [CLI Reference](./cli.md)
- [Generated Files](./generated-files.md)
- [Uninstallation](../getting-started/uninstallation.md)
