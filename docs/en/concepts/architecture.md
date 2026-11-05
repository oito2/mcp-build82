🌐 [Português](../../pt-br/concepts/architecture.md) | **English** | 🏠 [Index](../index.md)

---

# Architecture

Overview of the components of `build82`, the data flow, and the operating modes.

---

## Project structure

```
mcp-build82/
├── go.mod, go.sum        ← module definition and dependencies
├── CONTRIBUTING.md        ← release process, cross-compilation, checksums
├── docs/                 ← full documentation (en/ and pt-br/)
├── scripts/release/      ← cross-compiles every release binary + build82.mcpb (MCPB bundle) + checksums.txt + server.json (MCP Registry manifest) into dist/
├── testdata/moodle/      ← fixture Moodle tree used by integration tests
├── cmd/build82/      ← main.go — CLI dispatch: stdio server (default), install, self-update, uninstall
└── internal/
    ├── server/           ← MCP server wiring (tools + resources + prompts)
    ├── tools/            ← 15 MCP tools
    ├── resources/        ← 13 global + 1 aggregate + 12 per-plugin resource templates
    ├── prompts/          ← 4 MCP prompt templates
    ├── extractors/       ← reads and parses Moodle PHP/XML files (regex backend + tsbackend/ tree-sitter backend)
    ├── generators/       ← writes context .md files under .build82/ (global + per-plugin) and migrates legacy flat files
    ├── cache/            ← mtime cache, persisted to .build82/.cache.json
    ├── watcher/          ← automatic regeneration via fsnotify (debounced, dev plugins only)
    ├── config/           ← config loader: env vars → ~/.build82
    ├── installer/        ← `install`/`uninstall` — configures/removes the MCP client registration
    ├── selfupdate/       ← `self-update` — checks GitHub Releases, verifies signature (cosign) and checksum, atomic binary swap
    ├── moodletype/       ← plugin type ↔ directory map, path resolution and containment checks
    ├── phpcs/            ← detects phpcs with the moodle standard and runs it (JSON report) for check_coding_style and doctor
    ├── phparray/         ← shared PHP array/string literal parsing helpers
    ├── phpdoc/           ← shared PHPDoc parsing (visibility, @deprecated, @since...)
    ├── phptypes/         ← data-shape types shared by both extractor backends (type definitions only, no logic)
    ├── legacyhooks/      ← legacy callback → Hook API replacement map
    ├── genutil/          ← shared generator helpers: write + cache mark, header, timestamp, Safely error boundary
    ├── binpath/, fsutil/, toolutil/, version/  ← small shared utilities (binary path, atomic file I/O, tool results, version string)
    └── */                ← one `_test.go` suite per package (go test -race ./...)
```

---

## Data flow

**Entry point.** `cmd/build82/main.go` dispatches on the first argument: no arguments (or an unknown flag) starts the MCP server over **stdio**; `install`, `self-update` and `uninstall` run their own one-shot flows and exit. The server calls `server.NewServer()`, which registers the 15 tools, the resources (13 global + 1 aggregate + 12 per-plugin templates) and the 4 prompts, and reports a 64×64 PNG icon in `serverInfo` as an embedded data URI. Every handler is wrapped in a panic-recovery wrapper (`withRecover`, `withRecoverResource`, and the prompt equivalent), so a panic triggered by malformed plugin source becomes an error result instead of killing the process.

**Side effects.** The only things `build82` writes are: the generated `.md` files, the `.indevelopment` marker and `.cache.json` under `.build82/` (Moodle root and plugin roots), `~/.build82` (config), the `tags` file (when `ctags` is available), the ZIP created by `release_plugin`, the plugin skeleton created by `create_plugin_skeleton`, the `version.php` and `db/upgrade.php` edits made by `bump_plugin_version`, and — through `install`/`uninstall`/`self-update` — MCP client configuration files and the binary itself. Extractors never write to disk.

```
1. AI assistant calls init_moodle_context
   └─ extractors/moodledetect.go detects the Moodle version via version.php
   └─ config/config.go saves the path and version to ~/.build82

2. Generators call Extractors
   └─ each extractor reads one type of Moodle PHP file
   └─ generator writes .md under .build82/ (root or plugin directory)
   └─ cache/cache.go checks mtime — skips unchanged files
   └─ genutil.Write saves the file and marks it fresh; the cache is persisted to .build82/.cache.json

3. AI client reads Resources (passively)
   └─ moodle://plugin/local_myplugin → PLUGIN_AI_CONTEXT.md
   └─ moodle://api-index → MOODLE_API_INDEX.md

4. AI client calls Tools (explicitly)
   └─ get_plugin_info → returns the plugin's PLUGIN_AI_CONTEXT.md (or live metadata if it was never generated)
   └─ search_api → searches in MOODLE_API_INDEX.md
   └─ watch_plugins → enables an fsnotify watcher on dev plugin directories (dev plugin = has a .build82/.indevelopment marker)
   └─ every tool or watcher regeneration → notifications/resources/updated to the sessions subscribed to the rewritten resources

5. AI client uses Prompts
   └─ scaffold_plugin → injects Moodle context + few-shot template
   └─ review_plugin → injects plugin context + review checklist
   └─ debug_plugin → injects context + keyword-matched Moodle debugging hints
   └─ upgrade_plugin → injects deprecated calls, legacy callbacks and upgrade-step findings + migration steps
```

---

## Configuration resolution

The server determines the Moodle path following this priority order:

```
BUILD82_MOODLE_PATH environment variable
         │ (highest priority)
         ▼
~/.build82 file (written by init_moodle_context)
```

`init_moodle_context`'s own `moodle_path` parameter is what *writes* the `~/.build82` file in the first place — every other tool then reads whichever of the two sources above resolves.

> **Note about `BUILD82_MOODLE_VERSION`:** the two sources are never merged. When `BUILD82_MOODLE_PATH` is set (as `build82 install` does), the version comes only from `BUILD82_MOODLE_VERSION` and `BUILD82_MOODLE_FULLVERSION` (the numeric `$version` of `version.php`, e.g. `2024100700`), which are empty when unset; `~/.build82` is not read. `init_moodle_context` and `update_indexes` still detect the version from `version.php` for the indexes they generate. See details in [Installation](../getting-started/installation.md).

---

## Transport

build82 runs over **stdio** only: the AI client starts `build82` as a subprocess and exchanges MCP messages over its standard input and output. No network port is opened. This fits its use as a local development tool, next to the Moodle source and the AI client; with Moodle in Docker, build82 runs on the host and reads the mounted files (see [Docker](../guides/environments/docker.md)). The HTTP mode (`--http`) of earlier versions was removed in v1.2.0.

---

## Main dependencies

| Dependency | Why it is used |
| --- | --- |
| `github.com/modelcontextprotocol/go-sdk` | The official MCP Go SDK: protocol handling, the stdio transport, tool/resource/prompt registration |
| `github.com/odvcencio/gotreesitter` | Pure-Go (no cgo) tree-sitter runtime with the PHP grammar, used only by the opt-in `tsbackend` extractor backend |
| `github.com/fsnotify/fsnotify` | Filesystem notifications for `watch_plugins` (automatic regeneration on save) |

Everything else (`net/http`, `encoding/xml`, `regexp`, `sync`, ...) comes from the Go standard library; the remaining entries in `go.mod` are indirect dependencies of the SDK.

---

## Key design decisions

- **Extractors are pure, generators own the writes.** Extractors take a path and return structured data with no MCP dependency and no disk writes, which keeps them testable in isolation and lets any generator reuse them.
- **Two extractor backends behind one contract.** The regex backend is the default (fast, low memory); tree-sitter (`BUILD82_EXTRACTOR_BACKEND=treesitter`) is opt-in because it is far slower and heavier, in exchange for correctness on edge cases. Shared data types live in `internal/phptypes`, so both backends return the same Go types.
- **All output lives under `.build82/`.** Generated files never mix with plugin source; `MigrateLegacyFiles` moves files left by older releases into `.build82/` on every pass.
- **mtime cache persisted to disk.** A CLI-style server is restarted often, so marks survive restarts in `.build82/.cache.json` and unchanged files are not re-parsed.
- **Concurrency with one shared, mutex-guarded cache.** Independent generators (and plugin extractors) run in parallel; batch tools load and save the cache once per batch.
- **Fault isolation.** `genutil.Safely` isolates each generator, and the panic-recovery wrappers isolate each tool, resource and prompt handler, so one malformed plugin never takes down the server.
- **Single static binary.** No runtime to install; `install` registers it in 8 AI tools and `self-update` replaces it atomically after verifying a checksum.
- **Safe by default over HTTP.** Loopback bind by default, Host-header validation, constant-time Bearer comparison, and an empty token is a configuration error rather than silently disabling auth.

---

## See also

- [How build82 works](./how-build82-works.md) — complete Extractors and Generators pipeline
- [Extractors](../architecture/extractors.md) — how PHP files are read and interpreted
- [Generators](../architecture/generators.md) — how context `.md` files are generated
- [Cache System](../architecture/cache-system.md) — cache strategy and invalidation
- [Glossary](./glossary.md) — project terms (dev plugin, `.indevelopment`, backend, ...)

---

[🏠 Back to Index](../index.md)
