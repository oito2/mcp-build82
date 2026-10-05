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

package resources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestReadPluginFile_PathTraversalIsRejected verifies that readPluginFile checks
// moodletype.IsWithinMoodle on the client-controlled "component" (from the
// moodle://plugin/{component}... resource URI): an absolute component pointing outside the Moodle
// root, with a real PLUGIN_AI_CONTEXT.md sitting there, must not have its content read into the
// resource response.
func TestReadPluginFile_PathTraversalIsRejected(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", root)

	outside := t.TempDir()
	mustMkdirAll(t, filepath.Join(outside, ".build82"))
	mustWriteFile(t, filepath.Join(outside, ".build82", "PLUGIN_AI_CONTEXT.md"), "SECRET EXTERNAL CONTENT")

	text, err := readPluginFile(outside, "PLUGIN_AI_CONTEXT.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(text, "SECRET EXTERNAL CONTENT") {
		t.Fatalf("path traversal leaked external file content into the resource response: %s", text)
	}
	if !strings.Contains(text, "not found") && !strings.Contains(text, "Not found") {
		t.Errorf("expected a 'not found' style message, got: %s", text)
	}
}

// TestReadPluginFile_WithinRootResolvesNormally confirms the IsWithinMoodle check still accepts the
// legitimate case: a component that resolves within the Moodle root is readable.
func TestReadPluginFile_WithinRootResolvesNormally(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", root)

	pluginDir := filepath.Join(root, "local", "demo")
	mustMkdirAll(t, filepath.Join(pluginDir, ".build82"))
	mustWriteFile(t, filepath.Join(pluginDir, ".build82", "PLUGIN_AI_CONTEXT.md"), "real content")

	text, err := readPluginFile("local_demo", "PLUGIN_AI_CONTEXT.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "real content" {
		t.Errorf("expected 'real content', got %q", text)
	}
}
