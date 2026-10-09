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
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/oito2/mcp-build82/internal/cache"
)

// captureStderr redirects os.Stderr for the duration of fn and returns everything written to it.
// logMigrationFailures writes directly to os.Stderr and has no injectable writer, so the real
// os.Stderr is swapped for a pipe.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	return string(out)
}

// setVeryOldMtime backdates the file at `path` by 24 hours so it is older than every source file
// written by copyFixtureMoodleTree. Migration uses os.Rename, which preserves mtime, so without
// backdating a freshly written legacy file would look newer than the sources and be treated as
// up to date by the cache.
func setVeryOldMtime(t *testing.T, path string) {
	t.Helper()
	old := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

// TestGenerateAll_MigratesLegacyGlobalFilesEndToEnd verifies that GenerateAll moves a stale legacy
// root-level file into ContextDir and then regenerates it, in migration, cache, generation order.
func TestGenerateAll_MigratesLegacyGlobalFilesEndToEnd(t *testing.T) {
	moodlePath := copyFixtureMoodleTree(t)
	legacyPath := filepath.Join(moodlePath, "AI_CONTEXT.md")
	mustWriteFile(t, legacyPath, "stale legacy global content")
	setVeryOldMtime(t, legacyPath)

	c := cache.NewMtimeCache()
	old := swapGlobalCache(c)
	defer swapGlobalCache(old)

	results := GenerateAll(moodlePath, "5.0")
	for _, r := range results {
		if !r.Success {
			t.Errorf("expected every global generator to succeed, got %+v", r)
		}
	}

	if _, err := os.Stat(legacyPath); err == nil {
		t.Error("expected the legacy root-level file to be migrated away, but it still exists")
	}
	migrated, err := os.ReadFile(GlobalOutputPath(moodlePath, "AI_CONTEXT.md"))
	if err != nil {
		t.Fatalf("expected the legacy content to have been moved into %s: %v", ContextDir, err)
	}
	// The file was backdated, so the normal staleness check must have regenerated it.
	if string(migrated) == "stale legacy global content" {
		t.Error("expected the backdated legacy file to be detected stale and regenerated after migration, not left as the stale placeholder")
	}
}

// TestGenerateAll_MigratedFileNewerThanSourcesIsLeftAsIs verifies that migration only relocates a
// file: a legacy file newer than every source is skipped by the cache and keeps its content.
func TestGenerateAll_MigratedFileNewerThanSourcesIsLeftAsIs(t *testing.T) {
	moodlePath := copyFixtureMoodleTree(t)
	legacyPath := filepath.Join(moodlePath, "AI_CONTEXT.md")
	mustWriteFile(t, legacyPath, "still-fresh legacy global content")

	c := cache.NewMtimeCache()
	old := swapGlobalCache(c)
	defer swapGlobalCache(old)

	GenerateAll(moodlePath, "5.0")

	migrated, err := os.ReadFile(GlobalOutputPath(moodlePath, "AI_CONTEXT.md"))
	if err != nil {
		t.Fatalf("expected the legacy content to have been moved into %s: %v", ContextDir, err)
	}
	if string(migrated) != "still-fresh legacy global content" {
		t.Errorf("expected the migrated-but-still-fresh file to be left untouched, got:\n%s", migrated)
	}
}

// TestGenerateAllForPlugin_MigratesLegacyPluginFilesEndToEnd is the per-plugin counterpart of the
// global migration test: GenerateAllForPlugin migrates and regenerates a stale legacy plugin file.
func TestGenerateAllForPlugin_MigratesLegacyPluginFilesEndToEnd(t *testing.T) {
	moodlePath := copyFixtureMoodleTree(t)
	pluginPath := filepath.Join(moodlePath, "local", "demo")
	legacyPath := filepath.Join(pluginPath, "PLUGIN_CONTEXT.md")
	mustWriteFile(t, legacyPath, "stale legacy plugin content")
	setVeryOldMtime(t, legacyPath)

	c := cache.NewMtimeCache()
	old := swapGlobalCache(c)
	defer swapGlobalCache(old)

	result := GenerateAllForPlugin(pluginPath, moodlePath, true, nil)
	for _, f := range result.Files {
		if !f.Success {
			t.Errorf("expected every per-plugin generator to succeed, got %+v", f)
		}
	}

	if _, err := os.Stat(legacyPath); err == nil {
		t.Error("expected the legacy plugin-root file to be migrated away, but it still exists")
	}
	migrated, err := os.ReadFile(PluginOutputPath(pluginPath, "PLUGIN_CONTEXT.md"))
	if err != nil {
		t.Fatalf("expected the legacy content to have been moved into %s: %v", ContextDir, err)
	}
	if string(migrated) == "stale legacy plugin content" {
		t.Error("expected the backdated legacy file to be detected stale and regenerated after migration, not left as the stale placeholder")
	}
}

// legacySymlinkFixture creates a symlink at root/filename pointing outside `root`. Because
// MigrateLegacyFiles refuses symlinks, this forces a MigrationResult.Error without changing
// filesystem permissions.
func legacySymlinkFixture(t *testing.T, root, filename string) {
	t.Helper()
	outside := t.TempDir()
	target := filepath.Join(outside, "target.txt")
	mustWriteFile(t, target, "outside content")
	if err := os.Symlink(target, filepath.Join(root, filename)); err != nil {
		t.Fatalf("symlink: %v", err)
	}
}

// TestGenerateAll_LogsLegacyGlobalMigrationFailureToStderr verifies that GenerateAll logs a failed
// legacy migration (a refused symlink) to stderr.
func TestGenerateAll_LogsLegacyGlobalMigrationFailureToStderr(t *testing.T) {
	moodlePath := copyFixtureMoodleTree(t)
	legacySymlinkFixture(t, moodlePath, "AI_CONTEXT.md")

	c := cache.NewMtimeCache()
	old := swapGlobalCache(c)
	defer swapGlobalCache(old)

	stderr := captureStderr(t, func() { GenerateAll(moodlePath, "5.0") })

	if !strings.Contains(stderr, "AI_CONTEXT.md") || !strings.Contains(stderr, "refusing to migrate a symlink") {
		t.Errorf("expected the migration failure logged to stderr, got:\n%s", stderr)
	}
}

// TestGenerateAllForPlugin_LogsStaleDuplicateRemovalFailureToStderr verifies that a failed removal
// of a stale legacy duplicate is logged to stderr and the stale file stays in place.
func TestGenerateAllForPlugin_LogsStaleDuplicateRemovalFailureToStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a read-only directory cannot be made with chmod on Windows")
	}
	moodlePath := copyFixtureMoodleTree(t)
	pluginPath := filepath.Join(moodlePath, "local", "demo")

	// The destination already exists, so MigrateLegacyFiles takes the stale-duplicate branch.
	mustMkdirAll(t, filepath.Join(pluginPath, ContextDir))
	mustWriteFile(t, PluginOutputPath(pluginPath, "PLUGIN_CONTEXT.md"), "already migrated content")
	legacyPath := filepath.Join(pluginPath, "PLUGIN_CONTEXT.md")
	mustWriteFile(t, legacyPath, "stale legacy plugin content")

	// Removing a file needs write permission on its parent directory, so a read-only pluginPath
	// makes the removal fail while the separate ContextDir stays writable.
	if err := os.Chmod(pluginPath, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(pluginPath, 0o755) }) // let t.TempDir() clean up afterward

	c := cache.NewMtimeCache()
	old := swapGlobalCache(c)
	defer swapGlobalCache(old)

	stderr := captureStderr(t, func() { GenerateAllForPlugin(pluginPath, moodlePath, true, nil) })

	if !strings.Contains(stderr, "PLUGIN_CONTEXT.md") || !strings.Contains(stderr, "permission denied") {
		t.Errorf("expected the stale-duplicate removal failure logged to stderr, got:\n%s", stderr)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Error("expected the stale legacy file to still exist, since its removal failed")
	}
}

// TestGenerateAllForPlugin_LogsLegacyPluginMigrationFailureToStderr is the per-plugin counterpart
// of the global migration-failure test.
func TestGenerateAllForPlugin_LogsLegacyPluginMigrationFailureToStderr(t *testing.T) {
	moodlePath := copyFixtureMoodleTree(t)
	pluginPath := filepath.Join(moodlePath, "local", "demo")
	legacySymlinkFixture(t, pluginPath, "PLUGIN_CONTEXT.md")

	c := cache.NewMtimeCache()
	old := swapGlobalCache(c)
	defer swapGlobalCache(old)

	stderr := captureStderr(t, func() { GenerateAllForPlugin(pluginPath, moodlePath, true, nil) })

	if !strings.Contains(stderr, "PLUGIN_CONTEXT.md") || !strings.Contains(stderr, "refusing to migrate a symlink") {
		t.Errorf("expected the migration failure logged to stderr, got:\n%s", stderr)
	}
}
