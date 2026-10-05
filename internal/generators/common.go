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

var skipWalkDirNames = map[string]bool{"vendor": true, "node_modules": true, ".git": true}

// walkMoodleFiles walks root recursively, skipping vendor/, node_modules/, and .git/ at every
// depth, and calls visit for every non-directory entry found with its full path and its
// root-relative path in slash form. Shared by globMoodleSuffix/globMoodleClassesPhp/
// findMarkerFiles, which differ only in their match predicate.
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
		// Skip symlinked files: every visit callback
		// here eventually reads the file's content, which would otherwise follow the symlink to
		// wherever it points, potentially pulling content from outside the scanned Moodle tree into
		// a generated index.
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

// globMoodleSuffix returns every file whose root-relative path (in slash form) ends with suffix.
func globMoodleSuffix(root, suffix string) []string {
	var matches []string
	walkMoodleFiles(root, func(path, relSlash string) {
		if strings.HasSuffix(relSlash, suffix) {
			matches = append(matches, path)
		}
	})
	return matches
}

// globMoodleBasename returns every file whose base name (the last path segment) is exactly name —
// unlike globMoodleSuffix, which matches on raw string suffix and so also matches any file merely
// ending in name (e.g. "notajax.php" for name="ajax.php"), even when that's not a real filename
// boundary. Used wherever the convention being matched is "a file literally named X", not "a file
// whose name ends with X".
func globMoodleBasename(root, name string) []string {
	var matches []string
	walkMoodleFiles(root, func(path, relSlash string) {
		if filepath.Base(path) == name {
			matches = append(matches, path)
		}
	})
	return matches
}

// globMoodleClassesPhp returns every .php file under root that lives inside a "classes/"
// directory at any depth — the restricted glob GenerateClassesIndex uses instead of scanning the
// whole installation.
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

// findMarkerFiles finds every {ContextDir}/.indevelopment marker under root.
func findMarkerFiles(root string) []string {
	var matches []string
	walkMoodleFiles(root, func(path, _ string) {
		if filepath.Base(path) == ".indevelopment" && filepath.Base(filepath.Dir(path)) == ContextDir {
			matches = append(matches, path)
		}
	})
	return matches
}

// FindDevPlugins returns every plugin directory marked .indevelopment (unsorted — sorting is each
// caller's own responsibility).
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

// FindPluginDirs returns every plugin directory found under any of moodletype.PluginTypeToDir's
// values (unsorted — sorting is each caller's own responsibility).
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
// doesn't exist (e.g. a plugin with no db/tasks.php) — a real, legitimate case, not an error. Every
// generator that reads these fields must go through these helpers rather than dereferencing the
// pointer directly, whether the data came from PreloadedPluginData or a fresh extractor call.
func eventsOf(e *extractors.EventsExtraction) []extractors.EventObserver {
	if e == nil {
		return nil
	}
	return e.Observers
}

func tasksOf(e *extractors.TasksExtraction) []extractors.ScheduledTask {
	if e == nil {
		return nil
	}
	return e.Tasks
}

func servicesOf(e *extractors.ServicesExtraction) []extractors.WebServiceFunction {
	if e == nil {
		return nil
	}
	return e.Functions
}

func capsOf(e *extractors.CapabilitiesExtraction) []extractors.Capability {
	if e == nil {
		return nil
	}
	return e.Capabilities
}

func upgradeOf(e *extractors.UpgradeExtraction) []extractors.UpgradeStep {
	if e == nil {
		return nil
	}
	return e.Steps
}

// pluginInfoCache memoizes extractors.DetectPlugin across the several global generators that
// each need a plugin's identity within one GenerateAll call. Mutex-guarded because wave 1's
// generators access it concurrently.
type pluginInfoCache struct {
	mu sync.Mutex
	m  map[string]extractors.PluginInfo
}

func newPluginInfoCache() *pluginInfoCache {
	return &pluginInfoCache{m: map[string]extractors.PluginInfo{}}
}

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
