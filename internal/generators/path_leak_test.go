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
	"testing"
)

// This file verifies that generated Markdown never embeds absolute filesystem paths (host
// directory structure, often an OS username), since those files travel with the plugin repo and
// may be committed, shared, or pasted into a third-party AI chat.

func TestGeneratePluginContext_PathIsRelativeNotAbsolute(t *testing.T) {
	moodlePath := t.TempDir()
	pluginPath := filepath.Join(moodlePath, "local", "demo")
	mustMkdirAll(t, pluginPath)
	mustWriteFile(t, filepath.Join(pluginPath, "version.php"), "<?php\n$plugin->component = 'local_demo';\n")

	info := testPluginInfo(pluginPath)
	info.MoodlePath = moodlePath

	result := GeneratePluginContext(info, nil)
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	content, _ := os.ReadFile(PluginOutputPath(pluginPath, "PLUGIN_CONTEXT.md"))
	s := string(content)

	if strings.Contains(s, moodlePath) {
		t.Errorf("expected no absolute host path leaked, got:\n%s", s)
	}
	if !strings.Contains(s, "local/demo") {
		t.Errorf("expected the plugin's path relative to the Moodle root, got:\n%s", s)
	}
}

func TestGenerateAiContext_PathDoesNotLeakAbsoluteHostPath(t *testing.T) {
	moodlePath := t.TempDir()

	result := GenerateAiContext(moodlePath, "5.0", nil)
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	content, _ := os.ReadFile(GlobalOutputPath(moodlePath, "AI_CONTEXT.md"))
	s := string(content)

	if strings.Contains(s, moodlePath) {
		t.Errorf("expected no absolute host path leaked, got:\n%s", s)
	}
	if !strings.Contains(s, filepath.Base(moodlePath)) {
		t.Errorf("expected the installation directory's own name still shown, got:\n%s", s)
	}
}
