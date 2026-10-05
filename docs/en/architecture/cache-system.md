🌐 [Português](../../pt-br/architecture/cache-system.md) | **English** | 🏠 [Index](../index.md)

---

# Cache System

`build82` uses a cache system based on **mtime** (file modification time) to avoid regenerating context files unnecessarily. Without caching, every call to `update_indexes` or `plugin_batch` would reprocess every PHP and XML file in the installation — regardless of whether they changed or not.

build82's cache is **persisted to disk**, at `{moodle_root}/.build82/.cache.json`, so a server restart (a real event for a CLI tool that MCP clients launch fresh for each session) doesn't force a full re-scan.

---

## How it works

The cache compares the modification date (`mtime`) of each PHP/XML source file with the corresponding context `.md` file:

```
PHP/XML file (source)          .md file (output)
│                                │
│  mtime: 2024-04-20 10:00       │  mtime: 2024-04-21 09:00
│                                │
└───────────── comparison ───────┘
              │
    source older than output?
     │                  │
    YES                 NO
     │                  │
    SKIP             REGENERATE
 (Skipped: true)      (writes .md)
```

**Three possible results per file, tracked as running totals for the process:**

| Result | Condition | Counted as |
| --- | --- | --- |
| **Hit** | Output was marked fresh this session/persisted, and no source changed since | `Hits` |
| **Skip** | Output exists and every source is older than it (mtime comparison, no prior mark) | `Skips` |
| **Miss** | Output was explicitly invalidated, doesn't exist, or some source is newer | `Misses` |

---

## Implementation

The cache is implemented as `MtimeCache` in `internal/cache/cache.go`. A single shared, mutex-guarded instance — `cache.Global` — is used by every generator orchestrator in the process.

```go
type MtimeCache struct {
	mu     sync.Mutex
	marked map[string]time.Time // outputPath -> when it was marked fresh
	stats  CacheStats           // in-memory only, always resets on restart

	// Outputs explicitly invalidated (force: true, watcher events): reported stale
	// unconditionally until regenerated and Mark'd again. In memory only, keyed by absolute
	// output path, and left untouched by EnsureLoaded.
	forced map[string]struct{}

	loadedRoot string // moodlePath this cache is currently bound to
	dirty      bool   // true if marked/loadedRoot changed since the last Save
}
```

On disk the file is JSON with a `version` field (currently `1`) and an `entries` map of output path → mark time. Generators call `IsStale` through `genutil.RunCached` and `genutil.Write` calls `Mark` after a successful write.

### `IsStale()` logic

The regeneration decision follows this exact sequence:

```
0. Was the output explicitly invalidated (Invalidate / InvalidateAll) and not
   regenerated since?
   YES → miss (regenerate), whatever the mtimes say
   NO  → continue

1. Does the output .md file exist?
   NO  → miss (regenerate)
   YES → continue

2. Was the output marked fresh (this session, or loaded from the persisted
   .cache.json)?
   YES → compare every source file's mtime against the mark's timestamp.
         Nothing changed since the mark → hit (skip).
         Something did change → delete the mark, fall through to step 3.
   NO  → continue to step 3

3. Is any source file newer than the output file's own mtime?
   YES → miss (regenerate)
   NO  → skip (skip)
```

### Monitored source files

**For global files** (`cache.GetMoodleSourceFiles`, used by the API/classes/dev-rules/plugin-guide/ctags outputs; the other global outputs are gated by globs of `db/events.php`, `db/tasks.php`, `db/services.php`, `db/install.xml`, `db/access.php` or by every plugin's `version.php`):

```
{moodlePath}/version.php
{moodlePath}/lib/moodlelib.php
{moodlePath}/lib/accesslib.php
```

**For plugin files** (`cache.GetPluginSourceFiles`):

```
version.php
lib.php
locallib.php
settings.php
db/install.xml
db/access.php
db/events.php
db/tasks.php
db/services.php
db/upgrade.php
db/hooks.php
db/subplugins.json
db/subplugins.php
```

The same list (`cache.PluginSourceFileNames`) is what the file watcher monitors.

A source file that doesn't exist is simply ignored in the comparison — it never causes an error.

---

## Persistence: `EnsureLoaded` / `Save`

- **`EnsureLoaded(moodlePath)`** binds the cache to a Moodle root and loads `.build82/.cache.json`, but only the first time it's called for that exact path in the process — a no-op on every subsequent call. Binding to a different root first saves any unsaved marks for the previous one, and resets the in-memory statistics. A missing, corrupt, or version-mismatched cache file degrades to an empty cache; this is never a fatal error.
- **`Save()`** persists the current state via an atomic write (`fsutil.WriteAtomic` — write to a temp file, then rename), but only if something actually changed since the last load or save (`dirty`). A crash mid-write can never leave a half-written cache file behind.

**Batch operations load and save once per batch, not once per plugin.** `plugin_batch` and `update_indexes`'s `include_plugins` path call `EnsureLoaded` before their loop and `Save` once after it, delegating to `GenerateAllForPluginCore` (the cache-bracket-free half of `GenerateAllForPlugin`) inside the loop itself. Calling the full `Save()` — which re-serializes the *entire* marked-files map — once per plugin in a large batch would otherwise serialize the I/O across a `parallel` worker pool and erase most of its benefit.

---

## Cache invalidation

### Forced invalidation

`Invalidate(outputFile)` drops the file's mark **and** adds it to the in-memory `forced` set. While an output is in that set, `IsStale` reports it stale (step 0 above) regardless of any mtime comparison, so the next generator run rewrites it. `Mark`, called after the regeneration writes the file, removes it from the set. The set is never persisted and `EnsureLoaded` does not clear it, so an invalidation issued before the cache is bound (or re-bound) to a Moodle root still applies once it loads. `InvalidateAll()` does the same for every output currently recorded in the cache (call `EnsureLoaded` first).

### By `force=true`

- `plugin_batch force=true` calls `Invalidate` for each of the 12 plugin output files of every plugin before generating it.
- `update_indexes force=true` (`forceGlobalRegeneration` in `internal/tools/update.go`) calls `EnsureLoaded`, then `Invalidate` for the 13 global Markdown files and `.build82/tags`, before regenerating the global files. With `include_plugins`, it also invalidates each dev plugin's 12 output files.

The result is a real regeneration: forced outputs are rewritten even when every source file is older than the output.

### Per individual file

`cache.Global.Invalidate(outputFile)` on a single path — used by the file watcher (see below) and by `force` on a specific plugin's file list.

---

## Cache statistics

`cache.Global.Stats()` accumulates hit/miss/skip counts for the lifetime of the process (in-memory only — this part doesn't persist, unlike the marks themselves). The `doctor` tool reports it:

```
## Cache

  Hits: 42, Misses: 7, Skips: 18
  Cache file: /var/www/moodle/.build82/.cache.json (3421 bytes)
```

**How to interpret:**

- **Many hits:** watch mode is active, or the same tool was called repeatedly in this session
- **Many skips:** the installation is stable — few PHP files have changed since the persisted cache was last written
- **Many misses:** first run against this installation, or many files changed (e.g. a Moodle upgrade)

---

## Interaction with watch mode

`internal/watcher` uses `fsnotify` to monitor the plugin source files listed above (those that exist) in every `.indevelopment`-marked plugin, up to 20 plugins. When a matching file is saved:

1. The watcher's 500 ms debounce timer fires after the change settles
2. It calls `cache.Global.Invalidate(...)` for each of the plugin's 12 output files, which forces their regeneration regardless of modification times
3. It runs `GenerateAllForPlugin` for that plugin, then refreshes `MOODLE_AI_INDEX.md`

Watch mode itself is in-memory only and does not survive a server restart — it has to be started again with `watch_plugins action="start"` each session.

---

## See also

- [Extractors](./extractors.md) — the packages whose results the cache avoids recomputing
- [Generators](./generators.md) — where `IsStale()` and the write-then-mark step happen
- [Tools Reference](../reference/tools.md) — `update_indexes`/`plugin_batch` (`force`) and `doctor` (stats)

---

[🏠 Back to Index](../index.md)
