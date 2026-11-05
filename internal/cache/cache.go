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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// Since returns the counts accumulated between `before` (an earlier Stats snapshot) and `s`.
func (s CacheStats) Since(before CacheStats) CacheStats {
	return CacheStats{Hits: s.Hits - before.Hits, Misses: s.Misses - before.Misses, Skips: s.Skips - before.Skips}
}

// MtimeCache tracks which generated output files are known-fresh relative to their source files,
// persisting that state to disk so a server restart doesn't force a full re-scan.
//
// Marks are kept per Moodle root: every root bound with EnsureLoaded keeps its own set of marks
// in memory, and a mark is stored under the bound root that contains the output path (the longest
// one when roots are nested). Outputs outside every bound root are kept in an in-memory set that is
// never persisted. This keeps a run on one root from writing its marks into another root's cache
// file when sessions on different roots run concurrently.
type MtimeCache struct {
	mu    sync.Mutex
	roots map[string]*rootMarks // bound Moodle root -> its marks; the "" key holds unpersisted marks
	stats CacheStats            // in-memory only, always resets on restart

	// forced holds outputs explicitly invalidated (for example by a forced run or a watcher
	// event). A forced output is reported stale regardless of any mtime comparison until it is
	// marked again. Keys are absolute output paths, so the set is naturally scoped per Moodle
	// root. EnsureLoaded leaves it untouched so that an invalidation issued before the cache is
	// bound to a root still applies afterwards. It is never persisted.
	forced map[string]struct{}

	loadedRoot string // moodlePath this cache is currently bound to; "" until EnsureLoaded succeeds
}

// mark records when an output was generated and which source set it was generated from.
type mark struct {
	// At is the time the sources were read for the generation that produced the output.
	At time.Time `json:"at"`
	// Set is the SourceSetHash of the sources the output was generated from, or "" when the mark
	// was recorded without a source set (Mark).
	Set string `json:"set,omitempty"`
}

// rootMarks holds the marks of one Moodle root and whether they changed since the last load or
// save.
type rootMarks struct {
	marked map[string]mark
	dirty  bool
}

// NewMtimeCache returns an empty cache that is not yet bound to any Moodle root.
func NewMtimeCache() *MtimeCache {
	return &MtimeCache{
		roots:  map[string]*rootMarks{"": {marked: map[string]mark{}}},
		forced: map[string]struct{}{},
	}
}

// Global is the process-wide cache instance shared by every generator orchestrator.
var Global = NewMtimeCache()

// SourceSetHash returns a hex SHA-256 digest of the set of paths in `sources`, independent of
// their order and of duplicates. Two calls return the same value only when the same paths are
// listed, so a source that was added, removed or renamed changes the digest.
func SourceSetHash(sources []string) string {
	sorted := slices.Clone(sources)
	slices.Sort(sorted)
	sorted = slices.Compact(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\x00")))
	return hex.EncodeToString(sum[:])
}

// rootForLocked returns the marks of the bound root that contains `outputFile` (the longest such
// root), or the unpersisted set when no bound root contains it. The caller must hold c.mu.
func (c *MtimeCache) rootForLocked(outputFile string) *rootMarks {
	best := ""
	for root := range c.roots {
		if root == "" || len(root) <= len(best) {
			continue
		}
		if strings.HasPrefix(outputFile, root+string(filepath.Separator)) {
			best = root
		}
	}
	return c.roots[best]
}

// IsStale reports whether `outputFile` must be regenerated, given the `sourceFiles` it is derived
// from. Source files that cannot be stat'ed are ignored. It updates the hit, miss and skip
// counters. The decision steps are:
//  0. The output was invalidated (Invalidate/InvalidateAll) and not marked since: stale (miss).
//  1. The output file does not exist, or is not a regular file (e.g. a symbolic link planted in its
//     place, whose target says nothing about the output): stale (miss).
//  2. The output has a mark recorded with a source set (MarkAt) and `sourceFiles` is a different
//     set — a source was added, removed or renamed, whatever its mtime: the mark is dropped and
//     the output is stale (miss).
//  3. The output has a mark: compare the source mtimes with the mark time, not with the output's
//     mtime. If no source is newer, it is fresh (hit). Otherwise the mark is dropped and the
//     output is stale (miss), even when the output was written after that source: the mark time
//     is when the sources were read, so the source was saved after the output's content was
//     computed.
//  4. Without a mark, compare every source mtime with the output's mtime. If any source is newer,
//     it is stale (miss); otherwise it is fresh (skip). Without a mark, a change to the set of
//     sources cannot be detected.
func (c *MtimeCache) IsStale(outputFile string, sourceFiles []string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.forced[outputFile]; ok {
		c.stats.Misses++
		return true
	}

	outInfo, err := os.Lstat(outputFile)
	if err != nil || !outInfo.Mode().IsRegular() {
		c.stats.Misses++
		return true
	}

	rm := c.rootForLocked(outputFile)
	if m, ok := rm.marked[outputFile]; ok {
		if m.Set != "" && m.Set != SourceSetHash(sourceFiles) {
			delete(rm.marked, outputFile)
			rm.dirty = true
			c.stats.Misses++
			return true
		}
		changedAfterMark := false
		for _, src := range sourceFiles {
			if info, err := os.Stat(src); err == nil && info.ModTime().After(m.At) {
				changedAfterMark = true
				break
			}
		}
		if !changedAfterMark {
			c.stats.Hits++
			return false
		}
		delete(rm.marked, outputFile)
		rm.dirty = true
		c.stats.Misses++
		return true
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

// Mark records `outputFile` as freshly generated at the current time, without a source set, and
// clears any pending invalidation for it. The mark is persisted by the next Save. Generators
// should use MarkAt, which also detects sources edited during the generation and changes to the
// set of sources.
func (c *MtimeCache) Mark(outputFile string) {
	c.setMark(outputFile, mark{At: time.Now()})
}

// MarkAt records `outputFile` as generated from `sourceFiles` as they were at `sourcesReadAt`, the
// time taken before the sources were read, and clears any pending invalidation for it. A source
// saved after that time, even while the output was being generated, makes the output stale again,
// and so does a later call to IsStale with a different set of sources. The mark is persisted by the
// next Save.
func (c *MtimeCache) MarkAt(outputFile string, sourcesReadAt time.Time, sourceFiles []string) {
	c.setMark(outputFile, mark{At: sourcesReadAt, Set: SourceSetHash(sourceFiles)})
}

// setMark stores `m` for `outputFile` under the root that contains it and clears its pending
// invalidation.
func (c *MtimeCache) setMark(outputFile string, m mark) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rm := c.rootForLocked(outputFile)
	rm.marked[outputFile] = m
	rm.dirty = true
	delete(c.forced, outputFile)
}

// Invalidate makes IsStale report `outputFile` as stale until it is marked again, even when its
// mtime is newer than every source file, and drops its recorded mark. It may be called before
// EnsureLoaded, because loading persisted marks does not cancel a pending invalidation.
func (c *MtimeCache) Invalidate(outputFile string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, rm := range c.roots {
		if _, ok := rm.marked[outputFile]; ok {
			delete(rm.marked, outputFile)
			rm.dirty = true
		}
	}
	c.forced[outputFile] = struct{}{}
}

// InvalidateAll invalidates, as Invalidate does for one path, every output that currently has a
// recorded mark in any bound root. It cannot affect outputs without a mark, so call EnsureLoaded
// first. Callers that know their output paths should use Invalidate per path, which covers
// unmarked outputs too.
func (c *MtimeCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, rm := range c.roots {
		if len(rm.marked) == 0 {
			continue
		}
		for out := range rm.marked {
			c.forced[out] = struct{}{}
		}
		rm.marked = map[string]mark{}
		rm.dirty = true
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
// Version 2 records the source set of each mark; version 1 files are ignored, so the first run
// after an upgrade regenerates every output once.
const cacheFileVersion = 2

// cacheFile is the JSON document persisted to .cache.json: the format version and the mark of
// each output path.
type cacheFile struct {
	Version int             `json:"version"`
	Entries map[string]mark `json:"entries"`
}

// cachePath returns the location of the cache file for the Moodle root `moodlePath`.
func cachePath(moodlePath string) string {
	return filepath.Join(moodlePath, ContextDirName, ".cache.json")
}

// EnsureLoaded binds the cache to the Moodle root `moodlePath`, loading its persisted marks from
// disk the first time the root is bound. It does nothing when the cache is already bound to that
// exact path, so it can be called at the start of every generator run. A missing, corrupt or
// version-mismatched cache file yields no marks, and an unreadable one additionally prints a
// warning to stderr; it never fails. Switching roots resets the statistics.
//
// Roots bound earlier keep their marks in memory, so a run still in flight on another root keeps
// marking into that root's own set. When the cache switches away from a root with unsaved marks,
// they are first saved to that root's own cache file; when that root no longer exists, its marks
// are dropped instead. A save error there is ignored, because EnsureLoaded has no error return and
// refusing to switch would be worse than losing those marks.
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

	if prev := c.loadedRoot; prev != "" {
		// A previous root that no longer exists (deleted or moved) is not recreated just to hold
		// its cache file.
		if !isDir(prev) {
			delete(c.roots, prev)
		} else if c.roots[prev].dirty {
			_ = c.saveRootLocked(prev)
		}
	}

	if _, ok := c.roots[moodlePath]; !ok {
		c.roots[moodlePath] = &rootMarks{marked: loadMarks(moodlePath)}
	}
	c.loadedRoot = moodlePath
	c.stats = CacheStats{}
}

// loadMarks reads the persisted marks of the Moodle root `moodlePath`. A missing file is not an
// error; other read failures print a warning to stderr. A missing, unreadable, corrupt or
// version-mismatched file yields an empty map.
func loadMarks(moodlePath string) map[string]mark {
	path := cachePath(moodlePath)
	content, ok, err := fsutil.ReadOptional(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build82: warning: could not read cache file %s: %v\n", path, err)
	}
	if ok {
		var cf cacheFile
		if jsonErr := json.Unmarshal(content, &cf); jsonErr == nil && cf.Version == cacheFileVersion && cf.Entries != nil {
			return cf.Entries
		}
	}
	return map[string]mark{}
}

// Save writes the marks of every bound root that changed since its last load or save to that
// root's cache file, and does nothing when nothing changed. Bound roots other than the current one
// that no longer exist are dropped instead of recreated. It returns the first error met: the cache
// file's directory is a symbolic link, or marshaling or the atomic file write failed. Call it at
// the end of every generator run.
func (c *MtimeCache) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var firstErr error
	for root, rm := range c.roots {
		if root == "" || !rm.dirty {
			continue
		}
		if root != c.loadedRoot && !isDir(root) {
			delete(c.roots, root)
			continue
		}
		if err := c.saveRootLocked(root); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// isDir reports whether `path` is an existing directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// saveRootLocked writes the marks of the bound root `root` to its cache file and clears its dirty
// flag. The caller must hold c.mu.
func (c *MtimeCache) saveRootLocked(root string) error {
	rm := c.roots[root]
	dest := cachePath(root)
	if err := fsutil.RefuseSymlinkedDir(filepath.Dir(dest)); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cacheFile{Version: cacheFileVersion, Entries: rm.marked}, "", "  ")
	if err != nil {
		return err
	}
	if err := fsutil.WriteAtomic(dest, b, 0o644); err != nil {
		return err
	}
	rm.dirty = false
	return nil
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
