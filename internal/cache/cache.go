// Copyright (C) 2026  OITO2
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
// file. It equals genutil.ContextDir; the literal is duplicated because genutil imports this
// package, so importing it here would create an import cycle.
const ContextDirName = ".build82"

// CacheStats counts IsStale outcomes since the cache was last bound to a Moodle root: Hits are
// outputs confirmed fresh through their recorded mark, Misses are outputs reported stale, and
// Skips are outputs found fresh by comparing file mtimes.
type CacheStats struct {
	Hits, Misses, Skips int
}

// MtimeCache tracks which generated output files are known-fresh relative to their source files,
// persisting that state to disk so a server restart doesn't force a full re-scan.
type MtimeCache struct {
	mu     sync.Mutex
	marked map[string]time.Time // outputPath -> when it was marked fresh (persisted across restarts)
	stats  CacheStats           // in-memory only, always resets on restart

	// forced holds outputs explicitly invalidated (for example by a forced run or a watcher
	// event). A forced output is reported stale regardless of any mtime comparison until it is
	// marked again. Keys are absolute output paths, so the set is naturally scoped per Moodle
	// root. EnsureLoaded leaves it untouched so that an invalidation issued before the cache is
	// bound to a root still applies afterwards. It is never persisted.
	forced map[string]struct{}

	loadedRoot string // moodlePath this cache is currently bound to; "" until EnsureLoaded succeeds
	dirty      bool   // true if marked/loadedRoot changed since the last Save
}

// NewMtimeCache returns an empty cache that is not yet bound to any Moodle root.
func NewMtimeCache() *MtimeCache {
	return &MtimeCache{marked: map[string]time.Time{}, forced: map[string]struct{}{}}
}

// Global is the process-wide cache instance shared by every generator orchestrator.
var Global = NewMtimeCache()

// IsStale reports whether `outputFile` must be regenerated, given the `sourceFiles` it is derived
// from. Source files that cannot be stat'ed are ignored. It updates the hit, miss and skip
// counters. The decision steps are:
//  0. The output was invalidated (Invalidate/InvalidateAll) and not marked since: stale (miss).
//  1. The output file does not exist: stale (miss).
//  2. The output has a mark: compare the source mtimes with the mark time, not with the output's
//     mtime. If no source is newer, it is fresh (hit). Otherwise the mark is dropped and the
//     check continues with step 3.
//  3. Compare every source mtime with the output's mtime. If any source is newer, it is stale
//     (miss); otherwise it is fresh (skip).
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

// Mark records `outputFile` as freshly generated at the current time and clears any pending
// invalidation for it. The mark is persisted by the next Save.
func (c *MtimeCache) Mark(outputFile string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.marked[outputFile] = time.Now()
	delete(c.forced, outputFile)
	c.dirty = true
}

// Invalidate makes IsStale report `outputFile` as stale until it is marked again, even when its
// mtime is newer than every source file, and drops its recorded mark. It may be called before
// EnsureLoaded, because loading persisted marks does not cancel a pending invalidation.
func (c *MtimeCache) Invalidate(outputFile string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.marked[outputFile]; ok {
		delete(c.marked, outputFile)
		c.dirty = true
	}
	c.forced[outputFile] = struct{}{}
}

// InvalidateAll invalidates, as Invalidate does for one path, every output that currently has a
// recorded mark. It cannot affect outputs without a mark, so call EnsureLoaded first. Callers that
// know their output paths should use Invalidate per path, which covers unmarked outputs too.
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

// Stats returns a copy of the hit, miss and skip counters. They are kept in memory only and reset
// when the cache is bound to a different Moodle root.
func (c *MtimeCache) Stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

// cacheFileVersion is the on-disk format version; files with another version are ignored.
const cacheFileVersion = 1

// cacheFile is the JSON document persisted to .cache.json: the format version and the mark time
// of each output path.
type cacheFile struct {
	Version int                  `json:"version"`
	Entries map[string]time.Time `json:"entries"`
}

// cachePath returns the location of the cache file for the Moodle root `moodlePath`.
func cachePath(moodlePath string) string {
	return filepath.Join(moodlePath, ContextDirName, ".cache.json")
}

// EnsureLoaded binds the cache to the Moodle root `moodlePath` and loads its persisted marks from
// disk. It does nothing when the cache is already bound to that exact path, so it can be called at
// the start of every generator run. A missing, corrupt or version-mismatched cache file yields an
// empty cache, and an unreadable one additionally prints a warning to stderr; it never fails.
// Switching roots resets the statistics.
//
// When the cache is bound to a different root and has unsaved marks, they are first saved to that
// previous root's own cache file, so sessions using different Moodle installations never lose
// marks or write them into the wrong root. A save error there is ignored, because EnsureLoaded
// has no error return and refusing to switch would be worse than losing those marks.
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

	// A missing file is not an error (ok is false). Other read failures only produce a warning
	// and leave the cache empty.
	path := cachePath(moodlePath)
	content, ok, err := fsutil.ReadOptional(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build82: warning: could not read cache file %s: %v\n", path, err)
	}
	marked := map[string]time.Time{}
	if ok {
		var cf cacheFile
		if jsonErr := json.Unmarshal(content, &cf); jsonErr == nil && cf.Version == cacheFileVersion {
			if cf.Entries != nil {
				marked = cf.Entries
			}
		}
	}

	c.loadedRoot = moodlePath
	c.marked = marked
	c.dirty = false
	c.stats = CacheStats{}
}

// Save writes the marks to the bound root's cache file when they changed since the last load or
// save, and does nothing when no root is bound or nothing changed. It returns an error when
// marshaling or the atomic file write fails. Call it at the end of every generator run.
func (c *MtimeCache) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.saveLocked()
}

// saveLocked implements Save for callers that already hold c.mu, such as EnsureLoaded.
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

// PluginSourceFileNames lists the plugin-relative file names whose mtimes decide whether a
// plugin's generated files are stale (through GetPluginSourceFiles) and that the watcher package
// monitors for live regeneration.
var PluginSourceFileNames = []string{
	"version.php", "lib.php", "locallib.php", "settings.php",
	"db/install.xml", "db/access.php", "db/events.php",
	"db/tasks.php", "db/services.php", "db/upgrade.php", "db/hooks.php",
	"db/subplugins.json", "db/subplugins.php",
}

// GetPluginSourceFiles returns PluginSourceFileNames joined with the plugin directory
// `pluginPath`. It does not check that the files exist, because IsStale ignores missing sources.
func GetPluginSourceFiles(pluginPath string) []string {
	files := make([]string, len(PluginSourceFileNames))
	for i, n := range PluginSourceFileNames {
		files[i] = filepath.Join(pluginPath, n)
	}
	return files
}

// GetMoodleSourceFiles returns the version.php, lib/moodlelib.php and lib/accesslib.php paths
// under the Moodle root `moodlePath`, which are the staleness signal for the site-wide generators.
func GetMoodleSourceFiles(moodlePath string) []string {
	names := []string{"version.php", "lib/moodlelib.php", "lib/accesslib.php"}
	files := make([]string, len(names))
	for i, n := range names {
		files[i] = filepath.Join(moodlePath, n)
	}
	return files
}
