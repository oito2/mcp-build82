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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestTargetByID verifies lookup of known and unknown target IDs.
func TestTargetByID(t *testing.T) {
	if _, ok, err := targetByID("claude"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else if !ok {
		t.Error("expected 'claude' target to exist")
	}
	if _, ok, err := targetByID("nonexistent"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else if ok {
		t.Error("expected 'nonexistent' target to not exist")
	}
}

// TestTargets_Has9Entries verifies that the install table lists exactly the supported targets.
func TestTargets_Has9Entries(t *testing.T) {
	ts, err := targets()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(ts); got != 8 {
		t.Errorf("expected 8 targets, got %d", got)
	}
}

// TestTargets_HomeDirUnresolvableReturnsError verifies that targets() returns an error when the
// home directory can't be resolved (unset $HOME), instead of resolving paths relative to the
// working directory.
func TestTargets_HomeDirUnresolvableReturnsError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	if _, err := targets(); err == nil {
		t.Fatal("expected an error when the home directory cannot be resolved")
	}
}

// TestWriteConfig_McpServersShape verifies the entry written for the mcpServers shape.
func TestWriteConfig_McpServersShape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	tg := target{ID: "cursor", InstallPaths: fixedPaths(path), Shape: shapeMcpServers}

	if err := writeConfig(tg, path, "/usr/local/bin/build82", "/var/www/moodle"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected config file to exist: %v", err)
	}
	if jsonErr := json.Unmarshal(content, &parsed); jsonErr != nil {
		t.Fatalf("expected valid JSON: %v", jsonErr)
	}
	servers := parsed["mcpServers"].(map[string]any)
	entry := servers["build82"].(map[string]any)
	if entry["command"] != "/usr/local/bin/build82" {
		t.Errorf("unexpected command: %+v", entry)
	}
	env := entry["env"].(map[string]any)
	if env["BUILD82_MOODLE_PATH"] != "/var/www/moodle" {
		t.Errorf("unexpected env: %+v", env)
	}
}

// TestWriteConfig_NeverOverwritesWholeFile verifies that writing an entry preserves the rest of an existing file.
func TestWriteConfig_NeverOverwritesWholeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	mustWriteFile(t, path, []byte(`{"mcpServers":{"existing":{"command":"/bin/existing"}},"unrelatedTopLevelKey":"keep-me"}`), 0o644)

	tg := target{ID: "cursor", InstallPaths: fixedPaths(path), Shape: shapeMcpServers}
	if err := writeConfig(tg, path, "/usr/local/bin/build82", "/var/www/moodle"); err != nil {
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
	if _, ok := servers["build82"]; !ok {
		t.Error("expected the new build82 entry to be added")
	}
}

// TestWriteConfig_OpenCodeShape verifies the entry written for the OpenCode shape.
func TestWriteConfig_OpenCodeShape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	tg := target{ID: "opencode", InstallPaths: fixedPaths(path), Shape: shapeOpenCode}

	if err := writeConfig(tg, path, "/usr/local/bin/build82", "/var/www/moodle"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	content := mustReadFile(t, path)
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatal(err)
	}
	mcp := parsed["mcp"].(map[string]any)
	entry := mcp["build82"].(map[string]any)
	if entry["type"] != "local" {
		t.Errorf("expected type=local, got %+v", entry)
	}
}

// TestWriteConfig_ZedShape verifies the entry written for the Zed shape.
func TestWriteConfig_ZedShape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	tg := target{ID: "zed", InstallPaths: fixedPaths(path), Shape: shapeZed}

	if err := writeConfig(tg, path, "/usr/local/bin/build82", "/var/www/moodle"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	content := mustReadFile(t, path)
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatal(err)
	}
	servers := parsed["context_servers"].(map[string]any)
	entry := servers["build82"].(map[string]any)
	if entry["command"] != "/usr/local/bin/build82" {
		t.Errorf("unexpected command shape: %+v", entry)
	}
}

// TestClineDir_LinuxDefaultBranch verifies the Cline extension directory on Linux.
func TestClineDir_LinuxDefaultBranch(t *testing.T) {
	// The OS is pinned to Linux to reach the "default" (non-darwin, non-windows) branch.
	prev := goos
	goos = "linux"
	t.Cleanup(func() { goos = prev })
	dir, err := clineDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir == "" {
		t.Error("expected a non-empty clineDir")
	}
	if !containsAll(dir, ".config", "Code", "User", "globalStorage", "saoudrizwan.claude-dev") {
		t.Errorf("expected the Linux default path shape, got %q", dir)
	}
}

// TestClineDir_HomeDirUnresolvableReturnsError is the clineDir-side counterpart of
// TestTargets_HomeDirUnresolvableReturnsError.
func TestClineDir_HomeDirUnresolvableReturnsError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	if _, err := clineDir(); err == nil {
		t.Fatal("expected an error when the home directory cannot be resolved")
	}
}

// containsAll reports whether `s` contains every one of `parts`.
func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !contains(s, p) {
			return false
		}
	}
	return true
}

// contains reports whether `s` contains `substr`.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}
