🌐 [Português](../../pt-br/reference/tools.md) | **English** | 🏠 [Index](../index.md)

---

# Tools Reference

All 15 MCP tools exposed by build82, read from the registrations and input structs in `internal/tools/`.

## Conventions

| Topic | Behavior |
|-------|----------|
| `format` parameter | Optional `"text"` (default) or `"json"`; an empty string means the default. The input schema declares the values as an `enum`, so any other value is rejected before the tool runs. Accepted by 11 tools: `init_moodle_context`, `generate_plugin_context`, `plugin_batch`, `update_indexes`, `search_plugins`, `search_api`, `get_plugin_info`, `list_dev_plugins`, `doctor`, `bump_plugin_version`, `check_coding_style`. It selects the text block only: Markdown for `text`, the structured output as one indented JSON block for `json`. |
| Structured output | Those 11 tools declare an `outputSchema`, and every successful result carries the same document as `structuredContent`, **whatever the `format`** (each tool's "JSON" below describes it). Clients that read `structuredContent` — Claude Code shows it to the agent instead of the text block — therefore receive JSON in either format. |
| No `format` parameter | `watch_plugins`, `explain_plugin`, `release_plugin`, `create_plugin_skeleton` — output is always plain text/Markdown, with no `outputSchema` and no `structuredContent`. |
| Errors | A failed call (invalid argument, missing configuration, unknown plugin, missing index) is an error result (`isError: true`) whose only content is the message text, in either format, and carries no `structuredContent`. The only exception is `doctor`, whose failed diagnosis is an error result that still carries the full structured report. |
| `doctor` | Its structured report is described under [`doctor`](#doctor). |
| Initialization guard | Every tool except `init_moodle_context`, `doctor` and `watch_plugins` (`stop`/`status`) needs a resolvable configuration (`BUILD82_MOODLE_PATH` or `~/.build82`). Otherwise it returns an error result: ``❌ build82 is not initialized. Run `init_moodle_context` first.`` |
| Configuration failure | If the configuration location cannot be resolved (e.g. no home directory): error result `❌ Failed to resolve build82 configuration: <cause>`. |
| Panics | A panic inside any tool is recovered and returned as an error result: `❌ Internal error while handling this request: <cause>`. |
| Plugin files | Files read from plugin directories (and the generated files under `.build82/`) are opened only when they are regular files: a symbolic link at the file is not followed and a FIFO or device is never waited on, so a planted special file cannot hang the server. Generated files are written to a temp file and renamed into place, so a symbolic link planted at an output is replaced, not followed, and readers never see a partial file; a `.build82/` directory that is a symbolic link is refused (the outputs fail with `.build82 is a symbolic link: symbolic links are not followed for writes`). |
| Plugin identifiers | Tools that take a plugin accept different forms; see each tool. `get_plugin_info`, `plugin_batch mode="list"`, `generate_plugin_context`, `explain_plugin`, `release_plugin`, `bump_plugin_version` and `check_coding_style` all accept a component (`local_myplugin`), a Moodle-relative path (`local/myplugin`) or an absolute path. |
| Path containment | Every plugin path must resolve inside the configured Moodle root, with symbolic links resolved — also for a path that does not exist yet, through its deepest existing directory. The Moodle root itself is never a plugin, and a component identifier must be `type_name` with lowercase letters, digits and underscores (so `mod_..` is not a path). A `$plugin->component` in `version.php` that is not such a name is ignored and the plugin is named `type_name` after its directory. |
| Reported paths | Generation and plugin reports do not include absolute paths (`doctor` and `init_moodle_context` do report the configured Moodle path, config file and cache file). Generated-file lists are relative (to the Moodle root for global files, to the plugin for plugin files); plugin locations (`Path`, `Source`, the skeleton location) are relative to the Moodle root. |


### Annotations

Every tool declares all four MCP hints explicitly, plus a title, so clients never fall back to the protocol defaults (which assume a destructive, open-world tool). No tool reaches outside the Moodle installation (`openWorldHint: false` everywhere).

| Tool | Title | `readOnlyHint` | `destructiveHint` | `idempotentHint` |
|------|-------|:---:|:---:|:---:|
| `init_moodle_context` | Initialize Moodle Context | false | false | true |
| `generate_plugin_context` | Generate Plugin Context | false | false | true |
| `plugin_batch` | Generate Context for Many Plugins | false | false | true |
| `update_indexes` | Update Indexes | false | false | true |
| `watch_plugins` | Watch Dev Plugins | false | false | true |
| `search_plugins` | Search Plugins | true | false | true |
| `search_api` | Search Moodle API | true | false | true |
| `get_plugin_info` | Get Plugin Info | true | false | true |
| `list_dev_plugins` | List Dev Plugins | true | false | true |
| `doctor` | Doctor | true | false | true |
| `explain_plugin` | Explain Plugin | true | false | true |
| `release_plugin` | Package Plugin Release | false | true (replaces an existing ZIP of the same name) | true |
| `create_plugin_skeleton` | Create Plugin Skeleton | false | false | false (a second call fails: the plugin exists) |
| `bump_plugin_version` | Bump Plugin Version | false | true (edits existing `version.php` and `db/upgrade.php`) | false (each call raises the version again) |
| `check_coding_style` | Check Coding Style | true | false | true |

The exact input and output schemas of every tool are kept as golden files in `internal/server/testdata/tools/`.

---

## `init_moodle_context`

Initializes context for a Moodle installation: validates the path, detects the version, saves the configuration and generates all global index files.

| Parameter | Type | Required | Default | Description |
|-----------|------|:---:|---------|-------------|
| `moodle_path` | string | ✅ | — | Path to the Moodle root. A relative path is resolved against the server's working directory and stored as an absolute path |
| `force` | boolean | ❌ | `false` | Re-initialize even if a configuration is already resolvable |
| `format` | `text`/`json` | ❌ | `text` | Output format |

**Validation** (all must hold, otherwise error `❌ Invalid Moodle path: <reason>`): the directory exists; `version.php` exists; `lib/` exists; `config.php` or `config-dist.php` exists.

**Side effects:** writes `~/.build82` (see [Configuration](./configuration.md)); runs the 13 global generators plus the ctags step, writing under `{moodle_root}/.build82/` and updating `{moodle_root}/.build82/.cache.json`; migrates legacy flat files into `.build82/`. See [Generated Files](./generated-files.md).

**Returns:** text report with Moodle path, version, full version, config file path and Generated/Cached/Failed file lists. JSON: `success`, `already_initialized`, `moodle_path`, `moodle_version`, `moodle_full_version`, `config_path`, `generated[]`, `skipped[]`, `failed[]` (`{file, error}`).

**Environment override:** when `force` is `true` and `BUILD82_MOODLE_PATH` is set in the server's environment, the file is still written, but the report warns that the variable keeps taking precedence over it.

**Already initialized:** if a configuration is already resolvable (environment variable or file) and `force` is not `true`, the tool returns a non-error message with the stored path and version and does nothing else.

> When `BUILD82_MOODLE_PATH` is set, the configuration always resolves from the environment, so this tool reports "already initialized" without generating anything unless `force: true`. Run `update_indexes` to generate the global files in that setup. With `force: true` the tool also writes `~/.build82`, but the environment variable keeps precedence when the configuration is loaded.

---

## `generate_plugin_context`

Generates the 12 `PLUGIN_*.md` files for one plugin and marks it as under development.

| Parameter | Type | Required | Default | Description |
|-----------|------|:---:|---------|-------------|
| `plugin_path` | string | ✅ | — | Component (`local_myplugin`), path relative to the Moodle root (`local/myplugin`) or absolute path |
| `format` | `text`/`json` | ❌ | `text` | Output format |

**Validation:** the path is inside the Moodle root, exists, and contains `version.php`. Errors: `❌ Invalid plugin path: must be within the Moodle installation.`, `❌ Plugin directory not found: <rel>`, `❌ <rel> does not appear to be a Moodle plugin.`, `❌ Failed to detect plugin: <cause>`.

**Side effects:** writes the 12 files under `{plugin}/.build82/` (subject to the mtime cache), always writes `{plugin}/.build82/.indevelopment`, updates `{moodle_root}/.build82/MOODLE_AI_INDEX.md` and `.cache.json`, migrates legacy flat files.

**Returns:** text report (component, type, version, path relative to the Moodle root, Generated/Cached/Failed lists, pointer to `.build82/PLUGIN_AI_CONTEXT.md`). JSON: `component`, `type`, `version`, `path` (relative to the Moodle root), `generated[]`, `skipped[]`, `failed[]`.

> This tool has no `force` parameter — it always respects the mtime cache. To force regeneration, use `plugin_batch` with `mode="list"` and `force: true`.

---

## `plugin_batch`

Generates or refreshes context for multiple plugins.

| Parameter | Type | Required | Default | Description |
|-----------|------|:---:|---------|-------------|
| `mode` | `dev`/`all`/`list` | ❌ | `dev` | `dev`: every `.indevelopment` plugin. `all`: every plugin found under the known plugin-type directories. `list`: the plugins in `plugins` |
| `plugins` | string[] | ❌ | — | Required and non-empty when `mode=list`; max 500 entries. Each entry: component, relative path or absolute path; entries naming the same plugin are processed once. Ignored in other modes |
| `force` | boolean | ❌ | `false` | Regenerates the 12 plugin files of every plugin in the batch regardless of file modification times: each output is marked stale in the cache before generating |
| `mark_as_dev` | boolean | ❌ | `false` | For `mode=all`/`list`, also writes `.build82/.indevelopment`. `mode=dev` always marks |
| `parallel` | number | ❌ | `0` | Worker-pool size. `0` (or negative) = sequential. Capped at 16 and at the number of plugins |
| `format` | `text`/`json` | ❌ | `text` | Output format |

**Errors:** `❌ No plugins found in this Moodle installation.` (`all`); `❌ mode='list' requires a non-empty 'plugins' array.`; `❌ too many plugins in 'plugins' (max 500).`; `❌ Could not resolve the following plugin identifiers: <list>` (unresolvable or outside the Moodle root; nothing is generated). `mode=dev` with no marked plugins returns a non-error info message. A failure in one plugin does not abort the batch.

**Side effects:** same per-plugin writes as `generate_plugin_context`; updates `MOODLE_AI_INDEX.md` and `.cache.json`.

**Returns:** text report grouped Regenerated / Cached / Failed, the cache hit/miss/skip counters of this call (`Cache (this call): …`), and a marker tip or count. JSON: `{mode, plugins[]}`, each plugin `{component, path, generated, skipped, failed, error?}` (`path` relative to the Moodle root; counts are file counts). In `dev` mode with no marked plugin, the call succeeds with an empty `plugins` list and a tip as text.

---

## `update_indexes`

Regenerates the 13 global indexes (and `tags`), re-detecting the Moodle version.

| Parameter | Type | Required | Default | Description |
|-----------|------|:---:|---------|-------------|
| `force` | boolean | ❌ | `false` | Regenerates every global output (the 13 Markdown files and `tags`) regardless of file modification times; with `include_plugins`, also the 12 files of each dev plugin |
| `include_plugins` | boolean | ❌ | `false` | Also regenerate every `.indevelopment` plugin (and re-write its marker) |
| `format` | `text`/`json` | ❌ | `text` | Output format |

**Side effects:** rewrites `~/.build82` when the detected version differs from the stored one and the configuration was loaded from that file (with `BUILD82_MOODLE_PATH` set, the file is left alone); writes the global files and `.cache.json`; with `include_plugins`, the per-plugin files.

**Returns:** text report with Moodle version, counts of Regenerated/Cached/Failed global files, per-plugin lines (when requested) and the cache counters of this call (`Cache (this call): …`). JSON: `moodle_version`, `regenerated[]`, `skipped[]`, `failed[]` (`{file, error}`), `plugins[]` (one summary line per dev plugin).

---

## `watch_plugins`

Starts, stops, or reports the status of the file watcher that regenerates dev-plugin context on change. No `format` parameter.

| Parameter | Type | Required | Default | Description |
|-----------|------|:---:|---------|-------------|
| `action` | `start`/`stop`/`status` | ❌ | `start` | Action to perform |

**Behavior of `start`:**

| Aspect | Detail |
|--------|--------|
| Scope | Plugins marked `.indevelopment`, sorted by path; only the first 20 are watched (extra ones are logged to stderr) |
| Watched files | Any of `version.php`, `lib.php`, `locallib.php`, `settings.php`, `db/install.xml`, `db/access.php`, `db/events.php`, `db/tasks.php`, `db/services.php`, `db/upgrade.php`, `db/hooks.php`, `db/subplugins.json`, `db/subplugins.php` that exists when the watcher starts. Saving, creating, renaming or removing one of them triggers a regeneration |
| Debounce | 500 ms per plugin |
| On change | Marks the plugin's 12 outputs stale in the cache (so the regeneration is forced, regardless of modification times), regenerates its 12 files (re-marking it), refreshes `MOODLE_AI_INDEX.md`, then sends `notifications/resources/updated` for the plugin's resources, `moodle://index` and `moodle://plugins/with-context` to the sessions subscribed to them (see [Subscriptions](./resources.md#-subscriptions)). It also still sends an MCP log notification (level `info`, logger `build82/watcher`) to every connected session; MCP logging is deprecated, so this log message is transitional and may be removed in a later release |
| No watchable files | If no `.indevelopment` plugin has any watchable file, the watcher is not started: the call returns a non-error `ℹ️ Watcher not started — no watchable files found.` with a hint to mark a plugin first, and no watcher is left active (`status` reports not running, `stop` reports nothing to stop) |
| Persistence | In memory only; does not survive a server restart |

**Returns (text):** `start` — `✅ Watcher started — monitoring <n> files across dev plugins.`, or `ℹ️ Watcher not started — no watchable files found.` (non-error) when there is nothing to watch; already running — `⚠ Watcher is already running. Use action: 'stop' first.`; `stop` — `✔ Watcher stopped.` or `No active watcher to stop.`; `status` — `✔ Watcher is active.` or `Watcher is not running.`. `start` needs initialization (error result otherwise).

---

## `search_plugins`

Searches the plugin index (`MOODLE_PLUGIN_INDEX.md`) for a query string.

| Parameter | Type | Required | Default | Description |
|-----------|------|:---:|---------|-------------|
| `query` | string | ✅ | — | Search term; max 200 characters |
| `limit` | number | ❌ | `20` | Max results. Values `<= 0` use the default; values above `100` are clamped to `100` |
| `format` | `text`/`json` | ❌ | `text` | Output format |

**Matching:** case-insensitive substring match against the index table rows. If nothing matches, a fuzzy fallback compares each word of each row with the query by edit distance (allowed distance: 0 for queries up to 3 characters, 1 up to 6, 2 above).

**Errors:** `❌ query too long (max 200 characters).`; ``❌ Plugin index not found. Run `init_moodle_context` or `update_indexes` first.``

**Returns:** text table `Component | Type | Name | Version | Path` (with a note when fuzzy), or `No plugins matched "<query>".` (not an error). JSON: `{query, fuzzy, matches[]}` where `matches` are raw Markdown table rows.

---

## `search_api`

Searches the Moodle API index (`MOODLE_API_INDEX.md`, function lines) for a query string.

| Parameter | Type | Required | Default | Description |
|-----------|------|:---:|---------|-------------|
| `query` | string | ✅ | — | Function name or keyword in the summary; max 200 characters |
| `visibility` | `public`/`deprecated`/`all` | ❌ | `public` | `public` excludes lines marked `@deprecated`; `deprecated` keeps only those; `all` keeps both. Any other value is rejected by the input schema |
| `limit` | number | ❌ | `30` | Max results. Values `<= 0` use the default; values above `100` are clamped to `100` |
| `format` | `text`/`json` | ❌ | `text` | Output format |

**Matching:** same exact-then-fuzzy strategy as `search_plugins`, over lines starting with `` - ` ``. Visibility and `limit` are applied after matching.

**Errors:** `❌ query too long (max 200 characters).`; ``❌ API index not found. Run `init_moodle_context` or `update_indexes` first.``

**Returns:** text list of matching index lines, or `No functions matched "<query>".` (not an error). JSON: `{query, visibility, fuzzy, matches[]}`.

---

## `get_plugin_info`

Returns a plugin's generated context — or live-detected metadata if not yet generated.

| Parameter | Type | Required | Default | Description |
|-----------|------|:---:|---------|-------------|
| `plugin` | string | ✅ | — | Component (`local_myplugin`), relative path or absolute path |
| `format` | `text`/`json` | ❌ | `text` | Output format |

**Behavior, in order:**

1. If the plugin cannot be found inside the Moodle root (including the root itself and directories without `version.php`), the plugin index is searched for the value; matching rows are returned as "possible matches" in an error result. No match: error `❌ Plugin not found: <plugin>`.
2. If `{plugin}/.build82/PLUGIN_AI_CONTEXT.md` exists, its full content is the text block.
3. Otherwise the plugin is detected live and a Field/Value table is returned (Type, Version, Requires, Display name, Path) with a note that `generate_plugin_context` has not been run. Error `❌ Failed to detect plugin: <cause>` if detection fails.

JSON (both cases): `{path, component, type, name, version, requires, display_name, maturity, has_ai_context, ai_context?}` — the detected metadata, plus the full `PLUGIN_AI_CONTEXT.md` in `ai_context` when it exists.

Every path in the response is relative to the Moodle root. Read-only.

---

## `list_dev_plugins`

Lists every plugin marked `.indevelopment`.

| Parameter | Type | Required | Default | Description |
|-----------|------|:---:|---------|-------------|
| `format` | `text`/`json` | ❌ | `text` | Output format |

**Returns:** table `Component | Path | Has AI Context` (`✔` when `.build82/PLUGIN_AI_CONTEXT.md` exists), sorted by path. JSON: `{plugins[]}`, each `{component, path, has_ai_context}`. With no marked plugin, the call succeeds with an empty `plugins` list and the text `No .indevelopment plugins found.`. Read-only.

**Example:**
```
Which plugins are marked as under development?
```

---

## 🏷️ Marking Plugins as Under Development

A plugin is "under development" when it has a `.indevelopment` file **inside its `.build82/` directory** (`{plugin}/.build82/.indevelopment`). The marker is used by `list_dev_plugins`, `watch_plugins`, `plugin_batch mode="dev"`, `update_indexes include_plugins`, and `doctor`.

### How to mark a plugin

**Option 1 — Via assistant (when generating context):**

`generate_plugin_context` always marks the plugin as a side effect — just ask:

```
Generate the context for local_myplugin.
```

**Option 2 — Multiple plugins at once:**

```
Generate context for local_reports and local_audit
and mark both as under development.
```

The assistant will call `plugin_batch` with `mode="list"` and `mark_as_dev=true`.

**Option 3 — Manual:**

```bash
mkdir -p /your/moodle/local/myplugin/.build82
touch /your/moodle/local/myplugin/.build82/.indevelopment
```

> A `.indevelopment` file placed at the plugin root (the pre-`.build82/` layout) is not recognized until the next generation run on that plugin moves it into `.build82/`.

### How to unmark a plugin

```bash
rm /your/moodle/local/myplugin/.build82/.indevelopment
```

### Check which plugins are marked

```
Which plugins are marked as under development?
```

The assistant will call `list_dev_plugins`.

---

## `doctor`

Read-only environment diagnostic. Returns a Markdown report by default, or the structured report as JSON text with `format: "json"`; the structured report is the `structuredContent` in both formats, also when the verdict is `fail` (an error result).

| Parameter | Type | Required | Default | Description |
|-----------|------|:---:|---------|-------------|
| `format` | `text`/`json` | ❌ | `text` | Output format |

**Report sections, in order:**

| Section | Checks |
|---------|--------|
| System Dependencies | `php`, `ctags`, `git`, `phpcs` on `PATH` (missing = warning, "optional"; a `ctags` that is not Universal Ctags = warning, the tags file is skipped; a `phpcs` whose `phpcs -i` fails or does not list the `moodle` standard = warning, `check_coding_style` is unavailable) |
| Configuration | Config file path, Moodle path, Moodle version. If not initialized: `Config — not initialized` and the report stops (non-error result). If the config location cannot be resolved: error result |
| Moodle Installation | Directory exists and looks like a Moodle root |
| Global Index Files | Each of the 13 global files: missing = failure; older than 7 days = warning (`stale`); otherwise age in days |
| Development Plugins | For each `.indevelopment` plugin, the same freshness check on its 12 files |
| Legacy Files Pending Migration | Count of flat legacy files awaiting move into `.build82/`: global ones (moved by the next `init_moodle_context` or `update_indexes`) and dev-plugin ones (moved by the plugin's next `generate_plugin_context` or `plugin_batch` run, or a watcher regeneration) |
| Cross-Plugin Consistency | Capability names declared by more than one dev plugin (needs at least 2 dev plugins) |
| Deprecated Core API Usage | Bare calls in dev-plugin PHP to functions marked `@deprecated` in the global API index; a fully qualified global call (`\name(`) counts too. Not flagged: method and static calls, function declarations (`function name(`), class instantiations (`new name(`), variable calls (`$name(`) and namespaced names (`ns\name(`). Skipped without dev plugins |
| Capability Usage | `has_capability()`/`require_capability()` calls naming one of the plugin's own capabilities (`type/name:`, the form Moodle requires for every plugin type, e.g. `local/myplugin:view`) not declared in `db/access.php` |
| Lang String Usage | `get_string()` calls naming the plugin's own component (for an activity module, `mod_forum` or `forum`) with a string not declared in its English language file (`lang/en/{component}.php`; `lang/en/{name}.php` for an activity module) |
| Upgrade Consistency | Each dev plugin's `db/upgrade.php` against `db/install.xml` and `version.php`. **fail:** a step without a savepoint, or whose last savepoint is for another version; a savepoint function or arguments wrong for the plugin type (`upgrade_mod_savepoint(true, V, name)` for `mod`, `upgrade_block_savepoint(true, V, name)` for `block`, `upgrade_plugin_savepoint(true, V, type, name)` otherwise); step versions that do not strictly increase; a step newer than `$plugin->version`; a table or field the steps leave in place that `db/install.xml` lacks (a fresh install would miss it). **warn:** a table or field the steps remove that `db/install.xml` still defines. Schema operations are replayed in order and only each name's final state counts; names a later step handles only through a helper function, or fields named through a `foreach` variable, are skipped. A table added to `db/install.xml` without an upgrade step is not detected (that needs the file history) |
| Privacy API | Each dev plugin's `classes/privacy/provider.php` (comments ignored), all **warn**: the file is missing; it implements neither `\core_privacy\local\metadata\null_provider` nor `\core_privacy\local\metadata\provider`; it is a `null_provider` while a `db/install.xml` table has a `userid`, `usermodified` or `relateduserid` field or a foreign key to `user` |
| Cache | Hits/misses/skips counters and `.cache.json` size |

**Verdict line:** `❌ Issues found` (any failure), `⚠️ Warnings found` (any warning), otherwise `✅ All checks passed.`

**Structured output (JSON):** one object with an array per report section, plus the overall verdict. Every check is `{label, status, detail}` where `status` is `ok`, `warn` or `fail` (`detail` is omitted when empty).

| Key | Content |
|-----|---------|
| `system_dependencies` | Checks for `php`, `ctags`, `git` |
| `configuration` | Config file, Moodle path and Moodle version checks |
| `moodle_installation` | Installation directory check |
| `global_index_files` | One check per global file |
| `development_plugins` | Array of `{component, checks[]}`, one per dev plugin |
| `legacy_files` | The legacy-files check |
| `cross_plugin_consistency` | Duplicate-capability checks |
| `deprecated_api_usage` | Deprecated core API checks |
| `capability_usage` | Capability-usage checks |
| `lang_string_usage` | Lang-string-usage checks |
| `upgrade_consistency` | Upgrade-consistency checks |
| `privacy_api` | Privacy API checks |
| `cache` | `{hits, misses, skips, file?, file_bytes?}`; `file`/`file_bytes` only when `.cache.json` exists. Omitted on early exits |
| `verdict` | `ok`, `warn` or `fail` (same rule as the text verdict line, computed from the check sections; see the early exits below) |
| `hint` | Only when not initialized: ``Run `init_moodle_context` to initialize.`` |

Every section key is always present as an array (empty when the section did not run or has nothing to report), never `null`. Early exits keep the same shape: if the configuration location cannot be resolved (error result) or the config file path cannot be resolved (error result), `configuration` holds the single `fail` check and `verdict` is `fail`; if build82 is not initialized (non-error result), `configuration` holds `Config` = `fail` ("not initialized"), `verdict` is `fail` and `hint` is set. All other sections are empty in those cases.

---

## `explain_plugin`

Compact, section-selectable explanation of a plugin. No `format` parameter (Markdown only).

| Parameter | Type | Required | Default | Description |
|-----------|------|:---:|---------|-------------|
| `plugin` | string | ✅ | — | Component (`local_myplugin`), path relative to the Moodle root or absolute path |
| `section` | `all`/`overview`/`database`/`events`/`classes`/`services`/`flow` | ❌ | `all` | Section to return |

**Behavior:** with `section=all` and an existing `.build82/PLUGIN_AI_CONTEXT.md`, returns its first 1 MiB. Otherwise extracts live from the PHP/XML sources: `overview` (metadata table with the plugin path relative to the Moodle root + key-file checklist), `database` (tables with field/key counts), `classes` (FQN, kind, extends), `events` (observer → callback), `services` (web-service functions and capabilities), `flow` (entry-point checklist and scheduled tasks); `all` returns every section. Every live section except `overview` is preceded by the overview.

**Errors:** `❌ Unknown section "<x>". Valid values: all, overview, database, events, classes, services, flow.`; the path errors listed for `generate_plugin_context`. Read-only.

---

## `release_plugin`

Packages a plugin directory into a distributable ZIP, excluding build82's own generated files and other non-shippable artifacts. No `format` parameter (plain text).

| Parameter | Type | Required | Default | Description |
|-----------|------|:---:|---------|-------------|
| `component` | string | ✅ | — | Plugin identifier: component (`local_myplugin`), path relative to the Moodle root (`local/myplugin`) or absolute path |
| `output_dir` | string | ❌ | current working directory | Directory to write the ZIP into. Must already exist |
| `strict` | boolean | ❌ | `false` | Validate moodle.org plugin directory submission requirements before packaging |

**Errors:** `❌ component is required: ...` (empty or blank value); `❌ Output directory does not exist: <dir>`; plugin path errors (as `generate_plugin_context`); `❌ Could not read version from <plugin>/version.php`; `❌ Failed to create ZIP: <cause>`; in strict mode the list of failed checks.

**`strict:true` checks** (if any fails, the ZIP is **not** created and every failure is listed):

| Check | Requirement |
|-------|-------------|
| Component | The requested value equals `$plugin->component` in `version.php`. For a path identifier (relative or absolute) the expected value is `{type}_{dir}` (plugin type plus its directory name) |
| `$plugin->requires` | Present |
| `$plugin->maturity` | Present and one of `MATURITY_ALPHA`, `MATURITY_BETA`, `MATURITY_RC`, `MATURITY_STABLE` |
| Language file | `lang/en/{component}.php` exists (`lang/en/{name}.php` for an activity module) |
| Privacy API | `classes/privacy/provider.php` exists |
| Third-party libraries | `thirdpartylibs.xml` exists when a `thirdparty/` directory is present |
| Git | No `.git` directory in the plugin |

**Output:** ZIP `{output_dir}/{component}_{version}.zip`, using the component and `$plugin->version` read from `version.php`. An existing file with that name is replaced. The ZIP is written atomically (to a temporary file, then renamed), so a failure never leaves a partial ZIP and keeps a previously existing one intact. The archive has a single root folder named after the plugin directory (e.g. `caedauth/`); empty directories are not included. Each entry is compressed (Deflate) and keeps its file's modification time and permission bits (on Windows, only the read-only bit).

**Excluded from the ZIP** (matched by base name at any depth; the files stay in the project):

| Name | Reason |
|------|--------|
| `.build82/` (and the 12 legacy bare `PLUGIN_*.md` names) | Generated by build82 |
| `.indevelopment` | Development marker |
| `.git` | Version-control history (excluded in every mode) |
| `CLAUDE.md`, `GEMINI.md`, `AGENTS.md` | AI assistant context files |
| `.claudeignore`, `.geminiignore`, `.aiexclude` | AI tool config files |
| `.buildignore` | build82's own config file |
| `node_modules` | Dependencies |
| Names listed in `.buildignore` | Optional file at the plugin root: one base name per line, blank lines and `#` comments ignored |

**Returns (text):**
```
✅ Released local_caedauth (version 2026041000).

ZIP folder: caedauth/
Output: local_caedauth_2026041000.zip
Source: local/caedauth

Excluded from the archive:
- .indevelopment
- .build82
- CLAUDE.md
```
The first line names the component read from `version.php`. `Source` is relative to the Moodle root. `Output` is relative to the Moodle root when the ZIP is written inside it; otherwise it is the bare file name (the ZIP is in `output_dir`, or the working directory). The "Excluded" list contains only names actually found in the plugin.

**Symbolic links:** symbolic links (to files or directories) are never added to the ZIP, so a link pointing outside the plugin cannot pull its target into the archive. Each skipped link is listed after the "Excluded" list under `⚠️ Symbolic links skipped (never added to the archive):`, as a path relative to the plugin (never absolute).

**Example:**
```
Package the local_caedauth plugin for distribution.
```

---

## `create_plugin_skeleton`

Materializes a new Moodle plugin's directory structure on disk — deterministic scaffolding only, no generated business logic. No `format` parameter (plain text).

| Parameter | Type | Required | Default | Description |
|-----------|------|:---:|---------|-------------|
| `type` | string | ✅ | — | Plugin type; one of the values in the table below |
| `name` | string | ✅ | — | A name Moodle accepts: for `mod`, `^[a-z][a-z0-9]*$` (no underscores); for other types, lowercase letters, digits and single underscores, starting with a letter, ending with a letter or digit, at least 2 characters |
| `display_name` | string | ❌ | `name` with `_` replaced by spaces | Value of `$string['pluginname']` in the English language file (escaped for PHP) |
| `features` | string | ❌ | — | Comma-separated list of stubs to add (see below) |
| `requires` | string | ❌ | configured Moodle build number | `$plugin->requires`; must match `^\d+(\.\d+)?$`. If no build number is known, `0` is written with a `TODO` comment |
| `maturity` | string | ❌ | `MATURITY_ALPHA` | One of `MATURITY_ALPHA`, `MATURITY_BETA`, `MATURITY_RC`, `MATURITY_STABLE` |

**Target directory:** `{moodle_root}/{type directory}/{name}`. Accepted `type` values and their directories:

| Type | Directory | Type | Directory |
|------|-----------|------|-----------|
| `mod` | `mod` | `block` | `blocks` |
| `local` | `local` | `tool` | `admin/tool` |
| `auth` | `auth` | `enrol` | `enrol` |
| `theme` | `theme` | `report` | `report` |
| `format` | `course/format` | `filter` | `filter` |
| `qtype` | `question/type` | `availability` | `availability/condition` |
| `assignsubmission` | `mod/assign/submission` | `assignfeedback` | `mod/assign/feedback` |
| `gradereport` | `grade/report` | `gradeimport` | `grade/import` |
| `gradeexport` | `grade/export` | `plagiarism` | `plagiarism` |
| `portfolio` | `portfolio` | `repository` | `repository` |
| `profilefield` | `user/profile/field` | `workshopform` | `mod/workshop/form` |
| `workshopallocation` | `mod/workshop/allocation` | `workshopeval` | `mod/workshop/evaluation` |
| `datafield` | `mod/data/field` | `datapreset` | `mod/data/preset` |
| `ltisource` | `mod/lti/source` | `ltiservice` | `mod/lti/service` |
| `quizaccess` | `mod/quiz/accessrule` | `scormreport` | `mod/scorm/report` |
| `tinymce` | `lib/editor/tinymce/plugins` | `atto` | `lib/editor/atto/plugins` |
| `editor` | `lib/editor` | `adminpresets` | `admin/presets` |
| `antivirus` | `lib/antivirus` | `calendartype` | `calendar/type` |
| `logstore` | `admin/tool/log/store` | `paygw` | `payment/gateway` |
| `mlbackend` | `lib/mlbackend` | `search` | `search/engine` |
| `aiplacement` | `ai/placement` | `aiprovider` | `ai/provider` |
| `cachelock` | `cache/locks` | `cachestore` | `cache/stores` |
| `communication` | `communication/provider` | `contenttype` | `contentbank/contenttype` |
| `coursereport` | `course/report` | `customfield` | `customfield/field` |
| `dataformat` | `dataformat` | `fileconverter` | `files/converter` |
| `gradepenalty` | `grade/penalty` | `gradingform` | `grade/grading/form` |
| `h5plib` | `h5p/h5plib` | `media` | `media/player` |
| `message` | `message/output` | `mnetservice` | `mnet/service` |
| `qbank` | `question/bank` | `qbehaviour` | `question/behaviour` |
| `qformat` | `question/format` | `smsgateway` | `sms/gateway` |
| `tiny` | `lib/editor/tiny/plugins` | `webservice` | `webservice` |

**Files written:**

| Always | Condition |
|--------|-----------|
| `version.php` (component, version `YYYYMMDD00` of today, requires, maturity, release `1.0.0`) | — |
| `lang/en/{component}.php` (`lang/en/{name}.php` for `mod`) | — |
| Entry-point file(s) by type: `mod` → `lib.php` (`{name}_add_instance`, `{name}_update_instance`, `{name}_delete_instance`, as Moodle calls them), `index.php`, `view.php`, `mod_form.php`; `block` → `block_{name}.php`; `auth` → `auth.php`; `tool` → `index.php`; every other type → `lib.php` | — |
| `db/install.xml` | `features` matches database/table; always for `mod` (with the instance table `{name}`: `id`, `course`, `name`, `intro`, `introformat`, `timecreated`, `timemodified`) |
| `db/tasks.php` | matches task |
| `db/services.php` | matches service/api |
| `db/events.php` + `classes/observer.php` | matches event |
| `db/access.php` | matches capability/permission |
| `settings.php` | matches setting |

`features` is split on commas; each token is matched case-insensitively by substring against `database`/`table`, `task`, `service`/`api`, `event`, `capabilit`/`permission`, `setting` (first match wins per token). Unrecognized tokens are ignored.

**Errors:** `❌ Unknown plugin type "<type>".`; `❌ name must start with a lowercase letter ...`; `❌ Resolved plugin path escapes the Moodle root.`; `❌ <path> already exists — create_plugin_skeleton never overwrites an existing plugin.`; `❌ Unrecognized maturity "<x>" ...`; `❌ requires must be a plain Moodle build number ...`; `❌ Failed to write <file>: <cause>` (the directories this call created are removed, so no partial skeleton is left behind; if that cleanup itself fails, the message says so).

**Returns (text):** `✅ Created skeleton for <component> at <path>` (path relative to the Moodle root; the "already exists" error also names the plugin by its Moodle-relative path) followed by the sorted list of written files.

Pair it with the [`scaffold_plugin` prompt](../prompts.md#scaffold-a-complete-plugin-with-ai-drafting) to have the AI fill in the implementation on top of the skeleton.

**Example:**
```
Create the skeleton for a new local plugin called "attendance_export" with database tables and a scheduled task.
```

---

## `bump_plugin_version`

Raises a plugin's `$plugin->version` in `version.php` and, optionally, adds the matching upgrade step to `db/upgrade.php`. Deterministic edit: only the assignments it changes are rewritten, every other byte is kept. Every change is computed and validated before anything is written.

| Parameter | Type | Required | Default | Description |
|-----------|------|:---:|---------|-------------|
| `plugin` | string | ✅ | — | Component (`local_myplugin`), relative path or absolute path |
| `version` | string | ❌ | today as `YYYYMMDD00`; the current version's integer part + 1 when today's is not greater | New `$plugin->version`; exactly ten digits and greater than the current version |
| `release` | string | ❌ | unchanged | New `$plugin->release`, written as a single-quoted PHP string (escaped); a single line without control characters |
| `maturity` | string | ❌ | unchanged | One of `MATURITY_ALPHA`, `MATURITY_BETA`, `MATURITY_RC`, `MATURITY_STABLE` |
| `upgrade_step` | boolean | ❌ | `false` | Also add an upgrade step for the new version to `db/upgrade.php` |
| `upgrade_description` | string | ❌ | — | One line written as a comment in the step (no control characters, no `?>`) |
| `format` | `text`/`json` | ❌ | `text` | Output format |

**`version.php`:** each assignment the call changes (`version` always; `release`/`maturity` when given) must appear exactly once outside comments and hold a plain literal — a number for `version`; a quoted string (no `$` interpolation) or a number for `release`; a `MATURITY_*` constant for `maturity`. Otherwise the call is refused and the assignment must be edited by hand.

**Upgrade step** (`upgrade_step: true`): inserted before the final `return true;` of the plugin's upgrade function — `xmldb_{name}_upgrade` for `mod`, `xmldb_{component}_upgrade` for every other type — using that line's indentation and the file's line endings (LF or CRLF):

```php
    if ($oldversion < 2026100900) {
        // <upgrade_description, when given>
        // TODO: implement this upgrade step.

        // local_myplugin savepoint reached.
        upgrade_plugin_savepoint(true, 2026100900, 'local', 'myplugin');
    }
```

The savepoint matches the plugin type: `upgrade_mod_savepoint(true, V, name)` for `mod`, `upgrade_block_savepoint(true, V, name)` for `block`, `upgrade_plugin_savepoint(true, V, type, name)` otherwise — the form the [`doctor`](#doctor) Upgrade Consistency section checks. A missing `db/upgrade.php` is created with an upgrade function that only returns `true`, and the step goes into it.

**Errors** (nothing is written): `❌ Unrecognized maturity "<x>" ...`; `❌ release must be a single line ...`; `❌ upgrade_description must be a single line ...`; `❌ version.php has no $plugin-><x> assignment; add it first`; `❌ version.php assigns $plugin-><x> <n> times ...`; `❌ version.php assigns $plugin-><x> a value that is not a plain literal ...`; `❌ version must have exactly ten digits (YYYYMMDDXX) ...`; `❌ version <v> is not greater than the current version <c>`; `❌ db/upgrade.php does not define function <f>() ...`; ``❌ <f>() in db/upgrade.php does not end with a `return true;` on its own line ...``; `❌ db/upgrade.php already has a step for version <v> ...` (a step for the new version or a later one); the plugin path errors listed under `generate_plugin_context`. If writing `version.php` fails after `db/upgrade.php` was written, `db/upgrade.php` is restored (or removed, with the `db/` directory the call created) and the message says so.

**Returns:** `✅ Bumped <component> from <old> to <new>.`, the plugin path relative to the Moodle root, the release and maturity written, the files changed (`db/upgrade.php (created)` when new), and the generated step. JSON: `{component, path, old_version, new_version, release?, maturity?, files_changed[], upgrade_file_created?, upgrade_step?}`.

It does not regenerate the plugin's context files; run `generate_plugin_context` (or let `watch_plugins` do it) afterwards.

**Example:**
```
Bump my local_attendance plugin to a new version, set the release to 1.2.0 and add an upgrade step for the new reports table.
```

---

## `check_coding_style`

Checks a plugin's PHP files against the **Moodle coding standard** with [`phpcs`](https://github.com/PHPCSStandards/PHP_CodeSniffer) and the [`moodlehq/moodle-cs`](https://github.com/moodlehq/moodle-cs) standard — both optional dependencies, see [Installation](../getting-started/installation.md#-prerequisites). Read-only: it never fixes files.

| Parameter | Type | Required | Default | Description |
|-----------|------|:---:|---------|-------------|
| `plugin` | string | ✅ | — | Component (`local_myplugin`), relative path or absolute path |
| `severity` | string | ❌ | `error` | `error` reports errors only; `warning` also reports warnings |
| `files` | string[] | ❌ | the whole plugin | Files or directories to check, relative to the plugin directory; each must exist inside it (symbolic links are resolved) |
| `limit` | integer | ❌ | `200` | Maximum number of messages returned |
| `format` | `text`/`json` | ❌ | `text` | Output format |

**Command run** (in the plugin directory, bounded to 120 s; no shell involved):

```bash
phpcs --standard=moodle --report=json --extensions=php -q \
  --ignore=*/.build82/*,*/vendor/*,*/node_modules/* [--warning-severity=0] -- <paths>
```

`--warning-severity=0` is added for `severity: "error"`. phpcs exits with a non-zero code when it finds violations; the result is taken from its JSON report whatever the exit code. Before running, the tool checks with `phpcs -i` (bounded to 30 s) that the `moodle` standard is installed.

**Reported paths** are relative to the plugin directory. When `phpcs` is a wrapper that runs it somewhere else (for example inside a container) and reports other absolute paths, the part after the plugin's Moodle-relative path (`blocks/html/…`) is used, so the container must mount the Moodle tree with the same layout.

**Returns (text):** the totals (`N error(s), N warning(s) in N file(s) checked; N fixable with phpcbf.`), then one table per file with violations — `Line | Col | Severity | Message | Sniff` — sorted by path, then by line and column; `✅ No violations found.` when clean; a `⚠ Showing N of M message(s)` note when `limit` cut the list. JSON: `{component, path, standard, severity, errors, warnings, fixable, files_checked, files[], returned, truncated?}`, each file `{file, errors, warnings, messages[]}`, each message `{line, column, severity, source, message, fixable?}`. `errors`/`warnings`/`fixable` are phpcs's totals, not limited by `limit`.

**Errors:** ``❌ phpcs was not found on PATH. To check the Moodle coding style, install it with `composer global require moodlehq/moodle-cs` ...``; `❌ phpcs was found, but the moodle coding standard is not installed. ...`; `❌ phpcs was found but does not work: <reason>` (`phpcs -i` failed or timed out); `❌ phpcs did not finish in time (limit 2m0s)`; `❌ phpcs produced no report: <phpcs message>`; `❌ severity must be "error" or "warning", got "<x>".`; `❌ files must be paths relative to the plugin directory, got "<x>"`; `❌ "<x>" is outside the plugin directory`; `❌ "<x>" does not exist in the plugin`; the plugin path errors listed under `generate_plugin_context`. See [Troubleshooting](../troubleshooting/common-issues.md#check_coding_style-says-phpcs-or-the-moodle-standard-is-missing).

When phpcs is available, the [`review_plugin` prompt](../prompts.md#review-a-plugin) with focus `all` or `standards` asks the agent to call this tool first.

**Example:**
```
Check local_attendance against the Moodle coding style, including warnings.
```

---

[← Back to Index](../index.md)
