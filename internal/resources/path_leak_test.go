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
	"path/filepath"
	"strings"
	"testing"
)

// This file tests that the "Not found" resource messages never embed an absolute filesystem path:
// a missing generated file is reported relative to the Moodle root, and an unresolvable plugin
// only refers to "the configured Moodle root" — an absolute path would leak host directory structure (and often an OS username) into a
// message that could end up quoted back into a third-party AI chat.

func TestReadMoodleFile_NotFoundMessageDoesNotLeakAbsolutePath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", root)

	text, err := readMoodleFile("AI_CONTEXT.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(text, root) {
		t.Errorf("expected no absolute host path leaked, got:\n%s", text)
	}
	if !strings.Contains(text, filepath.Join(".build82", "AI_CONTEXT.md")) {
		t.Errorf("expected the path relative to the Moodle root, got:\n%s", text)
	}
}

func TestReadPluginFile_NotFoundMessageDoesNotLeakAbsolutePath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", root)

	pluginDir := filepath.Join(root, "local", "demo")
	mustMkdirAll(t, pluginDir)
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"), "<?php\n$plugin->component = 'local_demo';\n")

	text, err := readPluginFile("local_demo", "PLUGIN_CONTEXT.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(text, root) {
		t.Errorf("expected no absolute host path leaked, got:\n%s", text)
	}
	if !strings.Contains(text, filepath.Join("local", "demo", ".build82", "PLUGIN_CONTEXT.md")) {
		t.Errorf("expected the path relative to the Moodle root, got:\n%s", text)
	}
}

func TestReadPluginFile_PluginNotFoundMessageDoesNotLeakAbsolutePath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", root)

	text, err := readPluginFile("local_missing", "PLUGIN_CONTEXT.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(text, "# Plugin not found") {
		t.Fatalf("expected the plugin-not-found message, got:\n%s", text)
	}
	if strings.Contains(text, root) {
		t.Errorf("expected no absolute host path leaked, got:\n%s", text)
	}
	if !strings.Contains(text, "local_missing") {
		t.Errorf("expected the requested component to be named, got:\n%s", text)
	}
}
