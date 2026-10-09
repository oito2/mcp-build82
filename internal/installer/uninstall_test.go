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

package installer

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oito2/mcp-build82/internal/config"
	"github.com/oito2/mcp-build82/internal/generators"
)

// TestHasEntry_FileTarget verifies that hasEntry detects a build82 entry in a file target.
func TestHasEntry_FileTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	tg := target{ID: "cursor", InstallPaths: fixedPaths(path), Shape: shapeMcpServers}

	if hasEntry(tg) {
		t.Error("expected no entry before the config file exists")
	}

	if err := writeConfig(tg, path, "/usr/local/bin/build82", "/var/www/moodle"); err != nil {
		t.Fatal(err)
	}
	if !hasEntry(tg) {
		t.Error("expected an entry after writeConfig")
	}
}

// TestRemoveEntryFile_NeverCreatesMissingConfig verifies that removing from a missing file does not create it.
func TestRemoveEntryFile_NeverCreatesMissingConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	tg := target{ID: "cursor", InstallPaths: fixedPaths(path), Shape: shapeMcpServers}

	if _, err := removeEntryFile(path, tg.Shape); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("removeEntryFile must not create a config file that never existed")
	}
}

// TestRemoveEntryFile_RemovesOnlyOwnKeyPreservesRest verifies that only the build82 key is deleted from a config file.
func TestRemoveEntryFile_RemovesOnlyOwnKeyPreservesRest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	mustWriteFile(t, path, []byte(`{"mcpServers":{"existing":{"command":"/bin/existing"},"build82":{"command":"/bin/build82"}},"unrelatedTopLevelKey":"keep-me"}`), 0o644)

	tg := target{ID: "cursor", InstallPaths: fixedPaths(path), Shape: shapeMcpServers}
	if _, err := removeEntryFile(path, tg.Shape); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	content := mustReadFile(t, path)
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["unrelatedTopLevelKey"] != "keep-me" {
		t.Error("expected unrelated top-level keys to survive")
	}
	servers := parsed["mcpServers"].(map[string]any)
	if _, ok := servers["existing"]; !ok {
		t.Error("expected the pre-existing mcpServers entry to survive")
	}
	if _, ok := servers["build82"]; ok {
		t.Error("expected the build82 entry to be removed")
	}
}

// TestUninstall_FileTarget_RoundTripsWithInstall verifies that uninstall undoes an install for a file target.
func TestUninstall_FileTarget_RoundTripsWithInstall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	tg := target{ID: "cursor", InstallPaths: fixedPaths(path), Shape: shapeMcpServers}

	if _, _, err := installTarget(tg, "/usr/local/bin/build82", "/var/www/moodle"); err != nil {
		t.Fatal(err)
	}
	if !hasEntry(tg) {
		t.Fatal("expected an entry right after install")
	}
	if _, _, err := uninstallTarget(tg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasEntry(tg) {
		t.Error("expected no entry after uninstall")
	}
}

// TestPurgeCandidates_OnlyListsFilesThatExist verifies that purge candidates include only existing generated files.
func TestPurgeCandidates_OnlyListsFilesThatExist(t *testing.T) {
	moodleRoot := t.TempDir()

	// Create only 2 of the 13 global files.
	mustMkdir(t, filepath.Join(moodleRoot, generators.ContextDir))
	mustWriteFile(t, filepath.Join(moodleRoot, generators.ContextDir, "AI_CONTEXT.md"), []byte("x"), 0o644)
	mustWriteFile(t, filepath.Join(moodleRoot, generators.ContextDir, "MOODLE_API_INDEX.md"), []byte("x"), 0o644)
	// A file NOT in the tracked list must never be picked up.
	mustWriteFile(t, filepath.Join(moodleRoot, generators.ContextDir, ".cache.json"), []byte("{}"), 0o644)

	// One dev-marked plugin with 1 of its 11 files present.
	pluginDir := filepath.Join(moodleRoot, "local", "demo")
	mustMkdir(t, filepath.Join(pluginDir, generators.ContextDir))
	mustWriteFile(t, filepath.Join(pluginDir, generators.ContextDir, ".indevelopment"), []byte("x"), 0o644)
	mustWriteFile(t, filepath.Join(pluginDir, generators.ContextDir, "PLUGIN_CONTEXT.md"), []byte("x"), 0o644)

	cfg := &config.Config{MoodlePath: moodleRoot}
	_, files, err := purgeCandidates(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 4 { // 2 global + 1 plugin file + 1 .indevelopment marker
		t.Errorf("expected 4 candidate files, got %d: %v", len(files), files)
	}
	for _, f := range files {
		if strings.Contains(f, ".cache.json") {
			t.Errorf("cache file must never be a purge candidate, got %v", files)
		}
	}
}

// TestPurgeCandidates_HomeDirUnresolvableReturnsError verifies that purgeCandidates returns an
// error when config.FilePath() can't resolve the home directory.
func TestPurgeCandidates_HomeDirUnresolvableReturnsError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	cfg := &config.Config{MoodlePath: t.TempDir()}
	if _, _, err := purgeCandidates(cfg); err == nil {
		t.Fatal("expected an error when the home directory cannot be resolved")
	}
}

// TestRunPurge_RequiresConfirmation verifies that purge deletes nothing unless the answer is "y".
func TestRunPurge_RequiresConfirmation(t *testing.T) {
	moodleRoot := t.TempDir()
	mustMkdir(t, filepath.Join(moodleRoot, generators.ContextDir))
	targetFile := filepath.Join(moodleRoot, generators.ContextDir, "AI_CONTEXT.md")
	mustWriteFile(t, targetFile, []byte("x"), 0o644)

	cfg := &config.Config{MoodlePath: moodleRoot}

	// Answering "n" must leave the file untouched.
	in := bufio.NewReader(strings.NewReader("n\n"))
	if err := runPurge(context.Background(), cfg, in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(targetFile); err != nil {
		t.Error("file must survive when the user declines the purge confirmation")
	}

	// Answering "y" deletes it.
	in = bufio.NewReader(strings.NewReader("y\n"))
	if err := runPurge(context.Background(), cfg, in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(targetFile); err == nil {
		t.Error("file must be deleted after confirming the purge")
	}
}
