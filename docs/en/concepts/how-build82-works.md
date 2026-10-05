🌐 [Português](../../pt-br/concepts/how-build82-works.md) | **English** | 🏠 [Index](../index.md)

---

# How build82 works

`build82` reads your Moodle installation from disk and gives your AI assistant deep and accurate knowledge of your codebase — without copying and pasting anything manually.

---

## The pipeline

The flow has three stages: **Extractors** read the PHP files, **Generators** transform that content into context `.md` files, and the **MCP Server** serves that context to the AI client via Tools, Resources, and Prompts.

```
Moodle PHP files (on disk)
         │
         ▼  Extractors
    Parse db/install.xml, db/events.php, db/hooks.php,
    lib/*.php, classes/, db/tasks.php, db/services.php,
    db/access.php, db/upgrade.php
         │
         ▼  Generators + mtime cache
    Write context .md files under .build82/:
    MOODLE_API_INDEX.md, PLUGIN_AI_CONTEXT.md, etc.
    (unchanged files are skipped via mtime cache)
         │
         ▼  MCP Server
    Serves context via Tools, Resources and Prompts
    to the AI client via stdio or HTTP
```

---

## Extractors

Each extractor parses a type of Moodle PHP or XML file and feeds the generators with structured data. Most PHP-parsing extractors have two interchangeable backends: a fast regex-based parser (the default) and an opt-in tree-sitter-based parser (`BUILD82_EXTRACTOR_BACKEND=treesitter`) that trades some speed for a handful of edge cases the regex backend can't express — see [Extractors](../architecture/extractors.md#two-backends-one-contract) for which extractors have both and when that actually matters.

| Extractor           | Parses                            | Produces                                                        |
| -------------------- | ---------------------------------- | ------------------------------------------------------------------ |
| `api.go`             | `lib/*.php`                        | Functions with PHPDoc visibility → `MOODLE_API_INDEX.md`           |
| `schema.go`          | `db/install.xml`                   | Database schema (tables, fields, keys) → `PLUGIN_DB_TABLES.md`     |
| `events.go`          | `db/events.php`                    | Event observer registrations → `PLUGIN_EVENTS.md`                  |
| `hooks.go`           | `db/hooks.php` + `classes/hook/`   | Hook API callbacks (4.3+) and legacy callback warnings → `PLUGIN_CALLBACK_INDEX.md` |
| `tasks.go`           | `db/tasks.php`                     | Scheduled task definitions → `PLUGIN_DEPENDENCIES.md`               |
| `services.go`        | `db/services.php`                  | Web service registrations → `PLUGIN_ENDPOINT_INDEX.md`             |
| `capabilities.go`    | `db/access.php`                    | Capability definitions → `PLUGIN_DEPENDENCIES.md`                  |
| `upgrade.go`         | `db/upgrade.php`                   | Upgrade step history → `PLUGIN_DEPENDENCIES.md`                    |
| `classes.go`         | `classes/**/*.php`                 | PHP classes, interfaces, traits and enums → `MOODLE_CLASSES_INDEX.md`, `PLUGIN_ARCHITECTURE.md` |
| `plugin.go`          | `version.php` + language files     | Plugin metadata → `PLUGIN_CONTEXT.md`                               |
| `settings.go`        | `settings.php`                     | Admin settings (`admin_setting_*`) → `PLUGIN_SETTINGS.md`           |
| `subplugins.go`      | `db/subplugins.json` (or legacy `db/subplugins.php`) | Subplugin types → `PLUGIN_DEPENDENCIES.md`, `PLUGIN_AI_CONTEXT.md` |

For the complete list of generated files, see the [Generated Files Reference](../reference/generated-files.md).

---

## Generators

Generators receive the output of the extractors and write structured Markdown files under `.build82/` — never directly into the Moodle root or a plugin root:

- **Global generators** — write 13 files under `{moodle_root}/.build82/` (`MOODLE_API_INDEX.md`, `MOODLE_PLUGIN_INDEX.md`, `MOODLE_DB_TABLES_INDEX.md`, etc.)
- **Plugin generators** — write 12 files under `{plugin_root}/.build82/` (`PLUGIN_AI_CONTEXT.md`, `PLUGIN_DB_TABLES.md`, `PLUGIN_FUNCTION_INDEX.md`, etc.)

The **mtime cache** compares the modification date of each source file with the corresponding `.md` file and skips regeneration when nothing changed — making subsequent runs much faster. It's persisted to `.build82/.cache.json`, so a server restart doesn't force a full re-scan.

> To force a full regeneration ignoring the cache, ask the assistant: _"Regenerate all Moodle indexes ignoring the cache"_. The AI will call `update_indexes` with `force=true`.

---

## Tools, Resources and Prompts

With the context files created, the MCP server exposes them to the AI client in three ways:

- **Tools** — the AI calls them explicitly to trigger actions (`init_moodle_context`, `search_api`, `get_plugin_info`, `watch_plugins`, etc.)
- **Resources** — the AI reads them passively as context, without explicit user action (`moodle://api-index`, `moodle://plugin/{component}`, etc.)
- **Prompts** — prebuilt templates that automatically inject context and guide the AI in complex tasks (`scaffold_plugin`, `review_plugin`, `debug_plugin`)

---

## See also

- [Why build82?](./why-build82.md)
- [Architecture](./architecture.md)
- [Tools Reference](../reference/tools.md)
- [Generated Files Reference](../reference/generated-files.md)

---

[🏠 Back to Index](../index.md)
