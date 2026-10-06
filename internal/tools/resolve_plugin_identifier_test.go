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

package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// setupResolveFixture creates a Moodle root holding local/demo (component local_demo) and points
// the configuration at it. Returns the Moodle root and the plugin's absolute path.
func setupResolveFixture(t *testing.T) (moodlePath, pluginPath string) {
	t.Helper()
	freshCache(t)
	t.Setenv("HOME", t.TempDir())
	moodlePath = t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)
	pluginPath = filepath.Join(moodlePath, "local", "demo")
	mustMkdirAll(t, filepath.Join(pluginPath, "lang", "en"))
	mustWriteFile(t, filepath.Join(pluginPath, "version.php"),
		"<?php\n$plugin->component = 'local_demo';\n$plugin->version = 2024010100;\n$plugin->requires = 2022041900;\n")
	mustWriteFile(t, filepath.Join(pluginPath, "lang", "en", "local_demo.php"),
		"<?php\n$string['pluginname'] = 'Demo';\n")
	return moodlePath, pluginPath
}

// resolveResultText returns the text of the first content block of `res`, failing the test when
// the result is empty.
func resolveResultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		t.Fatal("expected a non-empty result")
	}
	return res.Content[0].(*mcp.TextContent).Text
}

// TestResolveAndValidatePlugin_AcceptsAllIdentifierForms confirms a frankenstyle component, a
// Moodle-root-relative path, and an absolute path all resolve to the same plugin directory.
func TestResolveAndValidatePlugin_AcceptsAllIdentifierForms(t *testing.T) {
	moodlePath, pluginPath := setupResolveFixture(t)

	for _, id := range []string{"local_demo", "local/demo", pluginPath} {
		rp, errResult := resolveAndValidatePlugin(id, moodlePath)
		if errResult != nil {
			t.Fatalf("identifier %q: unexpected error: %s", id, resolveResultText(t, errResult))
		}
		if rp.Path != pluginPath || rp.Info.Component != "local_demo" {
			t.Errorf("identifier %q: resolved to %q (%s), want %q (local_demo)", id, rp.Path, rp.Info.Component, pluginPath)
		}
	}
}

// TestResolveAndValidatePlugin_ComponentOfMissingPluginIsNotFound confirms an unresolvable
// component still gets the ordinary "not found" message rather than a containment error.
func TestResolveAndValidatePlugin_ComponentOfMissingPluginIsNotFound(t *testing.T) {
	moodlePath, _ := setupResolveFixture(t)

	_, errResult := resolveAndValidatePlugin("local_missing", moodlePath)
	if errResult == nil {
		t.Fatal("expected an error for a component that does not exist")
	}
	text := resolveResultText(t, errResult)
	if !strings.Contains(text, "not found") || strings.Contains(text, moodlePath) {
		t.Errorf("expected a root-relative not-found message, got: %s", text)
	}
}

// TestResolveAndValidatePlugin_ComponentTraversalIsRejected confirms the component form cannot be
// used to escape the Moodle root: an empty type with ".." as the name resolves to the root's
// parent, which the containment check must reject.
func TestResolveAndValidatePlugin_ComponentTraversalIsRejected(t *testing.T) {
	moodlePath, _ := setupResolveFixture(t)

	for _, id := range []string{"_..", "local/../../x", "../local/demo"} {
		_, errResult := resolveAndValidatePlugin(id, moodlePath)
		if errResult == nil {
			t.Fatalf("identifier %q: expected rejection", id)
		}
		if text := resolveResultText(t, errResult); !strings.Contains(text, "Invalid plugin path") {
			t.Errorf("identifier %q: expected the containment rejection, got: %s", id, text)
		}
	}
}

// TestPluginTools_AcceptComponentIdentifier confirms each tool built on resolveAndValidatePlugin
// accepts the component form its schema advertises, and reports the plugin path relative to the
// Moodle root rather than as an absolute host path.
func TestPluginTools_AcceptComponentIdentifier(t *testing.T) {
	moodlePath, _ := setupResolveFixture(t)
	ctx := context.Background()

	explain, _, _ := handleExplainPlugin(ctx, nil, ExplainPluginInput{Plugin: "local_demo", Section: SectionOverview})
	generate, _, _ := handleGenerateContext(ctx, nil, GenerateContextInput{PluginPath: "local_demo"})
	release, _, _ := handleReleasePlugin(ctx, nil, ReleasePluginInput{Component: "local_demo", OutputDir: t.TempDir()})

	for name, res := range map[string]*mcp.CallToolResult{
		"explain_plugin": explain, "generate_plugin_context": generate, "release_plugin": release,
	} {
		text := resolveResultText(t, res)
		if res.IsError {
			t.Errorf("%s: component identifier rejected: %s", name, text)
			continue
		}
		if strings.Contains(text, moodlePath) {
			t.Errorf("%s: response leaks the absolute Moodle root: %s", name, text)
		}
		if !strings.Contains(text, "local/demo") {
			t.Errorf("%s: expected the root-relative plugin path local/demo, got: %s", name, text)
		}
	}
}

// TestGenerateContext_JSONPathIsRelative confirms the JSON form of generate_plugin_context
// reports the plugin path relative to the Moodle root.
func TestGenerateContext_JSONPathIsRelative(t *testing.T) {
	setupResolveFixture(t)

	res, _, _ := handleGenerateContext(context.Background(), nil, GenerateContextInput{PluginPath: "local/demo", Format: FormatJSON})
	var out PluginContextOutput
	if err := json.Unmarshal([]byte(resolveResultText(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Path != "local/demo" {
		t.Errorf("path = %q, want local/demo", out.Path)
	}
}

// TestGetPluginInfo_PathIsRelativeToMoodleRoot covers both get_plugin_info response shapes (the
// metadata table when no context has been generated yet, in text and JSON) and the cached
// PLUGIN_AI_CONTEXT.md JSON response.
func TestGetPluginInfo_PathIsRelativeToMoodleRoot(t *testing.T) {
	moodlePath, pluginPath := setupResolveFixture(t)
	ctx := context.Background()

	text, _, _ := handleGetPluginInfo(ctx, nil, GetPluginInfoInput{Plugin: "local_demo"})
	if got := resolveResultText(t, text); strings.Contains(got, moodlePath) || !strings.Contains(got, "| Path | local/demo |") {
		t.Errorf("text: expected root-relative path, got: %s", got)
	}

	var info struct{ Path string }
	js, _, _ := handleGetPluginInfo(ctx, nil, GetPluginInfoInput{Plugin: "local_demo", Format: FormatJSON})
	if err := json.Unmarshal([]byte(resolveResultText(t, js)), &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if info.Path != "local/demo" {
		t.Errorf("json: Path = %q, want local/demo", info.Path)
	}

	mustMkdirAll(t, filepath.Join(pluginPath, ".build82"))
	mustWriteFile(t, filepath.Join(pluginPath, ".build82", "PLUGIN_AI_CONTEXT.md"), "# cached\n")
	var cached struct {
		Path string `json:"path"`
	}
	js, _, _ = handleGetPluginInfo(ctx, nil, GetPluginInfoInput{Plugin: pluginPath, Format: FormatJSON})
	if err := json.Unmarshal([]byte(resolveResultText(t, js)), &cached); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cached.Path != "local/demo" {
		t.Errorf("cached json: path = %q, want local/demo", cached.Path)
	}
}
