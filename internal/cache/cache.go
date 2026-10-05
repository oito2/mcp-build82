// Copyright (C) 2026  oito2
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oito2/mcp-build82/internal/fsutil"
)

// ContextDirName is the name of the per-Moodle-root directory (".build82") that holds the cache
// file. It mirrors genutil.ContextDir's value; the literal is duplicated because genutil depends on
// cache, so importing genutil here would create an import cycle.
const ContextDirName = ".build82"

type CacheStats struct {
	Hits, Misses, Skips int
}

// MtimeCache tracks which generated output files are known-fresh relative to their source files,
// persisting that state to disk so a server restart doesn't force a full re-scan.
type MtimeCache struct {
	mu     sync.Mutex
	marked map[string]time.Time // outputPath -> when it was marked fresh (persisted across restarts)
	stats  CacheStats           // in-memory only, always resets on restart

	// forced holds outputs explicitly invalidated (e.g. by force: true or a watcher event). A
	// forced output is reported stale unconditionally, regardless of any mtime comparison, until
	// it is regenerated and Mark'd again. Keys are absolute output paths, so the set is naturally
	// scoped per Moodle root and is deliberately left untouched by EnsureLoaded: an invalidation
	// issued before the cache is bound (or re-bound) to a root must still apply once it loads.
	// In-memory only, never persisted.
	forced map[string]struct{}

	loadedRoot string // moodlePath this cache is currently bound to; "" until EnsureLoaded succeeds
	dirty      bool   // true if marked/loadedRoot changed since the last Save
}

func NewMtimeCache() *MtimeCache {
	return &MtimeCache{marked: map[string]time.Time{}, forced: map[string]struct{}{}}
}

// Global is the process-wide cache instance shared by every generator orchestrator.
var Global = NewMtimeCache()

// IsStale returns true when outputFile needs regeneration, comparing against the given source
// files. The decision steps are:
//  0. Output was explicitly invalidated (Invalidate/InvalidateAll) and not regenerated since ->
//     always stale (miss), whatever the mtimes say.
//  1. Output file doesn't exist -> always stale (miss).
//  2. Output was Mark'd fresh this session: compare source mtimes against the mark's timestamp
//     (not the output file's mtime). If nothing changed since the mark, it's a hit. If something
//     did change, delete the mark and fall through to step 3.
//  3. Compare every source file's mtime against the output file's mtime. Any source newer -> miss.
//     All sources older -> skip.
func (c *MtimeCache) IsStale(outputFile string, sourceFiles []string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.forced[outputFile]; ok {
		c.stats.Misses++
		return true
	}

	outInfo, err := os.Stat(outputFile)
	if err != nil {
		c.stats.Misses++
		return true
	}

	if markedAt, ok := c.marked[outputFile]; ok {
		changedAfterMark := false
		for _, src := range sourceFiles {
			if info, err := os.Stat(src); err == nil && info.ModTime().After(markedAt) {
				changedAfterMark = true
				break
			}
		}
		if !changedAfterMark {
			c.stats.Hits++
			return false
		}
		delete(c.marked, outputFile)
	}

	outMtime := outInfo.ModTime()
	for _, src := range sourceFiles {
		if info, err := os.Stat(src); err == nil && info.ModTime().After(outMtime) {
			c.stats.Misses++
			return true
		}
	}

	c.stats.Skips++
	return false
}

// Mark records outputFile as freshly generated now, clearing any pending forced invalidation.
func (c *MtimeCache) Mark(outputFile string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.marked[outputFile] = time.Now()
	delete(c.forced, outputFile)
	c.dirty = true
}

// Invalidate forces outputFile to be reported stale by IsStale until it is regenerated and Mark'd
// again, even when its mtime is newer than every source file. Safe to call before EnsureLoaded:
// loading persisted marks never cancels a pending invalidation.
func (c *MtimeCache) Invalidate(outputFile string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.marked[outputFile]; ok {
		delete(c.marked, outputFile)
		c.dirty = true
	}
	c.forced[outputFile] = struct{}{}
}

// InvalidateAll forces every output currently recorded in the cache to be reported stale until
// regenerated, exactly as Invalidate does for a single path. It only knows the outputs present in
// the loaded map, so call EnsureLoaded first; callers that know their output paths should prefer
// Invalidate per path, which also covers outputs that have no recorded mark at all.
func (c *MtimeCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.marked) > 0 {
		for out := range c.marked {
			c.forced[out] = struct{}{}
		}
		c.marked = map[string]time.Time{}
		c.dirty = true
	}
}

func (c *MtimeCache) Stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

const cacheFileVersion = 1

type cacheFile struct {
	Version int                  `json:"version"`
	Entries map[string]time.Time `json:"entries"`
}

func cachePath(moodlePath string) string {
	return filepath.Join(moodlePath, ContextDirName, ".cache.json")
}

// EnsureLoaded binds the cache to moodlePath and loads persisted state from disk, if not already
// bound to that exact path. Idempotent — safe to call at the top of every orchestrator invocation.
// A corrupt, missing, or version-mismatched cache file degrades to an empty cache, never a fatal
// error.
//
// If the cache is currently bound to a different root and holds unsaved marks (c.dirty), those
// marks are persisted to that previous root's own cache file before switching, so concurrent use
// against different Moodle installations (e.g. --http serving multiple sessions) never loses marks
// or writes them into the wrong root's .cache.json. A save error here is swallowed: EnsureLoaded
// has no error return, and refusing to switch roots over a save failure would be worse than losing
// the previous root's in-memory marks.
//
// The locking only serializes goroutines within one process. Separate build82 processes pointed at
// the same moodlePath can race on .cache.json and the last writer wins. The file is always written
// whole (fsutil.WriteAtomic) and the cache is only a staleness optimization, so the worst case is
// an unnecessary regeneration.
func (c *MtimeCache) EnsureLoaded(moodlePath string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.loadedRoot == moodlePath {
		return
	}

	if c.dirty && c.loadedRoot != "" {
		_ = c.saveLocked()
	}

	// Any read error (permission denied, etc.) degrades to an empty cache. A genuine read failure
	// (as opposed to the file not existing, which ReadOptional reports as ok==false, err==nil) is
	// reported with a stderr warning and does not block the operation.
	path := cachePath(moodlePath)
	content, ok, err := fsutil.ReadOptional(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build82: warning: could not read cache file %s: %v\n", path, err)
	}
	marked := map[string]time.Time{}
	if ok {
		var cf cacheFile
		if jsonErr := json.Unmarshal(content, &cf); jsonErr == nil && cf.Version == cacheFileVersion {
			marked = cf.Entries
		}
	}

	c.loadedRoot = moodlePath
	c.marked = marked
	c.dirty = false
	c.stats = CacheStats{}
}

// Save persists the current staleness map to disk, if anything changed since the last Load/Save.
// Call once at the end of every orchestrator invocation.
func (c *MtimeCache) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.saveLocked()
}

// saveLocked is Save's body, factored out so EnsureLoaded can persist the previous root's state
// while already holding c.mu (Save itself always acquires the lock, so it cannot call this other
// than through Save — this split exists solely so EnsureLoaded doesn't need to re-lock or unlock
// mid-method).
func (c *MtimeCache) saveLocked() error {
	if c.loadedRoot == "" || !c.dirty {
		return nil
	}

	dest := cachePath(c.loadedRoot)
	b, err := json.MarshalIndent(cacheFile{Version: cacheFileVersion, Entries: c.marked}, "", "  ")
	if err != nil {
		return err
	}
	if err := fsutil.WriteAtomic(dest, b, 0o644); err != nil {
		return err
	}
	c.dirty = false
	return nil
}

// PluginSourceFileNames is the fixed set of plugin-relative file names whose mtimes gate per-plugin
// generator staleness (via GetPluginSourceFiles) and that internal/watcher watches for live
// regeneration.
var PluginSourceFileNames = []string{
	"version.php", "lib.php", "locallib.php", "settings.php",
	"db/install.xml", "db/access.php", "db/events.php",
	"db/tasks.php", "db/services.php", "db/upgrade.php", "db/hooks.php",
	"db/subplugins.json", "db/subplugins.php",
}

// GetPluginSourceFiles returns PluginSourceFileNames joined with pluginPath. Existence is not
// checked here — callers rely on os.Stat failing gracefully inside IsStale's loop.
func GetPluginSourceFiles(pluginPath string) []string {
	files := make([]string, len(PluginSourceFileNames))
	for i, n := range PluginSourceFileNames {
		files[i] = filepath.Join(pluginPath, n)
	}
	return files
}

// GetMoodleSourceFiles returns the fixed set of source files used as the staleness signal for
// structural/summary generators.
func GetMoodleSourceFiles(moodlePath string) []string {
	names := []string{"version.php", "lib/moodlelib.php", "lib/accesslib.php"}
	files := make([]string, len(names))
	for i, n := range names {
		files[i] = filepath.Join(moodlePath, n)
	}
	return files
}
