# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.3.0] - 2026-11-05

### Changed

- `watch_plugins` watches every file of each development plugin, at any depth (except in
  `vendor/`, `node_modules/`, `.git/` and `.build82/`), including directories created while it
  runs. Before, it only watched `version.php`, `lib.php`, `locallib.php`, `settings.php` and the
  `db/` files, so editing a class, template or AMD module did not refresh the plugin's context,
  although the context is built from all of them. build82's own outputs do not trigger a
  regeneration.
- With the tree-sitter backend, a hook source it cannot parse (a file over its 2 MiB cap, or a
  parse that times out) makes that plugin's hook extraction fall back to the regex backend, with a
  warning on stderr. Before, the plugin's hooks and legacy callbacks were silently reported as
  empty.
- `doctor` reads each PHP file of a development plugin once for its deprecated-API, capability
  and lang-string checks, instead of once per check.
- gotreesitter 0.56.0 for the optional tree-sitter backend; its results are unchanged on the
  real Moodle installations used for the parity tests.
- Releases are signed with cosign v3.1.4.

### Removed

- The watcher's MCP log notification (`build82/watcher`), and the `logging` capability the server
  advertised. MCP logging is deprecated by the protocol; a client is told about a regeneration by
  the `notifications/resources/updated` of the resources it subscribed to.

### Development

- `TestRun_InterruptedPromptChangesNothing` cancels once the confirmation prompt is shown instead
  of after a fixed delay, which a slow CI runner could hit during the release lookup.

## [1.2.0] - 2026-11-05

### Security

- Generated context files are no longer written through symbolic links. A plugin shipping
  `.build82` as a link (or a link in place of one of its outputs) could make
  `generate_plugin_context`, `plugin_batch`, the watcher or legacy-file migration overwrite files
  outside the plugin. A symlinked `.build82` is now refused, outputs are written to a temp file and
  renamed into place (a planted link is replaced, and readers never see a partial file), and an
  output that is a link is regenerated.
- A `$plugin->component` that is not a frankenstyle name (such as `../../x`) is ignored and the
  plugin is named `type_name` after its directory. Before, it went into file paths: it could make
  `release_plugin` write the ZIP outside `output_dir` and make the extractors read files outside the
  plugin.
- The Moodle root itself, and directories without `version.php`, are no longer accepted as plugins
  (`generate_plugin_context .` marked the whole installation as a development plugin), and component
  identifiers such as `mod_..` are no longer joined into paths.
- A path that does not exist yet is checked through the symbolic links of its existing directories,
  so `create_plugin_skeleton` no longer writes through a plugin type directory linked outside the
  Moodle root.
- Plugin-controlled names (file and directory names, `db/subplugins.json` paths) can no longer
  break the Markdown structure of generated files.
- Configuration, cache, installer and scaffold writes are flushed to disk before the rename, and on
  Windows a rename blocked by another process is retried.

### Added

- Every field of the output schemas (the 11 tools with structured output) carries a description, so
  clients that read `structuredContent` know what each field means.
- Prompt `upgrade_plugin` (arguments `plugin`, `target_version`): builds a migration plan for a
  plugin to a newer Moodle release from its `requires`/`supported`, its calls to deprecated core
  functions (file and line), its legacy callbacks that have a Hook API replacement and its
  upgrade-step findings, noting that the deprecated list reflects the installed Moodle.
- Tool `check_coding_style`: checks a plugin's PHP files (or chosen files) against the Moodle
  coding standard with `phpcs` and `moodlehq/moodle-cs` (optional dependencies) and returns the
  violations per file — line, column, severity, sniff and message — errors only by default.
  `doctor` reports whether `phpcs` with the moodle standard is usable, and the `review_plugin`
  prompt asks the agent to run the check when it is.
- Tool `bump_plugin_version`: raises a plugin's `$plugin->version` (default: today as
  `YYYYMMDD00`, or the current version + 1) and optionally its release and maturity, rewriting only
  those assignments in `version.php`; with `upgrade_step` it also adds an `if ($oldversion < V)`
  block with the savepoint matching the plugin type to `db/upgrade.php`, creating the file when
  missing. It refuses, without writing anything, when an assignment is missing, duplicated or not a
  literal, or the upgrade function cannot be located safely.
- `doctor` checks each development plugin's `db/upgrade.php` against `db/install.xml` and
  `version.php` (missing or wrong savepoints, steps out of order or newer than the plugin version,
  tables or fields a fresh install would lack, removed ones `install.xml` still defines), and its
  Privacy API provider (missing, implementing no metadata interface, or a `null_provider` for a
  plugin whose tables store user data).
- Resource subscriptions: the server advertises `resources.subscribe`, and a client subscribed to a
  resource is sent `notifications/resources/updated` whenever `init_moodle_context`,
  `generate_plugin_context`, `plugin_batch`, `update_indexes` or the `watch_plugins` watcher
  regenerates its file. The watcher's log notification is kept for now, but MCP logging is
  deprecated and that message may be removed in a later release.

### Changed

- The optional tree-sitter extraction backend (`BUILD82_EXTRACTOR_BACKEND=treesitter`) is about
  4.5× faster and uses far less memory per file (gotreesitter 0.55.1); its results are unchanged.
- Releases are signed with cosign v3.1.3.
- Release binaries are built with `-buildvcs=false`, so they carry no git revision or
  modified-tree flag and a clean checkout, a modified one or a source tarball of the tag produce the
  same bytes.
- A release is published only for a tag whose commit is on `main`. The MCP Registry entry is
  published from the same verified build artifact as the release assets, after checking that its
  `fileSha256` matches the signed `build82.mcpb`, instead of from the `server.json` downloaded back
  from the release.

### Removed

- **The HTTP mode.** `build82 --http` (Streamable HTTP and SSE), its flags `--port`, `--host`,
  `--token` and `--allowed-host`, and the `BUILD82_TOKEN` variable are gone: build82 is a local
  development tool and runs over stdio only, which is what `install`, the Claude Desktop extension
  and every client guide already used. The mode exposed tools that write files over the network
  and had its own open issues (one server per request, notifications reaching only the calling
  session, idle sessions never closed). Those flags are now usage errors (exit code 2) that say the
  mode was removed. With Moodle in Docker, run build82 on the host and point it at the mounted
  Moodle directory (see the Docker guide).

### Fixed

- `doctor`'s Capability Usage check compares calls against `type/name:` — the capability form
  Moodle requires for every plugin type — instead of the frankenstyle component, so it now works for
  `local`, `tool`, `report`, `enrol` and every other non-`mod`/`block` plugin (it reported nothing
  for them before). The `create_plugin_skeleton` `db/access.php` example uses the same form.
- Activity modules: the language file is read from `lang/en/<name>.php` (as Moodle names it, e.g.
  `forum.php`) for the display name, `doctor`'s Lang String Usage and `release_plugin --strict`;
  `get_string()` calls naming the module as `mod_forum` or `forum` both count as its own (before,
  every call was flagged as undeclared and the short form was ignored); legacy callbacks declared
  with the short name (`forum_before_footer`) are detected.
- `create_plugin_skeleton` for `mod`: the instance callbacks are `<name>_add_instance`,
  `<name>_update_instance` and `<name>_delete_instance` (the names Moodle calls; they were prefixed
  with `mod_`), `db/install.xml` is always generated with the instance table, the language file is
  `lang/en/<name>.php`, and names follow Moodle's rules (no underscore in a module name; no doubled
  or trailing underscore elsewhere). The `scaffold_plugin` prompt's module notes were corrected too.
- The regex backend (the default) finds the PHPDoc of functions and classes across PHP 8
  `#[...]` attributes and with a `**/` closer, so the deprecated functions of Moodle 4.5+ (which
  carry `#[\core\attribute\deprecated]`) appear in the API index and in `doctor` and
  `upgrade_plugin` again (on Moodle 5.0: 41 instead of 1); it also finds `final` and `readonly`
  classes.
- Hook, event, task and service registrations written with `X::class` (alone,
  `X::class . '::method'` or `[X::class, 'method']`), the form Moodle documents, are read by both
  backends instead of being dropped; hook definitions read their label, tags and replaced callbacks
  from the `#[\core\attribute\...]` attributes.
- The tree-sitter backend reads `TRUE`/`True` as true, like PHP and the regex backend.
- `MOODLE_PLUGIN_GUIDE.md`'s `version.php` template set `$plugin->version` and `$plugin->requires` to
  the Moodle release (`4.5`); it now uses a date-based plugin version and the installation's build
  number.
- The plugin links in `MOODLE_AI_INDEX.md` resolved to a missing path; they are now relative to the
  index's own directory.
- Subplugin paths from `db/subplugins.json` are relative to the Moodle root (`mod/assign/submission`)
  instead of being overwritten by the plugin-relative `subplugintypes` value (`submission`).
- The plugin type map gains the 22 core plugin types it lacked (`qbank`, `qbehaviour`, `qformat`,
  `customfield`, `media`, `message`, `cachestore`, `aiprovider`, `tiny`, ...) and `portfolio`
  plugins are looked up in `portfolio/`, so these plugins are found, resolved by component and typed
  correctly.
- Legacy `lib.php` callbacks are matched against the callbacks Moodle core declares as replaced by
  a hook. Before, the list named hook classes that do not exist (`\core\hook\output\before_footer`
  instead of `before_footer_html_generation`, among others), flagged callbacks that no hook replaces
  (`extend_navigation`, `extend_signup_form`, `cron`, `course_module_viewed`, ...), and missed real
  ones (`add_htmlattributes`, `standard_footer_html`, `before_standard_top_of_body_html`,
  `after_file_created`, ...). This affects the legacy-callback sections of the generated context
  files and of the `upgrade_plugin` prompt.
- Switching to another Moodle root no longer recreates a previous root that was deleted or moved
  (as `<root>/.build82/.cache.json`) just to save its pending cache marks.
- The plugin type is inferred from the longest matching type directory, so question types,
  gradebook reports, SCORM reports and user profile fields (among others) are no longer reported
  as `calendartype`, `report` or `datafield`.
- The tool input schemas declare the allowed values of `format`, `watch_plugins` `action`,
  `plugin_batch` `mode` and `search_api` `visibility` as enums, so a misspelled value is rejected
  instead of silently falling back to text output or to `all`.
- `init_moodle_context` stores a relative `moodle_path` as an absolute path, and with `force`
  warns when `BUILD82_MOODLE_PATH` keeps overriding the saved file.
- `update_indexes` no longer writes `~/.build82` when the configuration comes from
  `BUILD82_MOODLE_PATH`.
- `build82 install` keeps the keys and extra environment variables a user added to an existing
  `build82` entry (including OpenCode's `environment`), and leaves the file untouched when the
  entry already matches.
- `build82 install` and `uninstall` give each `claude`/`codex` command 60 s to finish instead of
  waiting forever; on Windows they refuse paths with batch metacharacters when that CLI is a
  `.cmd`/`.bat` script; and a failed re-registration after the old one was removed says so.
- `build82 uninstall` reports a failing `claude mcp get` as a failure instead of "not registered",
  and an interrupted `--purge` confirmation no longer hides registrations already removed.
- A relative `CLINE_DATA_DIR` is ignored, as `CLINE_DIR` already was.
- When `self-update --rollback` cannot restore the current binary, the error says where the backup
  now is and how to put it back.
- Generated files no longer stay outdated after these changes, which were missed because no
  remaining source had a newer modification time:
  - a plugin was removed, or unpacked with its original (old) timestamps — the global indexes now
    regenerate when their set of sources changes;
  - a class was added — `MOODLE_CLASSES_INDEX.md` now depends on every `classes/**/*.php` file, and
    each plugin's files depend on every file of the plugin (classes, AMD modules, any PHP file), not
    only `version.php`, `lib.php` and `db/*`;
  - a source was saved while its context was being generated — the cache now records when the
    sources were read, not when the output was written.

  The cache file format changed (`.build82/.cache.json` version 2); the previous file is ignored, so
  the first run after updating regenerates every file once.
- `moodle://plugins/with-context`, read right after a tool or the watcher announced it as updated,
  lists the newly generated plugin instead of the listing cached up to 5 seconds before.
- `watch_plugins` regenerates a plugin when one of its watched files is deleted, not only when one
  is saved, created or renamed.
- With sessions on two Moodle installations at the same time, the cache no longer saves one
  installation's freshness marks into the other's cache file.
- The tree-sitter backend no longer takes tens of seconds on a large `lib.php` (a 1.1 MB file went
  from about 35 s to under a second): child nodes are read in a single pass, and the top-level
  functions are collected once instead of once per legacy-callback name.
- The tree-sitter backend parses files of at most 2 MiB (was 8 MiB) and stops a parse after 10
  seconds; a parse stopped early is skipped as an error instead of yielding a partial result
  missing declarations. This bounds its memory use, which reached gigabytes on a 7 MiB file.
- `doctor`'s deprecated-API, capability-usage and language-string checks, and `db/upgrade.php`
  parsing, compute line numbers in linear time; a file with many matches no longer takes seconds.
- Class extraction no longer slows down quadratically on a long heredoc.
- A plugin whose 12 context files are all fresh is no longer parsed: `generate_plugin_context`,
  `plugin_batch` and `update_indexes` check the cache before running the extractors.
- `init_moodle_context` and `update_indexes` walk the Moodle tree once for all the global indexes
  instead of once per index.
- `doctor` detects each development plugin once and lists its PHP files once for all its checks;
  `plugin_batch` and `update_indexes` no longer detect or scan plugins a second time.
- `plugin_batch` in `list` mode processes a plugin once when several entries name it (repeated, or
  as a component and as a path); before, it was regenerated twice, concurrently.
- The fuzzy fallback of `search_api` and `search_plugins` skips words whose length rules out a
  match, so a long query on a large index is answered in milliseconds.
- The `review_plugin` prompt checks for `phpcs` at most once a minute instead of on every render.
- The callback index reads `lib.php` and `locallib.php` with the same 8 MiB limit as the
  extractors, and a file that grows past the limit while being read is refused.
- Upgrade steps (regex backend) ignore commented-out `if ($oldversion < N)` blocks, and a quote
  inside a heredoc no longer hides every step after it, as the tree-sitter backend and `doctor`'s
  upgrade check already did.
- `doctor`'s Deprecated Core API Usage no longer reports function declarations, class
  instantiations, variable calls or namespaced functions that share a deprecated core name.
- `version.php` is read the same way by both backends: commented-out assignments are ignored, the
  last assignment of a field wins, and a decimal version such as `2024010100.01` keeps its decimal
  part (the regex backend truncated it, the tree-sitter backend returned nothing).
- Legacy callbacks declared inside an `if (!function_exists(...))` guard are detected by both
  backends and by the callback index; a commented-out declaration is not.
- `PLUGIN_CALLBACK_INDEX.md` names the Hook API callback that replaces every legacy callback it
  lists, including those under a module's short name, which were shown as having no replacement.
  Its main table lists exactly the legacy callbacks the other context files report: those declared
  in `lib.php` (the file Moodle loads to call them); a declaration inside a block comment is no
  longer listed. Legacy callbacks declared in `locallib.php` are listed in a separate "never called"
  section, with the advice to migrate them to the Hook API or move them to `lib.php`.
- Legacy callbacks are matched case-insensitively, as PHP resolves function names, on both
  backends (`function Local_X_Before_Footer()` was missed).
- Hook classes declared `readonly`, `final readonly` or `abstract` are read by the regex backend;
  they were skipped.
- Upgrade steps whose version has a decimal part (`if ($oldversion < 2024010100.01)`) are reported
  by both backends and ordered numerically; `doctor`'s upgrade check ignores a `$oldversion`
  comparison with a number that is not a ten-digit version, as the other readers already did.
- With the tree-sitter backend, a boolean field (`ajax`, `internal`, `blocking`,
  `defaultenabled`) set to an integer other than 1 or 0 keeps its default, as with the regex
  backend, instead of reading as false.
- `doctor` names a development plugin it cannot read by its path relative to the Moodle root
  instead of an absolute host path, and its legacy-files line names the tools that move each kind
  of file: `init_moodle_context`/`update_indexes` for global files, `generate_plugin_context`/
  `plugin_batch` (or the watcher) for plugin files.
- The cache line of the `update_indexes` and `plugin_batch` reports (`Cache (this call): …`) counts
  that call's lookups; it showed the running total since the server started.
- `release_plugin` ZIP entries keep each file's modification time and permission bits (an
  executable CLI script stays executable); they were dated 1980 with default permissions.
- `watch_plugins` and the `moodle://plugins/with-context` listing find plugins with the same walk as
  the other tools: a `.indevelopment` marker or context file that is a symbolic link, or inside
  `.git/`, is ignored.
- The `.build82/tags` file records paths relative to itself instead of absolute host paths, does
  not follow symbolic links out of the tree, skips `.build82`, and a ctags run longer than 10
  minutes is stopped and reported.
- Context files cut at a size limit (the plugin's AI context in `explain_plugin`, and the files
  embedded in the `review_plugin` and `scaffold_plugin` prompts) no longer end with a split UTF-8
  character.

### Development

- The CI coverage floor is 89.0%. It is measured by the Linux regex-backend run of the `test` job,
  so the Linux suite no longer runs a separate time for coverage.
- Benchmarks and time-bounded tests cover a 1.1 MB `lib.php` on the tree-sitter backend, many
  matches in one file, a long heredoc and a long fuzzy-search query.
- The tests that run against real Moodle installations read them from `BUILD82_REAL_MOODLE_DIR`
  (default `/srv/workspace/localhost/html`) and are skipped when it is missing.
- Parsing logic copied between the backends and the generators is shared: the legacy-callback
  warnings and their guidance (`legacyhooks.Warnings`), the class-declaration and namespace patterns,
  the upgrade-step locator and version rule, comment truncation (`phpdoc.CommentSummary`) and the
  symlink-skipping file walk (`fsutil.WalkFiles`). The per-plugin generators always receive the
  extracted data; their per-generator extraction fallback is gone.
- The real-file class parity test fails when the backends find different classes. New tests cover
  a parallel `plugin_batch` with duplicate identifiers and the Windows rename retry.
- The real-client smoke test (`scripts/smoke/claude.sh`) also covers the upgrade and Privacy API
  checks, the `upgrade_plugin` prompt, `check_coding_style` (when `phpcs` with the moodle standard
  is on `PATH`) and `bump_plugin_version`. Answers that could match by accident are asked as fixed
  `key=value` lines and matched exactly, a failed `claude` call fails its question, and the MCP
  configuration is built with `jq`.
- CI and the release workflow run the same checks from one reusable workflow (`checks.yml`): the
  release now also runs the cross-OS vet, `go mod tidy -diff`, actionlint, the link checker, the
  coverage floor and the TMPDIR-behind-a-symlink run. Packaging and metadata validation are shared
  composite actions; the release build uploads its artifact before any third-party validator runs.
- The bundle's `manifest.json` is validated with `@anthropic-ai/mcpb` 2.1.2 in CI and in the release.
- The documentation link checker also checks HTML `src`/`href` attributes.
- The release workflow verifies the signature with the `oito2/mcp-build82` identity `self-update`
  requires, written out instead of derived from the repository running it, and the signing job runs
  in a `release` environment.

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

[Unreleased]: https://github.com/oito2/mcp-build82/compare/v1.3.0...HEAD
[1.3.0]: https://github.com/oito2/mcp-build82/compare/v1.2.0...v1.3.0
[1.2.0]: https://github.com/oito2/mcp-build82/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/oito2/mcp-build82/compare/v1.0.1...v1.1.0
[1.0.1]: https://github.com/oito2/mcp-build82/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/oito2/mcp-build82/releases/tag/v1.0.0
