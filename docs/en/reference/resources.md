🌐 [Português](../../pt-br/reference/resources.md) | **English** | 🏠 [Index](../index.md)

---

# Resources Reference (MCP)

**Resources** expose the generated context files to MCP clients as read-only documents, without a tool call. Every resource has MIME type `text/markdown` and returns the content of one generated file, read from disk on each request.

## How the AI Uses Resources

The AI decides when to read a resource based on your question. You can be explicit to guide the query:

- _"Take a look at the resource `moodle://api-index` and see if there is a function to delete users in bulk."_
- _"Analyze the schema in `moodle://plugin/local_myplugin/database` and suggest missing indexes."_
- _"Before suggesting fixes, check the standards in `moodle://dev-rules`."_

Use `init_moodle_context` or `generate_plugin_context` to generate or update the content.

---

## Behavior

| Aspect | Detail |
|---|---|
| MIME type | `text/markdown` for all 14 resources and all 12 templates |
| Content source | The file is read from disk on every request; the server keeps no copy (except the 5-second listing cache of `moodle://plugins/with-context`) |
| Not initialized | Returns the placeholder `# Resource not available` (`build82 has not been initialized. Run the init_moodle_context tool ...`), not an error |
| File not generated yet | Returns `# <file> — Not found` with the relative expected path and the tool to run (`init_moodle_context`/`update_indexes` for global files, `generate_plugin_context` for plugin files), not an error |
| Protocol errors | Only when the configuration location cannot be resolved, or on an internal panic (`internal error while reading this resource: <cause>`) |
| When content changes | When the backing file is regenerated (see [Generated Files](./generated-files.md) for the triggers): by `init_moodle_context`, `update_indexes`, `generate_plugin_context`, `plugin_batch`, or the `watch_plugins` watcher |

---

## 🌍 Global Resources

Fixed URIs backed by files in `{moodle_root}/.build82/`.

| URI | Backing file | Content |
|---|---|---|
| `moodle://context` | `AI_CONTEXT.md` | Installation overview: version, directory purposes, key `lib/` APIs, coding guidelines |
| `moodle://index` | `MOODLE_AI_INDEX.md` | Master index linking every generated global file and every plugin AI context |
| `moodle://workspace` | `MOODLE_AI_WORKSPACE.md` | Workspace guide: version, all plugins, plugins in development, plugins with AI context |
| `moodle://api-index` | `MOODLE_API_INDEX.md` | Functions from core `lib/` (public and `@deprecated`), grouped by source file |
| `moodle://events-index` | `MOODLE_EVENTS_INDEX.md` | Event observers across all plugins |
| `moodle://tasks-index` | `MOODLE_TASKS_INDEX.md` | Scheduled tasks across all plugins |
| `moodle://services-index` | `MOODLE_SERVICES_INDEX.md` | Web service functions across all plugins |
| `moodle://db-tables` | `MOODLE_DB_TABLES_INDEX.md` | Database tables declared across all plugins |
| `moodle://classes-index` | `MOODLE_CLASSES_INDEX.md` | Classes, interfaces, traits and enums found under `classes/` directories (FQNs) |
| `moodle://capabilities-index` | `MOODLE_CAPABILITIES_INDEX.md` | Capabilities declared across all plugins |
| `moodle://plugin-index` | `MOODLE_PLUGIN_INDEX.md` | Component / type / name / version / path map of installed plugins |
| `moodle://dev-rules` | `MOODLE_DEV_RULES.md` | Coding standards, security rules, database conventions |
| `moodle://plugin-guide` | `MOODLE_PLUGIN_GUIDE.md` | Component naming, required files, `version.php` template, autoloaded class paths |
| `moodle://plugins/with-context` | — | Table with columns Component, Type, Path of plugins that have `.build82/PLUGIN_AI_CONTEXT.md`, sorted by component, plus a usage snippet. Computed by scanning the Moodle tree (skipping `vendor/` and `node_modules/`); the result is cached for 5 seconds |

The `tags` file is not exposed as a resource.

---

## 🧩 Plugin Resources

Twelve URI templates, one per generated plugin file. `{component}` is a plugin component such as `local_myplugin`, `mod_assign` or `block_html`. It is resolved as: the type prefix mapped to its directory (an unknown prefix is used as a directory name as-is) plus the name after the first underscore; the result must exist and be inside the Moodle root, otherwise the response is `# Plugin not found` with the text `Could not resolve "<component>" to a plugin directory under the configured Moodle root.` (no filesystem path is included). Templates cannot be enumerated by the client; use `moodle://plugins/with-context` to list plugins.

| URI template | Backing file (`{plugin}/.build82/`) | Content |
|---|---|---|
| `moodle://plugin/{component}` | `PLUGIN_AI_CONTEXT.md` | Combined AI context — recommended entry point |
| `moodle://plugin/{component}/context` | `PLUGIN_CONTEXT.md` | Metadata (component, type, version, requires, display name, maturity, path) and feature counts |
| `moodle://plugin/{component}/structure` | `PLUGIN_STRUCTURE.md` | Directory tree (2 levels) and key-files checklist |
| `moodle://plugin/{component}/architecture` | `PLUGIN_ARCHITECTURE.md` | Class count and classes grouped by directory |
| `moodle://plugin/{component}/settings` | `PLUGIN_SETTINGS.md` | Admin settings declared in `settings.php` |
| `moodle://plugin/{component}/functions` | `PLUGIN_FUNCTION_INDEX.md` | Top-level PHP functions, grouped by file |
| `moodle://plugin/{component}/database` | `PLUGIN_DB_TABLES.md` | Schema from `db/install.xml` |
| `moodle://plugin/{component}/events` | `PLUGIN_EVENTS.md` | Event observers from `db/events.php` |
| `moodle://plugin/{component}/callbacks` | `PLUGIN_CALLBACK_INDEX.md` | Legacy `lib.php` callbacks and Hook API registrations |
| `moodle://plugin/{component}/endpoints` | `PLUGIN_ENDPOINT_INDEX.md` | Web services, AJAX endpoints and AMD modules |
| `moodle://plugin/{component}/dependencies` | `PLUGIN_DEPENDENCIES.md` | Tasks, services, capabilities, Hook API, upgrade history, subplugins |
| `moodle://plugin/{component}/flow` | `PLUGIN_RUNTIME_FLOW.md` | Entry points, core logic files and class/event/task/service summary |

---

## See also

- [Tools Reference](./tools.md) — actions the AI can actively execute
- [Prompts Reference](./prompts.md) — templates for scaffold, review, and debugging
- [Generated Files](./generated-files.md) — the `.md` files that feed these resources

---

[🏠 Back to Index](../index.md)
