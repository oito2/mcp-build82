🌐 [Português](../../pt-br/architecture/generators.md) | **English** | 🏠 [Index](../index.md)

---

# Generators

The **Generators** receive the structured data produced by the [Extractors](./extractors.md) and transform it into Markdown context files written under `.build82/` in the Moodle installation. They are what makes the content available for the MCP server to serve via Resources and Tools.

---

## Overview

```
internal/generators/
├── moodle.go     ← 13 global generators (Moodle root)
├── plugin.go     ← 12 generators per plugin (plugin directory)
├── migration.go  ← legacy flat-file → .build82/ migration, run first on every pass
└── common.go     ← shared file-walking helpers (skip vendor/node_modules/.git), dev-plugin/plugin-dir discovery, nil-safe accessors, plugin-info cache
```

All generators follow the same contract:

- Receive a path (`moodlePath` or `extractors.PluginInfo`) as input
- Call the extractor(s) they need
- Build the Markdown content in memory (via `strings.Builder`)
- Check the [mtime cache](./cache-system.md) before writing — if the `.md` file already exists and is newer than the tracked PHP/XML sources, the write is skipped (`Skipped: true`)
- Write the file via `genutil.Write` (creates the directory, plain `os.WriteFile`), which also marks the cache fresh on success
- Return a `GeneratorResult{ File, Success, Skipped, Error }`
- Each generator body is wrapped in an **error boundary** via `genutil.Safely` — a panic or error in one generator never interrupts the others

---

## Global generators (`moodle.go`)

They generate files under `{moodle_root}/.build82/`. Called by `init_moodle_context` and `update_indexes`.

The entry point is `GenerateAll(moodlePath, moodleVersion)`, which migrates any legacy flat files first, then runs the first 12 generators below concurrently (each is independent — different output files, no shared mutable state beyond the mtime cache, which is itself mutex-guarded). `GenerateAiIndex` runs afterwards, because it lists the files the first wave just wrote, and `GenerateCtags` runs last. Each generator is wrapped by `genutil.RunCached`, which does the `IsStale` check and records a `Skipped: true` result without calling the generator when nothing changed.

| Function | Generated file | Extractors used |
| --- | --- | --- |
| `GenerateAiContext` | `AI_CONTEXT.md` | — (static content + the plugin directory count; version passed in by the caller) |
| `GenerateApiIndex` | `MOODLE_API_INDEX.md` | `api` |
| `GenerateEventsIndex` | `MOODLE_EVENTS_INDEX.md` | `events` |
| `GenerateTasksIndex` | `MOODLE_TASKS_INDEX.md` | `tasks` |
| `GenerateServicesIndex` | `MOODLE_SERVICES_INDEX.md` | `services` |
| `GenerateDbTablesIndex` | `MOODLE_DB_TABLES_INDEX.md` | `schema` |
| `GenerateClassesIndex` | `MOODLE_CLASSES_INDEX.md` | `classes` (restricted `**/classes/**/*.php` glob, resolved before parsing — not a full-installation scan) |
| `GenerateCapabilitiesIndex` | `MOODLE_CAPABILITIES_INDEX.md` | `capabilities` |
| `GeneratePluginIndex` | `MOODLE_PLUGIN_INDEX.md` | `plugin` |
| `GenerateDevRules` | `MOODLE_DEV_RULES.md` | — (static content) |
| `GeneratePluginGuide` | `MOODLE_PLUGIN_GUIDE.md` | — (static content + version) |
| `GenerateAiWorkspace` | `MOODLE_AI_WORKSPACE.md` | `plugin` (via the plugin dirs list) |
| `GenerateAiIndex` | `MOODLE_AI_INDEX.md` | — (lists existing files) |
| `GenerateCtags` | `.build82/tags` | — (shells out to `ctags`) |

---

## Plugin generators (`plugin.go`)

They generate files under `{plugin_root}/.build82/`. Called by `generate_plugin_context` and `plugin_batch`.

The entry point is `GenerateAllForPlugin(pluginPath, moodlePath, markAsDev, existingInfo)`, which loads the mtime cache, delegates to `GenerateAllForPluginCore`, then persists the cache. Batch orchestrators (`plugin_batch`, `update_indexes`'s `include_plugins` path) call `GenerateAllForPluginCore` directly instead, wrapping their *entire* loop in a single cache load/save pair — calling `Save()` once per plugin in a large batch would otherwise serialize the I/O and defeat the point of `plugin_batch`'s worker pool.

All 10 plugin extractors (Classes, Hooks, Schema, Events, Tasks, Services, Capabilities, Upgrade, Subplugins, Settings) run concurrently up front into a shared `PreloadedPluginData` struct, so the generators below read already-extracted data instead of re-parsing the plugin's files. When `markAsDev` is true, the run ends by writing the `.build82/.indevelopment` marker (a failure is logged to stderr, not returned).

| Function | Generated file | Fields used from `PreloadedPluginData` |
| --- | --- | --- |
| `GeneratePluginContext` | `PLUGIN_CONTEXT.md` | `Schema`, `Events`, `Tasks`, `Services`, `Capabilities` |
| `GeneratePluginStructure` | `PLUGIN_STRUCTURE.md` | — (directory reading only) |
| `GeneratePluginDbTables` | `PLUGIN_DB_TABLES.md` | `Schema` |
| `GeneratePluginEvents` | `PLUGIN_EVENTS.md` | `Events` |
| `GeneratePluginDependencies` | `PLUGIN_DEPENDENCIES.md` | `Tasks`, `Services`, `Capabilities`, `Hooks`, `Upgrade`, `Subplugins` |
| `GeneratePluginFunctionIndex` | `PLUGIN_FUNCTION_INDEX.md` | — (glob + file read, `api` extractor per file) |
| `GeneratePluginCallbackIndex` | `PLUGIN_CALLBACK_INDEX.md` | `Hooks` (+ a single read of `lib.php`/`locallib.php`, shared across the legacy-suffix checks from `legacyhooks.Map`) |
| `GeneratePluginEndpointIndex` | `PLUGIN_ENDPOINT_INDEX.md` | `Services` |
| `GeneratePluginRuntimeFlow` | `PLUGIN_RUNTIME_FLOW.md` | `Classes`, `Events`, `Tasks`, `Services` |
| `GeneratePluginArchitecture` | `PLUGIN_ARCHITECTURE.md` | `Classes` |
| `GeneratePluginSettings` | `PLUGIN_SETTINGS.md` | `Settings` |
| `GeneratePluginAiContext` | `PLUGIN_AI_CONTEXT.md` | every field of `PreloadedPluginData` except `Upgrade` |

---

## Design details

### Error boundary — `genutil.Safely`

Every generator body is wrapped by `genutil.Safely`, which recovers from a panic and converts it (or a returned error) into a failed `GeneratorResult` instead of crashing the whole batch:

```go
func Safely(outputFile string, fn func() (GeneratorResult, error)) GeneratorResult {
	result, err := func() (r GeneratorResult, e error) {
		defer func() {
			if p := recover(); p != nil {
				e = fmt.Errorf("%v", p)
			}
		}()
		return fn()
	}()
	if err != nil {
		return GeneratorResult{File: outputFile, Success: false, Error: err.Error()}
	}
	return result
}
```

If, say, `db/install.xml` is malformed enough to trip something unexpected, `GeneratePluginDbTables` fails on its own and the other 11 plugin generators keep running normally.

---

### Standard header — `genutil.Header`

Every generated `.md` file starts with a standardized header:

```
# File title

> Content description

_Generated by build82 on 2024-04-22 10:30:00_

---
```

---

### Migration — legacy flat files → `.build82/`

Before any generator runs, `MigrateLegacyGlobalFiles`/`MigrateLegacyPluginFiles` (in `migration.go`) move any of the 13 global filenames (plus `tags`) / 12 plugin filenames (plus `.indevelopment`) it finds sitting directly at the Moodle/plugin root into `.build82/` (the legacy flat layout). Both delegate to `MigrateLegacyFiles`, which rejects absolute or traversal filenames, refuses to move symlinks, and removes the root copy when a `.build82/` copy already exists. This runs unconditionally and idempotently on every pass (`doctor` uses the read-only `DetectLegacyFiles` instead); a file that fails to migrate (permission denied, a symlink, disk full) is logged to stderr rather than silently left in the wrong place, but never blocks the rest of the run.

---

### Filtering generated files from `PLUGIN_STRUCTURE.md`

`GeneratePluginStructure`'s directory tree needs to exclude build82's own output. Since everything now lives under one `.build82/` directory, this is a single name check (`ContextDir`), not a per-filename set — a structural simplification the flat legacy layout didn't allow.

---

### `PLUGIN_AI_CONTEXT.md` — the consolidator

`GeneratePluginAiContext` consolidates every field of `PreloadedPluginData` into a single file optimized to be the AI's entry point. It runs alongside the other 11 plugin generators — all of them share the same pre-extracted data, so there's no cost to also building this summary.

---

### Global and plugin generators: both run concurrently

Global generators in `GenerateAll` run concurrently — they're independent of each other and write to different files, so there's no race condition risk (the mtime cache itself is mutex-guarded).

Plugin generators in `GenerateAllForPluginCore` also run concurrently — since all their extractor data is pre-loaded up front into `PreloadedPluginData`, no generator depends on another's output.

---

### `GenerateCtags` — optional ctags generation

Shells out to `ctags` (`ctags -R --languages=PHP`, excluding `vendor` and `node_modules`) only if a `ctags` binary is found on `PATH`, and only if the tracked source files are newer than the existing `.build82/tags` file (same mtime-cache check as every other global generator). If `ctags` isn't installed, it returns a successful, skipped result (`Skipped: true`) and no `tags` file is created — `doctor` reports this as an optional, not required, tool.

---

## Adding a new generator

**1. Decide where the generated file should live:**

- Moodle root → add to `moodle.go` and register in `GenerateAll`
- Plugin directory → add to `plugin.go` and register in `GenerateAllForPluginCore`

**2. Implement the function following the contract:**

```go
func GenerateMyFile(moodlePath string) GeneratorResult {
	output := GlobalOutputPath(moodlePath, "MY_FILE.md")
	return genutil.Safely(output, func() (GeneratorResult, error) {
		data := extractors.ExtractMyData(filepath.Join(moodlePath, "lib", "myfile.php"))

		var b strings.Builder
		b.WriteString(genutil.Header("My File", "Content description"))
		// ... build content from data ...

		return genutil.Write(output, b.String()), nil
	})
}
```

The mtime-cache check (`cache.Global.IsStale`) happens in the caller (`genutil.RunCached`, used by `GenerateAll`/`GenerateAllForPluginCore`), not inside the generator itself.

**3. Register it in `GenerateAll` or `GenerateAllForPluginCore`**, providing the list of source files that should gate its staleness.

**4. Add the filename to `GlobalContextFilenames`/`PluginContextFiles` in `migration.go`** if it's a new file, so migration and exclusion logic pick it up automatically.

**5. Document the new file in [Generated Files](../reference/generated-files.md).**

---

## See also

- [Extractors](./extractors.md) — the packages that feed the generators
- [Cache System](./cache-system.md) — when a generator skips writing
- [Generated Files](../reference/generated-files.md) — complete list of produced `.md` files

---

[🏠 Back to Index](../index.md)
