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
	"testing"
)

// mustMkdirAll creates the directory `path` (and parents), failing the test on error.
func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

// mustWriteFile writes `content` to `path`, failing the test on error.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestMigrateLegacyFiles_MovesFile verifies an existing legacy file is moved into ContextDir.
func TestMigrateLegacyFiles_MovesFile(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "AI_CONTEXT.md"), "legacy content")

	results := MigrateLegacyFiles(dir, []string{"AI_CONTEXT.md", "MOODLE_API_INDEX.md"})
	if len(results) != 1 || results[0].Action != "moved" {
		t.Fatalf("expected 1 'moved' result, got %+v", results)
	}

	if _, err := os.Stat(filepath.Join(dir, "AI_CONTEXT.md")); err == nil {
		t.Error("expected the legacy root file to no longer exist")
	}
	content, err := os.ReadFile(filepath.Join(dir, ContextDir, "AI_CONTEXT.md"))
	if err != nil || string(content) != "legacy content" {
		t.Errorf("expected migrated content in .build82/, got %q, err=%v", content, err)
	}
}

// TestMigrateLegacyFiles_NothingToMigrate verifies no results are produced when no legacy file exists.
func TestMigrateLegacyFiles_NothingToMigrate(t *testing.T) {
	dir := t.TempDir()
	results := MigrateLegacyFiles(dir, []string{"AI_CONTEXT.md"})
	if len(results) != 0 {
		t.Errorf("expected no results when nothing exists at the legacy location, got %+v", results)
	}
}

// TestMigrateLegacyFiles_Idempotent verifies a second migration call is a no-op.
func TestMigrateLegacyFiles_Idempotent(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "AI_CONTEXT.md"), "v1")

	MigrateLegacyFiles(dir, []string{"AI_CONTEXT.md"})
	second := MigrateLegacyFiles(dir, []string{"AI_CONTEXT.md"})
	if len(second) != 0 {
		t.Errorf("expected a no-op on the second call, got %+v", second)
	}
}

// TestMigrateLegacyFiles_DiscardsStaleDuplicate verifies a legacy copy is removed when ContextDir already has the file.
func TestMigrateLegacyFiles_DiscardsStaleDuplicate(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, ContextDir))
	mustWriteFile(t, filepath.Join(dir, ContextDir, "AI_CONTEXT.md"), "fresh")
	mustWriteFile(t, filepath.Join(dir, "AI_CONTEXT.md"), "stale legacy copy")

	results := MigrateLegacyFiles(dir, []string{"AI_CONTEXT.md"})
	if len(results) != 1 || results[0].Action != "removed-stale-duplicate" {
		t.Fatalf("expected 'removed-stale-duplicate', got %+v", results)
	}
	if _, err := os.Stat(filepath.Join(dir, "AI_CONTEXT.md")); err == nil {
		t.Error("expected the stale root copy to be removed")
	}
	content, _ := os.ReadFile(filepath.Join(dir, ContextDir, "AI_CONTEXT.md"))
	if string(content) != "fresh" {
		t.Errorf("expected the .build82/ copy to remain the 'fresh' one, got %q", content)
	}
}

// TestMigrateLegacyFiles_RejectsTraversalFilename verifies a filename escaping the root is
// rejected and the outside file is left untouched.
func TestMigrateLegacyFiles_RejectsTraversalFilename(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	mustWriteFile(t, filepath.Join(outside, "victim.txt"), "do not touch")

	rel, err := filepath.Rel(dir, filepath.Join(outside, "victim.txt"))
	if err != nil {
		t.Fatal(err)
	}

	results := MigrateLegacyFiles(dir, []string{rel})
	if len(results) != 1 || results[0].Error == "" {
		t.Fatalf("expected an error result for a traversal filename, got %+v", results)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "victim.txt")); statErr != nil {
		t.Errorf("expected the outside file to be untouched, got stat error: %v", statErr)
	}
}

// TestMigrateLegacyFiles_RejectsAbsoluteFilename verifies an absolute filename is rejected and
// the target file is left untouched.
func TestMigrateLegacyFiles_RejectsAbsoluteFilename(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim.txt")
	mustWriteFile(t, victim, "do not touch")

	results := MigrateLegacyFiles(dir, []string{victim})
	if len(results) != 1 || results[0].Error == "" {
		t.Fatalf("expected an error result for an absolute filename, got %+v", results)
	}
	if _, statErr := os.Stat(victim); statErr != nil {
		t.Errorf("expected the outside file to be untouched, got stat error: %v", statErr)
	}
}

// TestMigrateLegacyFiles_RejectsSymlink verifies a legacy entry that is a symlink is refused and
// left in place.
func TestMigrateLegacyFiles_RejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "target.txt")
	mustWriteFile(t, target, "outside content")

	link := filepath.Join(dir, "AI_CONTEXT.md")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	results := MigrateLegacyFiles(dir, []string{"AI_CONTEXT.md"})
	if len(results) != 1 || results[0].Error == "" {
		t.Fatalf("expected an error result for a symlinked legacy file, got %+v", results)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("expected the symlink to be left in place untouched, got: %v", err)
	}
}

// TestDetectLegacyFiles_ReadOnly verifies DetectLegacyFiles reports existing files without moving them.
func TestDetectLegacyFiles_ReadOnly(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "AI_CONTEXT.md"), "legacy content")

	found := DetectLegacyFiles(dir, []string{"AI_CONTEXT.md", "MOODLE_API_INDEX.md"})
	if len(found) != 1 || found[0] != "AI_CONTEXT.md" {
		t.Fatalf("expected to detect AI_CONTEXT.md, got %+v", found)
	}
	// Detection must not move anything.
	if _, err := os.Stat(filepath.Join(dir, "AI_CONTEXT.md")); err != nil {
		t.Error("expected DetectLegacyFiles to leave the file in place")
	}
}

// TestGlobalContextFilenames_Has13Entries verifies the global context file list has 13 entries.
func TestGlobalContextFilenames_Has13Entries(t *testing.T) {
	if len(GlobalContextFilenames) != 13 {
		t.Errorf("expected 13 global context filenames, got %d", len(GlobalContextFilenames))
	}
}

// TestPluginContextFiles_Has12Entries verifies the plugin context file list has 12 entries.
func TestPluginContextFiles_Has12Entries(t *testing.T) {
	if len(PluginContextFiles) != 12 {
		t.Errorf("expected 12 plugin context filenames, got %d", len(PluginContextFiles))
	}
}

// TestMigrateLegacyFiles_DotsInNameAreNotTraversal verifies a name containing ".." inside a
// segment is migrated, while real traversal and separators are rejected.
func TestMigrateLegacyFiles_DotsInNameAreNotTraversal(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a..b"), "x")
	results := MigrateLegacyFiles(root, []string{"a..b", "..", "sub/file", "../x"})
	byFile := map[string]MigrationResult{}
	for _, r := range results {
		byFile[r.File] = r
	}
	if r, ok := byFile["a..b"]; !ok || r.Error != "" {
		t.Errorf("expected a..b to migrate, got %+v", r)
	}
	for _, bad := range []string{"..", "sub/file", "../x"} {
		if byFile[bad].Error == "" {
			t.Errorf("expected %q to be rejected", bad)
		}
	}
}
