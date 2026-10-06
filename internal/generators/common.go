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

package generators

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/genutil"
	"github.com/oito2/mcp-build82/internal/moodletype"
)

// GeneratorResult is genutil.GeneratorResult, re-exported for convenience within this package.
type GeneratorResult = genutil.GeneratorResult

// skipWalkDirNames lists the directory names walkMoodleFiles never descends into.
var skipWalkDirNames = map[string]bool{"vendor": true, "node_modules": true, ".git": true}

// walkMoodleFiles walks `root` recursively, skipping vendor/, node_modules/, and .git/ at every
// depth, and calls `visit` for every non-directory, non-symlink entry with its full path and its
// root-relative slash-form path. Unreadable entries are silently skipped. It is the shared walker
// behind the glob/find helpers, which differ only in their match predicate.
func walkMoodleFiles(root string, visit func(path, relSlash string)) {
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && skipWalkDirNames[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip symlinked files: visit callbacks read file content, which would otherwise follow
		// the link and pull content from outside the scanned Moodle tree into a generated index.
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		visit(path, filepath.ToSlash(rel))
		return nil
	})
}

// globMoodleSuffix returns the full path of every file under `root` whose root-relative slash-form
// path ends with `suffix`.
func globMoodleSuffix(root, suffix string) []string {
	var matches []string
	walkMoodleFiles(root, func(path, relSlash string) {
		if strings.HasSuffix(relSlash, suffix) {
			matches = append(matches, path)
		}
	})
	return matches
}

// globMoodleBasename returns the full path of every file under `root` whose base name is exactly
// `name`. Unlike globMoodleSuffix, it does not match files that merely end with `name` (e.g.
// "notajax.php" for "ajax.php").
func globMoodleBasename(root, name string) []string {
	var matches []string
	walkMoodleFiles(root, func(path, relSlash string) {
		if filepath.Base(path) == name {
			matches = append(matches, path)
		}
	})
	return matches
}

// globMoodleClassesPhp returns the full path of every .php file under `root` that lives inside a
// "classes/" directory at any depth. GenerateClassesIndex uses it to avoid scanning every file.
func globMoodleClassesPhp(root string) []string {
	var matches []string
	walkMoodleFiles(root, func(path, relSlash string) {
		if !strings.HasSuffix(path, ".php") {
			return
		}
		if strings.Contains(relSlash, "/classes/") || strings.HasPrefix(relSlash, "classes/") {
			matches = append(matches, path)
		}
	})
	return matches
}

// findMarkerFiles returns the full path of every ContextDir/.indevelopment marker file under `root`.
func findMarkerFiles(root string) []string {
	var matches []string
	walkMoodleFiles(root, func(path, _ string) {
		if filepath.Base(path) == ".indevelopment" && filepath.Base(filepath.Dir(path)) == ContextDir {
			matches = append(matches, path)
		}
	})
	return matches
}

// FindDevPlugins returns the root directory of every plugin under `moodlePath` that has a
// ContextDir/.indevelopment marker. The result is unsorted and free of duplicates.
func FindDevPlugins(moodlePath string) []string {
	markers := findMarkerFiles(moodlePath)
	seen := map[string]struct{}{}
	var dirs []string
	for _, m := range markers {
		d := filepath.Dir(filepath.Dir(m)) // grandparent: out of .build82/, to the plugin root
		if _, ok := seen[d]; !ok {
			seen[d] = struct{}{}
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// FindPluginDirs returns every plugin directory under `moodlePath` that contains a version.php
// directly inside a `<typeDir>/<name>/` folder, for each directory in moodletype.PluginTypeToDir.
// The result is unsorted and free of duplicates.
func FindPluginDirs(moodlePath string) []string {
	seen := map[string]struct{}{}
	var dirs []string
	for _, typeDir := range moodletype.PluginTypeToDir {
		matches, _ := filepath.Glob(filepath.Join(moodlePath, typeDir, "*", "version.php"))
		for _, m := range matches {
			d := filepath.Dir(m)
			if _, ok := seen[d]; !ok {
				seen[d] = struct{}{}
				dirs = append(dirs, d)
			}
		}
	}
	return dirs
}

// Nil-safe accessors: every ParseXxxPhp extractor returns a nil pointer when its source file
// does not exist (e.g. a plugin with no db/tasks.php), which is a legitimate case. Generators read
// these fields through the helpers below instead of dereferencing the pointer directly.

// eventsOf returns the observers of `e`, or nil when `e` is nil.
func eventsOf(e *extractors.EventsExtraction) []extractors.EventObserver {
	if e == nil {
		return nil
	}
	return e.Observers
}

// tasksOf returns the scheduled tasks of `e`, or nil when `e` is nil.
func tasksOf(e *extractors.TasksExtraction) []extractors.ScheduledTask {
	if e == nil {
		return nil
	}
	return e.Tasks
}

// servicesOf returns the web service functions of `e`, or nil when `e` is nil.
func servicesOf(e *extractors.ServicesExtraction) []extractors.WebServiceFunction {
	if e == nil {
		return nil
	}
	return e.Functions
}

// capsOf returns the capabilities of `e`, or nil when `e` is nil.
func capsOf(e *extractors.CapabilitiesExtraction) []extractors.Capability {
	if e == nil {
		return nil
	}
	return e.Capabilities
}

// pluginInfoCache memoizes extractors.DetectPlugin results by plugin path, so the global
// generators of one GenerateAll call detect each plugin once. It is safe for concurrent use.
type pluginInfoCache struct {
	mu sync.Mutex
	m  map[string]extractors.PluginInfo
}

// newPluginInfoCache returns an empty pluginInfoCache.
func newPluginInfoCache() *pluginInfoCache {
	return &pluginInfoCache{m: map[string]extractors.PluginInfo{}}
}

// get returns the PluginInfo for `pluginPath`, detecting and caching it on first use. Detection
// errors are ignored and the resulting (possibly zero) info is cached.
func (c *pluginInfoCache) get(pluginPath string) extractors.PluginInfo {
	c.mu.Lock()
	if info, ok := c.m[pluginPath]; ok {
		c.mu.Unlock()
		return info
	}
	c.mu.Unlock()

	info, _ := extractors.DetectPlugin(pluginPath)

	c.mu.Lock()
	c.m[pluginPath] = info
	c.mu.Unlock()
	return info
}
