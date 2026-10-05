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
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oito2/mcp-build82/internal/cache"
)

// captureStderr redirects os.Stderr for the duration of fn and returns everything written to it.
// logMigrationFailures writes directly to os.Stderr, matching every other "best-effort warning" in
// this codebase (e.g. cache.Save()'s own warning) — there's no injectable io.Writer seam for it, so
// capturing the real os.Stderr is the only way to assert on it.
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

// veryOldMtime backdates a legacy fixture file so it is unambiguously older than every source file
// copyFixtureMoodleTree just wrote — the real "installation upgrading from a previous build82
// version" scenario, where the legacy file was last generated long ago and the mtime-based
// staleness check (internal/cache) must correctly find it stale relative to current sources after
// migration relocates it. os.Rename (what migration uses) preserves mtime, so without backdating,
// a legacy file written moments ago in a test would look *newer* than the sources and be (rightly,
// for that scenario) left untouched by the cache — which would make this test assert the wrong
// thing.
func setVeryOldMtime(t *testing.T, path string) {
	t.Helper()
	old := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

// TestGenerateAll_MigratesLegacyGlobalFilesEndToEnd verifies that MigrateLegacyGlobalFiles and
// MigrateLegacyPluginFiles, as called from inside GenerateAll/GenerateAllForPlugin, migrate real
// legacy files left at the root by an older layout, with the migration → cache → generation
// ordering intact.
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
	// Backdated relative to the fixture's sources, so the normal mtime-staleness check (not a
	// migration-specific special case) must have found it stale and regenerated it for real.
	if string(migrated) == "stale legacy global content" {
		t.Error("expected the backdated legacy file to be detected stale and regenerated after migration, not left as the stale placeholder")
	}
}

// TestGenerateAll_MigratedFileNewerThanSourcesIsLeftAsIs confirms migration itself never forces
// regeneration — it only relocates the file. If the legacy content is still fresher than every
// source (the common case: nothing about the plugin changed since it was last
// generated), the normal cache rules correctly skip regenerating it, and the migrated content is
// exactly what was on disk before migration.
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

// TestGenerateAllForPlugin_MigratesLegacyPluginFilesEndToEnd is TestGenerateAll_
// MigratesLegacyGlobalFilesEndToEnd's per-plugin counterpart, covering MigrateLegacyPluginFiles as
// actually invoked by GenerateAllForPlugin.
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

// legacySymlinkFixture creates a legacy-named symlink at root/filename pointing outside root —
// MigrateLegacyFiles always refuses to migrate a symlink, giving a portable, deterministic way to
// force a MigrationResult.Error without touching filesystem permissions.
func legacySymlinkFixture(t *testing.T, root, filename string) {
	t.Helper()
	outside := t.TempDir()
	target := filepath.Join(outside, "target.txt")
	mustWriteFile(t, target, "outside content")
	if err := os.Symlink(target, filepath.Join(root, filename)); err != nil {
		t.Fatalf("symlink: %v", err)
	}
}

// TestGenerateAll_LogsLegacyGlobalMigrationFailureToStderr verifies that GenerateAll logs to stderr
// a failed legacy migration (e.g. a legacy file build82 refuses to move).
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

// TestGenerateAllForPlugin_LogsStaleDuplicateRemovalFailureToStderr verifies that a failed
// os.Remove of a stale legacy duplicate is logged to stderr as an error, not reported as a
// successful "removed-stale-duplicate" migration action.
func TestGenerateAllForPlugin_LogsStaleDuplicateRemovalFailureToStderr(t *testing.T) {
	moodlePath := copyFixtureMoodleTree(t)
	pluginPath := filepath.Join(moodlePath, "local", "demo")

	// Destination already migrated, so MigrateLegacyFiles takes the "stale duplicate" branch
	// (attempts to remove the legacy copy at src), not the "moved" branch.
	mustMkdirAll(t, filepath.Join(pluginPath, ContextDir))
	mustWriteFile(t, PluginOutputPath(pluginPath, "PLUGIN_CONTEXT.md"), "already migrated content")
	legacyPath := filepath.Join(pluginPath, "PLUGIN_CONTEXT.md")
	mustWriteFile(t, legacyPath, "stale legacy plugin content")

	// Removing a file requires write permission on its *parent* directory, not the file itself —
	// making pluginPath read-only forces os.Remove(legacyPath) to fail with permission denied,
	// without affecting writes inside pluginPath/.build82/ (a separate, still-writable directory
	// that already exists from the mustWriteFile call above).
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

// TestGenerateAllForPlugin_LogsLegacyPluginMigrationFailureToStderr is
// TestGenerateAll_LogsLegacyGlobalMigrationFailureToStderr's per-plugin counterpart, covering
// MigrateLegacyPluginFiles as actually invoked by GenerateAllForPlugin.
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
