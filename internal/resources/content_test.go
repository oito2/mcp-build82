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

package resources

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file tests the per-plugin resource templates' URI parsing (makePluginFileHandler) across
// all pluginFileResources definitions, plus the aggregate moodle://plugins/with-context resource.

// TestMakePluginFileHandler_EveryTemplateResolvesItsOwnFile verifies that every pluginFileResources
// definition strips its own URI suffix and serves its own file, so swapped Suffix/Filename entries
// are detected.
func TestMakePluginFileHandler_EveryTemplateResolvesItsOwnFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", root)

	pluginDir := filepath.Join(root, "local", "demo")
	mustMkdirAll(t, filepath.Join(pluginDir, ".build82"))

	for _, def := range pluginFileResources {
		marker := "CONTENT FOR " + def.Filename
		mustWriteFile(t, filepath.Join(pluginDir, ".build82", def.Filename), marker)
	}

	for _, def := range pluginFileResources {
		def := def
		t.Run(def.Filename, func(t *testing.T) {
			handler := makePluginFileHandler(def)
			uri := "moodle://plugin/local_demo" + def.Suffix
			result, err := handler(context.Background(), &mcp.ReadResourceRequest{
				Params: &mcp.ReadResourceParams{URI: uri},
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result.Contents) != 1 {
				t.Fatalf("expected exactly 1 content block, got %d", len(result.Contents))
			}
			want := "CONTENT FOR " + def.Filename
			if got := result.Contents[0].Text; got != want {
				t.Errorf("suffix %q: expected %q, got %q", def.Suffix, want, got)
			}
			if result.Contents[0].URI != uri {
				t.Errorf("expected echoed URI %q, got %q", uri, result.Contents[0].URI)
			}
		})
	}
}

// TestHandlePluginsWithContext_ListsOnlyPluginsWithGeneratedContext verifies the listing includes
// only plugins that have a generated PLUGIN_AI_CONTEXT.md.
func TestHandlePluginsWithContext_ListsOnlyPluginsWithGeneratedContext(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", root)

	withContext := filepath.Join(root, "local", "withcontext")
	mustMkdirAll(t, filepath.Join(withContext, ".build82"))
	mustWriteFile(t, filepath.Join(withContext, "version.php"), "<?php\n$plugin->component = 'local_withcontext';\n")
	mustWriteFile(t, filepath.Join(withContext, ".build82", "PLUGIN_AI_CONTEXT.md"), "context")

	withoutContext := filepath.Join(root, "local", "withoutcontext")
	mustMkdirAll(t, withoutContext)
	mustWriteFile(t, filepath.Join(withoutContext, "version.php"), "<?php\n$plugin->component = 'local_withoutcontext';\n")

	result, err := handlePluginsWithContext(context.Background(), &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: "moodle://plugins/with-context"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := result.Contents[0].Text
	if !strings.Contains(text, "local_withcontext") {
		t.Errorf("expected local_withcontext listed, got:\n%s", text)
	}
	if strings.Contains(text, "local_withoutcontext") {
		t.Errorf("expected local_withoutcontext NOT listed (no generated context), got:\n%s", text)
	}
}
