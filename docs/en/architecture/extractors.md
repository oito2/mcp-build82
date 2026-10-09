🌐 [Português](../../pt-br/architecture/extractors.md) | **English** | 🏠 [Index](../index.md)

---

# Extractors

**Extractors** are the Go packages responsible for reading and interpreting PHP and XML files from the Moodle installation. Each extractor specializes in a specific file type and produces structured data that the [Generators](./generators.md) transform into `.md` context files.

---

## Overview

```
internal/extractors/
├── api.go            ← lib/*.php
├── capabilities.go   ← db/access.php
├── classes.go         ← classes/**/*.php + db/renamedclasses.php
├── events.go          ← db/events.php
├── hooks.go           ← db/hooks.php + classes/hook/*.php + legacy lib.php callbacks
├── moodledetect.go     ← version.php (Moodle root)
├── plugin.go           ← version.php (plugin) + lang/
├── schema.go           ← db/install.xml
├── services.go         ← db/services.php
├── tasks.go            ← db/tasks.php
├── upgrade.go          ← db/upgrade.php
├── settings.go         ← settings.php (admin_setting_* declarations)
├── subplugins.go       ← db/subplugins.json (legacy: db/subplugins.php)
├── deprecated.go       ← doctor: calls to @deprecated core functions in plugin PHP
├── capabilityusage.go  ← doctor: has_capability()/require_capability() checks vs declared capabilities
├── langstrings.go      ← doctor: get_string() calls vs strings declared in lang/en
├── shared.go           ← readFileCapped (8 MiB read cap) and symlink-safe directory walk
├── backend.go          ← useTreesitter() dispatch, read once per call, never cached
└── tsbackend/          ← the opt-in tree-sitter backend, mirror package (see below)
```

All extractors are pure functions: they receive a file or directory path, return structured data, and **never write to disk** — that's exclusively the Generators' job.

---

## Two backends, one contract

The extractors that parse structured PHP — `api.go`, `capabilities.go`, `classes.go`, `events.go`, `hooks.go`, `services.go`, `tasks.go`, `upgrade.go`, plus the `version.php` read in `plugin.go` — have **two interchangeable backends**. The rest are regex/XML only and ignore `BUILD82_EXTRACTOR_BACKEND`: `moodledetect.go` and `schema.go` (header lines and XML — nothing to gain from a real PHP parser), `settings.go`, `subplugins.go`, and the `doctor` diagnostic scans (`deprecated.go`, `capabilityusage.go`, `langstrings.go`).

| Backend | Package | Default | Trade-off |
| --- | --- | --- | --- |
| **regex** | `internal/extractors/*.go` | ✅ Yes | Fast, low memory, a handful of edge cases it can't express (escaped quotes inside strings, multi-line `implements` clauses, PHP 8 attributes between a docblock and a function...) |
| **tree-sitter** | `internal/extractors/tsbackend/*.go` | Opt-in via `BUILD82_EXTRACTOR_BACKEND=treesitter` | ~250x slower and ~100x more memory per call (measured against a real ~10k-line `moodlelib.php` — see `tsbackend/BENCHMARKS.md`), but correctly handles every edge case above |

`backend.go`'s `useTreesitter()` reads the environment variable fresh on every call (never cached), so tests and callers can switch backends mid-process. The two backends differ on a few edge cases, all in tree-sitter's favor; the cross-backend parity tests in `internal/extractors/*_test.go` log each expected difference.

`internal/extractors` imports `tsbackend` for the dispatch, so `tsbackend` cannot import the extractor types back without an import cycle. Both backends therefore return the same Go types, defined once in the neutral `internal/phptypes` package (type definitions only); `internal/extractors` re-exports them as type aliases (e.g. `type ApiFunction = phptypes.ApiFunction`). Two pieces of logic that must stay identical between backends — the legacy-callback-suffix→Hook-API-replacement map, and PHPDoc visibility classification — live in their own shared packages (`internal/legacyhooks`, `internal/phpdoc`) that neither backend imports from the other.

---

## Reference table

| Extractor | Source file | Produced data |
| --- | --- | --- |
| `api.go` | `lib/*.php` (top-level functions only) | Functions with name, line, PHPDoc visibility, `@since`, `@deprecated` |
| `capabilities.go` | `db/access.php` | Capabilities with name, riskbitmask, captype, archetypes |
| `classes.go` | `classes/**/*.php`, `db/renamedclasses.php` | Classes/interfaces/traits/enums with namespace, FQN, kind, extends/implements; renamed-class autoload map |
| `events.go` | `db/events.php` | Observers with eventname, callback, priority, internal flag |
| `hooks.go` | `db/hooks.php` + `classes/hook/*.php` | Hook API callbacks + hook definitions + legacy `lib.php` callback migration warnings |
| `moodledetect.go` | `version.php` (Moodle root) | Moodle version (human-readable and numeric) |
| `plugin.go` | plugin `version.php` + `lang/en/<component>.php` | Component, type, name, version, requires, display name, maturity |
| `schema.go` | `db/install.xml` | Tables, fields, keys, indexes |
| `services.go` | `db/services.php` | Web service functions with classname, methodname, type, ajax, capabilities |
| `tasks.go` | `db/tasks.php` | Scheduled tasks with classname and cron schedule |
| `upgrade.go` | `db/upgrade.php` | Upgrade steps with version and inferred description |
| `settings.go` | `settings.php` | Admin settings: `admin_setting_*` class name and (when a string literal) setting name |
| `subplugins.go` | `db/subplugins.json`, falling back to legacy `db/subplugins.php` | Subplugin types declared by the plugin (type prefix → path) |
| `deprecated.go` | Plugin `*.php` (recursive) + deprecated names from the API index | Call sites of `@deprecated` core functions (used by `doctor`) |
| `capabilityusage.go` | Plugin `*.php` | `has_capability()`/`require_capability()` calls naming the plugin's own capabilities (used by `doctor`) |
| `langstrings.go` | `lang/en/<component>.php` + plugin `*.php` | Declared language strings and `get_string()` calls (used by `doctor`) |

---

## Details by extractor

### `api.go` — Core functions

**Source file:** `lib/*.php`, top-level functions only (not methods inside classes).

**Parsing strategy (regex):** line-by-line scan pairing a preceding `/** ... */` PHPDoc block (tolerating up to 3 blank lines and, since the tree-sitter parity work, a PHP 8 `#[...]` attribute in between) with the following `function` declaration.

**Visibility classification** (shared with the tree-sitter backend via `internal/phpdoc`):

| PHPDoc tag | Assigned visibility |
| --- | --- |
| `@deprecated` | `deprecated` |
| `@internal` | `internal` |
| `@access private` or `@private` | `private` |
| No restrictive tag | `public` |
| No PHPDoc at all | `unverified` |

`ExtractMoodleApi` scans the `.php` files directly under `lib/` (priority files first, not recursive), counts every visibility class, and returns only `public` and `deprecated` functions.

**Feeds:** `MOODLE_API_INDEX.md`.

---

### `schema.go` — Database

**Source file:** `db/install.xml` of any plugin.

**Parsing strategy:** Go's standard `encoding/xml` to decode XMLDB, tolerating malformed individual tables without aborting the whole file. XMLDB's `NOTNULL`/`SEQUENCE`/`NEXT` string attributes (`"true"`/`"false"`) are normalized to real booleans.

**Feeds:** `PLUGIN_DB_TABLES.md` and `MOODLE_DB_TABLES_INDEX.md`.

---

### `events.go` — Event observers

**Source file:** `db/events.php` of any plugin.

**Parsing strategy (regex):** array-literal parsing via the shared `internal/phparray` package, which understands PHP string/comment syntax well enough to find balanced array bodies and extract string/int/bool values without a full parser.

**Feeds:** `PLUGIN_EVENTS.md` and `MOODLE_EVENTS_INDEX.md`.

---

### `hooks.go` — Hook API (Moodle 4.3+)

**Source file:** `db/hooks.php` (registered callbacks) + `classes/hook/*.php` (hook definitions) + the plugin's `lib.php`/`locallib.php` (legacy callback detection).

**Parsing strategy:** three sub-scans — the callbacks array in `db/hooks.php`, class-level metadata in `classes/hook/*.php`, and a regex over `lib.php` matching any of the 12 legacy callback suffixes (`before_footer`, `extend_navigation`, `cron`, ...) that have a documented Hook API replacement in `internal/legacyhooks`.

**Feeds:** `PLUGIN_CALLBACK_INDEX.md` and `PLUGIN_DEPENDENCIES.md`.

---

### `capabilities.go` — Capabilities

**Source file:** `db/access.php` of any plugin.

**Parsing strategy:** array-literal parsing over the `$capabilities` array, keyed by `component:capabilityname`. `riskbitmask` may be a single named constant (e.g. `RISK_SPAM`) or a composite `|`-expression across several — the tree-sitter backend resolves the full expression; the regex backend only captures the first token (documented as a known divergence, not a bug to fix, since it's the regex approach's inherent limit).

**Feeds:** `PLUGIN_DEPENDENCIES.md` and `MOODLE_CAPABILITIES_INDEX.md`.

---

### `classes.go` — PHP classes

**Source file:** `classes/**/*.php` of any plugin (also `MOODLE_CLASSES_INDEX.md`'s restricted `**/classes/**/*.php` glob across the whole installation) and `db/renamedclasses.php`.

**Parsing strategy:** line-by-line regex for `class`/`interface`/`trait`/`enum` declarations, with a small look-ahead to join a multi-line declaration up to the opening `{`. Skips symlinked files during the directory walk (a real file planted as a symlink pointing outside the scanned tree must not have its target's content parsed).

**Feeds:** `MOODLE_CLASSES_INDEX.md` and `PLUGIN_ARCHITECTURE.md`.

---

### `services.go` — Web services

**Source file:** `db/services.php` of any plugin.

**Parsing strategy:** array-literal parsing over the `$functions` array, keyed by function name, extracting `classname`, `methodname`, `description`, `type`, `ajax`, `capabilities`.

**Feeds:** `PLUGIN_ENDPOINT_INDEX.md` and `MOODLE_SERVICES_INDEX.md`.

---

### `tasks.go` — Scheduled tasks

**Source file:** `db/tasks.php` of any plugin.

**Parsing strategy:** array-literal parsing over the `$tasks` array, extracting `classname` and the cron-style schedule fields (minute/hour/day/month/dayofweek) plus the `blocking` flag.

**Feeds:** `PLUGIN_DEPENDENCIES.md` and `MOODLE_TASKS_INDEX.md`.

---

### `upgrade.go` — Upgrade history

**Source file:** `db/upgrade.php` of any plugin.

**Parsing strategy:** control-flow scan (not just array literals) for `if ($oldversion < NNNNNNNNNN)` blocks (exactly 10 digits), with a three-level description fallback: an inline comment on the same line, then the first `new xmldb_table('literal')` reference whose argument actually resolves to a string literal, then the first loose `//` comment anywhere in the block.

**Feeds:** `PLUGIN_DEPENDENCIES.md`.

---

### `moodledetect.go` — Moodle version

**Source file:** `version.php` in the Moodle root.

**Parsing strategy:** regex over `$version` (numeric, e.g. `2024042200`), `$release` (string, e.g. `4.4 (Build: 20240422)`), and `$branch`. Also used to detect whether a directory is a Moodle root at all (for `doctor` and the CLI installer).

**Feeds:** `~/.build82` configuration and the `AI_CONTEXT.md` header.

---

### `plugin.go` — Plugin metadata

**Source file:** plugin `version.php` + `lang/en/<component>.php`.

**Parsing strategy:** regex over `$plugin->component`, `$plugin->version`, `$plugin->requires`, `$plugin->maturity`. Uses `internal/moodletype`'s `PluginTypeToDir` map (37 entries) as the single source of truth to resolve a plugin's type from its directory path.

**Feeds:** `PLUGIN_CONTEXT.md` and `MOODLE_PLUGIN_INDEX.md`.

---

### `settings.go`, `subplugins.go` — extra plugin metadata

`settings.go` regex-scans `settings.php` for `new admin_setting_*(` declarations (a non-literal name is kept with an empty name). `subplugins.go` reads `db/subplugins.json` (both the `plugintypes` and the newer `subplugintypes` keys) and only falls back to `db/subplugins.php` when the JSON file does not exist. Neither has a tree-sitter counterpart.

**Feeds:** `PLUGIN_SETTINGS.md` (settings); `PLUGIN_DEPENDENCIES.md` and `PLUGIN_AI_CONTEXT.md` (subplugins; settings also in the AI context).

---

### `deprecated.go`, `capabilityusage.go`, `langstrings.go` — `doctor` diagnostics

Regex-only scans used exclusively by the `doctor` tool, not by any generator: bare calls to functions marked `@deprecated` in the core API index (method and static calls are ignored), capability checks against the plugin's own capability prefix (`mod/name`, `block/name` for those two types, the frankenstyle component otherwise), and `get_string()` calls versus the keys declared in `lang/en/<component>.php`.

---

### `shared.go` — safe file access

`readFileCapped` refuses to buffer files larger than 8 MiB and reads only regular files (`fsutil.ReadRegular`: a symbolic link at the file is not followed, and a FIFO or device is refused without blocking), and the shared directory walk skips symlinked files, so scanning untrusted third-party plugin code cannot exhaust memory, read files outside the plugin tree, or hang on a planted FIFO. The tree-sitter backend applies the same size cap and the same regular-file rule.

---

## Adding a new extractor

To add support for a new Moodle file type:

**1. Create the file in `internal/extractors/`:**

```go
// internal/extractors/myextractor.go
package extractors

type MyData struct {
	Field string
}

func ExtractMyData(filePath string) *MyData {
	// read the file, parse it, return structured data (nil if the file doesn't exist)
	return nil
}
```

**2. If it needs a tree-sitter counterpart**, add the mirror in `internal/extractors/tsbackend/myextractor.go` (its own struct, never importing `extractors`), and dispatch from the regex-side function via `useTreesitter()` — see `backend.go` for the existing pattern.

**3. Connect to the relevant generator in `internal/generators/`:** call the extractor inside `moodle.go` (global indexes) or `plugin.go` (plugin context).

**4. Write real tests:** a well-formed fixture, a missing-file case, and — if there are two backends — a parity test comparing both against the same fixture, plus (when a real Moodle installation is available in the environment) a parity sweep against every real matching file.

**5. Format, build, vet, and test the whole module:**

```bash
gofmt -w . && go build ./... && go vet ./... && go test -race ./...
```

**6. Document the new file in [Generated Files](../reference/generated-files.md).**

---

## See also

- [Generators](./generators.md) — how extractor data becomes `.md` files
- [Cache System](./cache-system.md) — when the extractor is called and when it is skipped
- [Generated Files](../reference/generated-files.md) — the `.md` files each extractor feeds

---

[🏠 Back to Index](../index.md)
