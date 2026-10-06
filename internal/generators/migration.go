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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oito2/mcp-build82/internal/genutil"
)

// ContextDir is genutil.ContextDir, re-exported for convenience within this package.
const ContextDir = genutil.ContextDir

// GlobalOutputPath is genutil.GlobalOutputPath, re-exported for convenience within this package.
func GlobalOutputPath(moodlePath, filename string) string {
	return genutil.GlobalOutputPath(moodlePath, filename)
}

// PluginOutputPath is genutil.PluginOutputPath, re-exported for convenience within this package.
func PluginOutputPath(pluginPath, filename string) string {
	return genutil.PluginOutputPath(pluginPath, filename)
}

// GlobalContextFilenames is the ordered list of the 13 global Markdown files written under the
// Moodle root's ContextDir.
var GlobalContextFilenames = []string{
	"AI_CONTEXT.md",
	"MOODLE_API_INDEX.md",
	"MOODLE_EVENTS_INDEX.md",
	"MOODLE_TASKS_INDEX.md",
	"MOODLE_SERVICES_INDEX.md",
	"MOODLE_DB_TABLES_INDEX.md",
	"MOODLE_CLASSES_INDEX.md",
	"MOODLE_CAPABILITIES_INDEX.md",
	"MOODLE_PLUGIN_INDEX.md",
	"MOODLE_DEV_RULES.md",
	"MOODLE_PLUGIN_GUIDE.md",
	"MOODLE_AI_WORKSPACE.md",
	"MOODLE_AI_INDEX.md",
}

// PluginContextFiles is the ordered list of the 12 per-plugin Markdown files written under a
// plugin's ContextDir. It is the single list of plugin context files shared across the project.
var PluginContextFiles = []string{
	"PLUGIN_AI_CONTEXT.md",
	"PLUGIN_CONTEXT.md",
	"PLUGIN_STRUCTURE.md",
	"PLUGIN_DB_TABLES.md",
	"PLUGIN_EVENTS.md",
	"PLUGIN_DEPENDENCIES.md",
	"PLUGIN_ARCHITECTURE.md",
	"PLUGIN_SETTINGS.md",
	"PLUGIN_FUNCTION_INDEX.md",
	"PLUGIN_CALLBACK_INDEX.md",
	"PLUGIN_ENDPOINT_INDEX.md",
	"PLUGIN_RUNTIME_FLOW.md",
}

// legacyGlobalFilenames lists the global context files plus "tags" that are migrated when found
// directly at the Moodle root.
var legacyGlobalFilenames = append(append([]string{}, GlobalContextFilenames...), "tags")

// legacyPluginFilenames lists the plugin context files plus .indevelopment that are migrated when
// found directly at a plugin's root.
var legacyPluginFilenames = append(append([]string{}, PluginContextFiles...), ".indevelopment")

// MigrationResult describes the outcome of migrating one file: `File` is the root-relative name,
// `Action` is "moved" or "removed-stale-duplicate" on success (empty on failure), and `Error` is
// non-empty when the file could not be migrated.
type MigrationResult struct {
	File   string
	Action string // "moved" | "removed-stale-duplicate" | ""
	Error  string
}

// logMigrationFailures reports every result with a non-empty Error to stderr, such as a legacy
// file left in its old location after a failed migration (permission denied, disk full).
// Successes stay silent.
func logMigrationFailures(results []MigrationResult) {
	for _, r := range results {
		if r.Error != "" {
			fmt.Fprintf(os.Stderr, "[build82] warning: failed to migrate legacy file %s: %s\n", r.File, r.Error)
		}
	}
}

// MigrateLegacyFiles moves each of `filenames` found directly under `root` into root/.build82/
// and returns one MigrationResult per file that was moved, removed or failed; absent files produce
// no result. When the destination already exists, the stale source is removed instead. The call
// is idempotent. A name that is empty, absolute, contains a path separator, is "..", resolves outside `root`, or is a
// symlink is rejected with an Error result and left untouched. It never panics.
func MigrateLegacyFiles(root string, filenames []string) []MigrationResult {
	destDir := filepath.Join(root, ContextDir)
	var results []MigrationResult

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return results
	}

	for _, f := range filenames {
		if f == "" || filepath.IsAbs(f) || f == ".." || strings.ContainsAny(f, `/\\`) {
			results = append(results, MigrationResult{File: f, Error: "invalid filename: must be a root-relative name without traversal"})
			continue
		}

		src := filepath.Join(root, f)
		absSrc, err := filepath.Abs(src)
		if err != nil || (absSrc != absRoot && !strings.HasPrefix(absSrc, absRoot+string(filepath.Separator))) {
			results = append(results, MigrationResult{File: f, Error: "invalid filename: resolves outside root"})
			continue
		}

		fi, err := os.Lstat(src)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			results = append(results, MigrationResult{File: f, Error: "refusing to migrate a symlink"})
			continue
		}

		if err := os.MkdirAll(destDir, 0o755); err != nil {
			results = append(results, MigrationResult{File: f, Error: err.Error()})
			continue
		}
		dest := filepath.Join(destDir, f)
		if _, err := os.Stat(dest); err == nil {
			// A failed os.Remove (permission denied, file locked) is reported as an error result,
			// not as a "removed-stale-duplicate" success, because the stale file is still at src.
			if rmErr := os.Remove(src); rmErr != nil {
				results = append(results, MigrationResult{File: f, Error: rmErr.Error()})
			} else {
				results = append(results, MigrationResult{File: f, Action: "removed-stale-duplicate"})
			}
			continue
		}
		if err := os.Rename(src, dest); err != nil {
			results = append(results, MigrationResult{File: f, Error: err.Error()})
			continue
		}
		results = append(results, MigrationResult{File: f, Action: "moved"})
	}
	return results
}

// DetectLegacyFiles is the read-only counterpart of MigrateLegacyFiles. It returns the subset of
// `filenames` that exist directly under `root`, without modifying the filesystem.
func DetectLegacyFiles(root string, filenames []string) []string {
	var found []string
	for _, f := range filenames {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			found = append(found, f)
		}
	}
	return found
}

// MigrateLegacyGlobalFiles migrates the global context files and "tags" found at `moodlePath` into its ContextDir.
func MigrateLegacyGlobalFiles(moodlePath string) []MigrationResult {
	return MigrateLegacyFiles(moodlePath, legacyGlobalFilenames)
}

// MigrateLegacyPluginFiles migrates the plugin context files and .indevelopment found at `pluginPath` into its ContextDir.
func MigrateLegacyPluginFiles(pluginPath string) []MigrationResult {
	return MigrateLegacyFiles(pluginPath, legacyPluginFilenames)
}

// DetectLegacyGlobalFiles returns the legacy global files present at `moodlePath` without moving them.
func DetectLegacyGlobalFiles(moodlePath string) []string {
	return DetectLegacyFiles(moodlePath, legacyGlobalFilenames)
}

// DetectLegacyPluginFiles returns the legacy plugin files present at `pluginPath` without moving them.
func DetectLegacyPluginFiles(pluginPath string) []string {
	return DetectLegacyFiles(pluginPath, legacyPluginFilenames)
}
