🌐 [Português](../pt-br/prompts.md) | **English** | 🏠 [Index](./index.md)

---

# Example Prompts

Example requests you can type to your AI agent (Claude Code, Codex, Gemini, ...) so it uses build82. Write them in plain language: you never need to name a tool or write JSON. The agent picks the right build82 tool, MCP prompt, or resource from what you ask.

Replace `local_myplugin`, `local_reports`, and similar names with your own plugins. Phrasing is free; the examples only show which information to include.

**Prerequisite for almost everything:** build82 must be initialized once for your Moodle installation (see [Initialize build82](#initialize-build82)). Plugins to be tracked by the watcher and the `dev` batch mode must be marked `.indevelopment`.

## Categories

- [Setup and Maintenance](#setup-and-maintenance)
- [Analysis and Refactoring](#analysis-and-refactoring)
- [Automation and Generation](#automation-and-generation)
- [Testing and Validation](#testing-and-validation)
- [Working Across Sessions](#working-across-sessions)

---

## Setup and Maintenance

### Initialize build82

- **Expected parameters:** absolute path of the Moodle root. Say so if you want to re-initialize an installation that is already configured.
- **Example:**
  > Set up build82 for my Moodle installation at /var/www/moodle.
- **Expected output:** the agent calls `init_moodle_context`. It validates the path, detects the Moodle version, saves the configuration, and generates all global index files. You get a summary of the detected version and the files created. With "force" wording ("initialize again from scratch"), it re-initializes even if a configuration exists.

### Refresh the global indexes

- **Expected parameters:** none. Optionally say to include your development plugins, or to ignore the cache.
- **Example:**
  > I just upgraded Moodle. Refresh all the build82 indexes, including my plugins in development, and ignore any cache.
- **Expected output:** the agent calls `update_indexes` (with plugin regeneration and cache bypass when requested). It re-detects the Moodle version and regenerates the 13 global index files; the text reply gives the Moodle version and the counts of regenerated, cached and failed global indexes (plus one line per dev plugin when included); ask for JSON to get the file lists. When you ask to ignore the cache, every global file (and every dev plugin file, if requested) is really rewritten, even if its sources did not change.

### Watch plugins while you code

- **Expected parameters:** the plugins must already be marked `.indevelopment` (see [Mark a plugin as under development](#mark-a-plugin-as-under-development)).
- **Example:**
  > Start watching my dev plugins so their context stays up to date while I edit. Later, tell me if the watcher is still running, and stop it when I say so.
- **Expected output:** the agent calls `watch_plugins` with the start, status, or stop action. While it runs, changes to dev plugins regenerate their context automatically, and every connected session is notified when a regeneration finishes. If no plugin is marked `.indevelopment` (or none has watchable files), the watcher is not started and you are told so; mark a plugin first and ask again.

### Mark a plugin as under development

- **Expected parameters:** the plugin component or path.
- **Example:**
  > Generate the context for local_myplugin and keep treating it as a plugin I'm actively developing.
- **Expected output:** `generate_plugin_context` marks the plugin `.indevelopment` as a side effect. From then on, `watch_plugins`, the `dev` batch mode, `list_dev_plugins`, and `doctor` include it. You can also create the marker manually with `mkdir -p <moodle>/local/myplugin/.build82 && touch <moodle>/local/myplugin/.build82/.indevelopment`.

---

## Analysis and Refactoring

### Find a plugin

- **Expected parameters:** any search term (part of a component name, a plugin type, a name). Optionally a maximum number of results.
- **Example:**
  > Which installed plugins have "report" in their name? Show me only the first 10.
- **Expected output:** the agent calls `search_plugins`. You get matching plugins with component, type, name, version, and path (fuzzy matching is used when there is no exact hit).

### Look up a Moodle API function

- **Expected parameters:** a function name or a keyword from its description. Say if you want deprecated functions too, or only those.
- **Example:**
  > Is there a core function to format a date for the user's timezone? Also tell me if anything similar is deprecated.
- **Expected output:** the agent calls `search_api` (visibility `public`, `deprecated`, or `all`). You get matching `lib/` functions with their summary and source file, so the agent does not invent APIs.

### Get a plugin's details

- **Expected parameters:** the plugin (component, relative path, or absolute path).
- **Example:**
  > Give me the details of local_myplugin: version, dependencies, what it declares.
- **Expected output:** the agent calls `get_plugin_info`. You get the generated AI context for the plugin, or live-detected metadata if context was never generated.

### List the plugins I'm developing

- **Expected parameters:** none.
- **Example:**
  > Which plugins am I currently developing?
- **Expected output:** the agent calls `list_dev_plugins` and lists every plugin marked `.indevelopment`.

### Understand a plugin quickly

- **Expected parameters:** the plugin (component like `local_myplugin`, path relative to the Moodle root like `local/myplugin`, or absolute path), and optionally the aspect you care about: overview, database, events, classes, services, or flow.
- **Example:**
  > Explain local_myplugin to me, but only its database tables.
  >
  > Give me a short overview of mod_checklist, then walk me through its runtime flow.
- **Expected output:** the agent calls `explain_plugin` with the matching section (or all sections). You get a compact explanation that is cheaper to read than the full index files.

### Review a plugin

- **Expected parameters:** the plugin, and optionally a focus (security, performance, coding standards, database, core APIs, or everything; an unrecognized focus is rejected with an error listing the accepted values) and specific files.
- **Example:**
  > Do a full code review of local_myplugin focused on security: missing capability checks, unescaped output, unvalidated parameters, and direct SQL.
- **Expected output:** the agent uses the `review_plugin` prompt (focus `security`) and loads the plugin context. You get a findings report organized by the criteria checklist for that focus, with a suggested fix for each issue.

### Review only specific files

- **Expected parameters:** the plugin and the list of files.
- **Example:**
  > Review only lib.php and classes/external/api.php of local_myplugin for coding standards.
- **Expected output:** the `review_plugin` prompt with the `files` list and the `standards` focus; the review is restricted to those files.

### Review database usage

- **Expected parameters:** the plugin.
- **Example:**
  > Review the database interactions in local_myplugin. Look for missing indexes, inefficient queries, and places where get_records_sql is used instead of get_records.
- **Expected output:** `review_plugin` with focus `database`, using the plugin's AI context (`PLUGIN_AI_CONTEXT.md`, when generated), the database review criteria, and the code the agent reads. You get issues grouped by table or query.

### Find performance problems

- **Expected parameters:** the plugin.
- **Example:**
  > Analyze local_myplugin for performance issues: N+1 queries, IN queries without get_in_or_equal, unindexed WHERE columns, and expensive queries with no caching.
- **Expected output:** `review_plugin` with focus `performance`. You get a prioritized list of findings with code references.

### Check use of core APIs

- **Expected parameters:** the plugin.
- **Example:**
  > Check whether local_myplugin uses the Moodle core APIs correctly, and whether it uses any deprecated function.
- **Expected output:** `review_plugin` with focus `apis`, combined with `search_api` (deprecated visibility) when needed. You get misuse and deprecation findings with replacements.

### Debug a plugin error

- **Expected parameters:** the plugin, the full error message or stack trace, and when it happens.
- **Example:**
  > local_myplugin throws "Table 'mdl_local_myplugin_data' doesn't exist" when a teacher opens the settings page. What is the root cause and how do I fix it?
- **Expected output:** the agent uses the `debug_plugin` prompt with plugin, error, and context. Moodle-specific hints are selected from keywords in the error (capability, database, class not found, event/observer, task/cron, web service/AJAX), or generic hints when nothing matches. You get a diagnosis and a proposed fix.

### Debug a task, class, or permission error

- **Expected parameters:** the plugin and the exact error text.
- **Example:**
  > The scheduled task \local_myplugin\task\cleanup_task fails with "Permission denied" on cron runs. What capability or file permission could be causing it?
  >
  > local_myplugin triggers "Call to undefined method local_myplugin\output\renderer::render_summary()". Where should that method be defined according to the plugin structure?
- **Expected output:** `debug_plugin` plus the plugin's structure and dependencies context (resources `moodle://plugin/{component}/structure` and `/dependencies`). You get the likely location of the problem and how to fix it.

### Trace the runtime flow

- **Expected parameters:** the plugin and the scenario.
- **Example:**
  > Trace what happens in local_myplugin from its main entry points when a teacher opens the plugin's main page.
- **Expected output:** `explain_plugin` with section `flow`, or the `moodle://plugin/{component}/flow` resource. You get the entry points and execution path step by step.

### Migrate legacy callbacks to the Hook API

- **Expected parameters:** the plugin and the target Moodle version (4.3 or later).
- **Example:**
  > Check local_myplugin for legacy lib.php callbacks that were replaced by the Hook API in Moodle 4.3+, and show the migration steps for each one.
- **Expected output:** the agent reads the callbacks and hook context (`/callbacks` resource), consults the plugin guide (`moodle://plugin-guide`) and `search_api` if needed. You get a list of legacy callbacks with the replacement hook and the code changes.

### Read the Moodle reference indexes

- **Expected parameters:** which index you want (API, events, tasks, services, DB tables, classes, capabilities, plugins, coding rules, plugin guide).
- **Example:**
  > What are the Moodle coding and security rules I should follow here?
  >
  > Which scheduled tasks and event observers already exist across my installation?
- **Expected output:** the agent reads the global resources (`moodle://dev-rules`, `moodle://tasks-index`, `moodle://events-index`, and the others such as `moodle://api-index`, `moodle://db-tables`, `moodle://classes-index`, `moodle://capabilities-index`, `moodle://plugin-index`, `moodle://services-index`, `moodle://workspace`, `moodle://index`, `moodle://context`) and answers from them.

### List plugins that already have AI context

- **Expected parameters:** none; plugins appear only after their context was generated (`.build82/PLUGIN_AI_CONTEXT.md` exists).
- **Example:**
  > Which plugins in my Moodle already have build82 AI context generated?
- **Expected output:** the agent reads the `moodle://plugins/with-context` resource. You get a table of component, type and path for each plugin with generated context, sorted by component.

### Check a plugin's metadata and admin settings

- **Expected parameters:** the plugin component (e.g. `local_myplugin`); its context must already be generated.
- **Example:**
  > What version, Moodle requirement and maturity does local_myplugin declare, and which admin settings does it have?
- **Expected output:** the agent reads `moodle://plugin/{component}/context` (metadata and feature counts) and `moodle://plugin/{component}/settings` (settings declared in `settings.php`). You get the plugin's metadata and its list of admin settings.

### Explore a plugin's classes and endpoints

- **Expected parameters:** the plugin component; its context must already be generated.
- **Example:**
  > How are the classes of local_myplugin organized, and which web services, AJAX endpoints and AMD modules does it expose?
- **Expected output:** the agent reads `moodle://plugin/{component}/architecture` (classes grouped by directory) and `moodle://plugin/{component}/endpoints` (web services, AJAX endpoints and AMD modules). You get an overview of the class layout and the plugin's entry points for clients.

---

## Automation and Generation

### Scaffold a plugin skeleton

- **Expected parameters:** plugin type (local, mod, block, ...), name (lowercase letters, digits, underscores; must start with a letter). Optionally a display name, the features to stub (database, tasks, services, events, capabilities, settings), the minimum Moodle version, and the maturity. The target directory must not exist.
- **Example:**
  > Create the skeleton of a local plugin called reports, with stubs for database tables, scheduled tasks, and capabilities. Name it "Custom Reports" and mark it as beta.
- **Expected output:** the agent calls `create_plugin_skeleton`. It writes `version.php`, `lang/en/local_reports.php`, the type's mandatory entry-point files, and stub `db/*.php` files for the requested features. Nothing is generated for business logic, and it refuses to overwrite an existing plugin.

### Scaffold a complete plugin with AI drafting

- **Expected parameters:** type, name, what the plugin does, and the features you need (database tables, scheduled tasks, web services, events, capabilities, settings). Best combined with a skeleton created first.
- **Example:**
  > Scaffold a complete Moodle 4.4 local plugin called local_reports that generates attendance and participation reports. It needs a config table per course and a cache table with expiry, a scheduled task that rebuilds expired cache every 6 hours, a capability local/reports:viewreports for teachers, a web service that returns the report as JSON, and an observer for course_viewed events. Follow Moodle coding standards.
- **Expected output:** the agent uses the `scaffold_plugin` prompt (type, name, description, features), which injects Moodle context and a worked example, and often runs `create_plugin_skeleton` first. You get all required files written with proper Moodle structure and conventions.

### Scaffold an activity module

- **Expected parameters:** module name, purpose, tables, capabilities, and the pages you need.
- **Example:**
  > Scaffold a Moodle 4.4 activity module called mod_checklist where teachers create checklists that students complete. Include the standard lib.php callbacks, a checklist_item table, capabilities for students to submit and teachers to manage, and a view page with completion tracking.
- **Expected output:** `scaffold_plugin` with type `mod`, plus `create_plugin_skeleton` for the mandatory mod files. You get the full module structure.

### Scaffold a block

- **Expected parameters:** block name, purpose, formats where it applies, and capabilities.
- **Example:**
  > Scaffold a Moodle 4.4 block called block_coursestats that shows course statistics in a sidebar block on course pages, with a view capability.
- **Expected output:** `scaffold_plugin` with type `block` (and optionally `create_plugin_skeleton`). You get the block class, capability, and language strings.

### Generate a plugin's context

- **Expected parameters:** the plugin (component like `local_myplugin`, path relative to the Moodle root like `local/myplugin`, or absolute path).
- **Example:**
  > Generate the AI context for local_myplugin so you understand it before we start.
- **Expected output:** the agent calls `generate_plugin_context`. It creates the 12 `PLUGIN_*` context files (metadata, structure, DB tables, events, dependencies, functions, callbacks, endpoints, runtime flow, architecture, settings, and the combined AI context) and marks the plugin `.indevelopment`.

### Generate context for my dev plugins

- **Expected parameters:** none; plugins must be marked `.indevelopment`. Optionally say to force regeneration or to run in parallel.
- **Example:**
  > Regenerate the context of all my development plugins, ignoring the cache, using 4 workers.
- **Expected output:** the agent calls `plugin_batch` with mode `dev` (default), `force`, and `parallel`. You get a per-plugin success or failure summary.

### Generate context for every plugin

- **Expected parameters:** none. Say if you also want them all marked as under development.
- **Example:**
  > Generate context for every plugin in the installation and mark them all as in development.
- **Expected output:** `plugin_batch` with mode `all` and `mark_as_dev`. You get a per-plugin summary; sequential by default, so large installations can take a while.

### Generate context for a chosen list of plugins

- **Expected parameters:** the plugins (components, relative paths, or absolute paths).
- **Example:**
  > Generate context for local_reports, mod_checklist, and block_coursestats.
- **Expected output:** `plugin_batch` with mode `list` and the plugin identifiers. Unresolvable identifiers are reported by name.

### Get machine-readable output

- **Expected parameters:** which operation, and that you want structured (JSON) output.
- **Example:**
  > Run a health check and give me the result as JSON so I can feed it to a script.
- **Expected output:** the agent passes the `json` format to tools that support it (`doctor`, `init_moodle_context`, `update_indexes`, `search_plugins`, `search_api`, `get_plugin_info`, `list_dev_plugins`, `generate_plugin_context`, `plugin_batch`) and returns a structured response instead of Markdown.

### Generate PHPDoc

- **Expected parameters:** the plugin, and optionally the files or classes.
- **Example:**
  > Load local_myplugin and write a PHPDoc block for every public function and method that is missing one, with accurate @param, @return, and @throws tags.
- **Expected output:** the agent loads the plugin context (`generate_plugin_context` / `moodle://plugin/{component}/functions`) and edits the source files. You get documented code.

### Generate a database upgrade

- **Expected parameters:** the plugin, the schema changes (new columns, indexes, renames).
- **Example:**
  > local_myplugin needs a DB upgrade: add a "status" column (tinyint, default 0) to local_myplugin_data, add an index on (userid, status), and rename old_value to previous_value in local_myplugin_logs. Load the current schema and update db/upgrade.php, db/install.xml, and version.php.
- **Expected output:** the agent reads the schema (`moodle://plugin/{component}/database`) and writes the upgrade step, the updated `install.xml`, and the bumped `$plugin->version`.

### Package a plugin for release

- **Expected parameters:** the plugin (component like `local_myplugin`, path relative to the Moodle root like `local/myplugin`, or absolute path), optionally an existing output directory.
- **Example:**
  > Package local_myplugin as a ZIP in ~/releases.
- **Expected output:** the agent calls `release_plugin`. It builds a ZIP that excludes build82's own generated files and other non-shippable artifacts (also respecting `.buildignore`) and reports the released component and version, the source (relative to the Moodle root), the ZIP location (relative to the Moodle root when inside it, otherwise just the file name), and what was excluded. The ZIP is written atomically, so a failure never leaves a partial file.

---

## Testing and Validation

### Run a health check

- **Expected parameters:** none.
- **Example:**
  > Run a full health check of build82 and tell me what needs fixing.
- **Expected output:** the agent calls `doctor`. You get the checks for system dependencies (`php`, `ctags`, `git`), configuration, Moodle installation, global index freshness, development plugins, legacy files pending migration, cross-plugin consistency, deprecated core API usage, own-capability checks, own-language-string usage, and cache stats, plus an overall verdict.

### Find deprecated API usage

- **Expected parameters:** development plugins marked `.indevelopment`.
- **Example:**
  > Do any of my dev plugins call deprecated Moodle functions? What should I use instead?
- **Expected output:** `doctor` (deprecated API section) plus `search_api` with deprecated visibility for replacements.

### Check capabilities and language strings

- **Expected parameters:** development plugins marked `.indevelopment`.
- **Example:**
  > Check that my dev plugins only check capabilities they declare and only use language strings that exist.
- **Expected output:** `doctor` (capability and lang-string consistency sections). You get the missing entries per plugin.

### Check consistency between plugins

- **Expected parameters:** at least two development plugins.
- **Example:**
  > Do any of the plugins I'm developing declare the same capability name?
- **Expected output:** `doctor` (cross-plugin consistency section). It only compares capability names declared in each plugin's `db/access.php`; table names and events are not checked.

### Verify a plugin is ready for moodle.org

- **Expected parameters:** the plugin (component, relative path, or absolute path).
- **Example:**
  > Package local_myplugin, but first check that it meets the moodle.org plugin directory requirements. Don't build the ZIP if something is missing.
- **Expected output:** `release_plugin` with `strict` on. It validates `version.php` fields (component, requires, maturity), the language file, the privacy provider, `thirdpartylibs.xml` when a `thirdparty/` folder exists, and the absence of a `.git` directory. If any check fails, it lists the issues and builds no ZIP.

### Generate PHPUnit tests

- **Expected parameters:** the class to test (fully qualified name) and the plugin.
- **Example:**
  > Generate a PHPUnit test class for \local_myplugin\util\data_processor. Load the plugin context first, then cover every public method using advanced_testcase, with fixtures and at least two cases per method.
- **Expected output:** the agent loads the plugin context and class index (`moodle://plugin/{component}/functions`, `moodle://classes-index`) and writes the test class in `tests/`.

### Verify freshness before working

- **Expected parameters:** none.
- **Example:**
  > Before we start, make sure the build82 context is fresh for my dev plugins, and refresh whatever is stale.
- **Expected output:** `doctor` to detect stale indexes (older than 7 days), followed by `update_indexes` and/or `plugin_batch` to regenerate.

---

## Working Across Sessions

Long tasks (a review, a debugging session, a scaffold) span several conversations. Give the agent the current state and it loads the plugin context again.

### Resume development

- **Expected parameters:** the plugin and where you stopped.
- **Example:**
  > Load the context for local_myplugin. Continuing from yesterday: the structure was created with the scaffold. Next step is the scheduled task in classes/task/sync_task.php.
- **Expected output:** the agent reads the plugin AI context (`moodle://plugin/{component}` or `get_plugin_info`) and continues from your stated point.

### Resume a review

- **Expected parameters:** the plugin, the review focus, what was already reviewed, and what is next.
- **Example:**
  > Load local_myplugin. I'm continuing yesterday's security review. Already reviewed: lib.php, db/install.xml, and index.php. Next: classes/external/ and classes/task/.
- **Expected output:** the agent loads the plugin context and uses `review_plugin` restricted to the remaining files.

### Resume a debugging session

- **Expected parameters:** the plugin, the error, what you already checked, and what is still unknown.
- **Example:**
  > Load local_myplugin. Continuing the "Table doesn't exist" investigation for mdl_local_myplugin_data. Already checked: db/install.xml is correct and the plugin was reinstalled. It only fails in course context. Continue from here.
- **Expected output:** the agent loads the context and continues with `debug_plugin`-style analysis, without repeating the checks you listed.

### Record progress in the project notes

- **Expected parameters:** what to record, and the notes file your agent uses (for example `CLAUDE.md`).
- **Example:**
  > Update CLAUDE.md with the current state: local_myplugin was reviewed for security in lib.php and index.php, and the "Table doesn't exist" error was caused by X, fixed with Y.
- **Expected output:** the agent edits your notes file; no build82 tool is needed. Next session it can read that file and reload the plugin context.

Some clients can also save and resume whole conversations (for example `/chat save` and `/chat resume` in Antigravity CLI, or `codex resume --last` in Codex).

---

## See Also

- [MCP prompts reference](./reference/prompts.md) — arguments and behavior of `scaffold_plugin`, `review_plugin`, and `debug_plugin`.
- [Tools reference](./reference/tools.md) — parameters of every tool.
