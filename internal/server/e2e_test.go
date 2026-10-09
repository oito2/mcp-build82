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

package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/cache"
	"github.com/oito2/mcp-build82/internal/generators"
)

// buildE2EFixture copies the shared testdata/moodle fixture and extends it into a multi-plugin
// installation: the dev-marked demo plugin, an assignsubmission_file plugin, a plugin whose
// output directory is read-only so its generation fails, and legacy root-level generated files.
// It returns the Moodle root.
func buildE2EFixture(t *testing.T) string {
	t.Helper()
	root := copyFixtureMoodleTree(t) // contains local/demo, which is not dev-marked yet.
	markDev(t, filepath.Join(root, "local", "demo"))

	// An assignsubmission_* plugin, to exercise splitting the component at the first underscore.
	asubDir := filepath.Join(root, "mod", "assign", "submission", "file")
	mustMkdirAll(t, asubDir)
	mustWriteFile(t, filepath.Join(asubDir, "version.php"), "<?php\n"+
		"$plugin->component = 'assignsubmission_file';\n"+
		"$plugin->version   = 2024010100;\n"+
		"$plugin->requires  = 2023100900;\n"+
		"$plugin->maturity  = MATURITY_STABLE;\n")
	markDev(t, asubDir)

	// A plugin whose .build82/ output directory is read-only, so every write fails, to check that
	// a batch does not abort on one bad plugin.
	brokenDir := filepath.Join(root, "local", "broken")
	mustMkdirAll(t, brokenDir)
	mustWriteFile(t, filepath.Join(brokenDir, "version.php"), "<?php\n$plugin->component = 'local_broken';\n$plugin->version = 2024010100;\n")
	brokenContextDir := markDev(t, brokenDir)
	if err := os.Chmod(brokenContextDir, 0o555); err != nil {
		t.Fatalf("chmod broken plugin context dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(brokenContextDir, 0o755) }) // let t.TempDir() clean up afterward

	// Generated files at the Moodle root, outside .build82/, which generation must migrate.
	for _, f := range append(append([]string{}, generators.GlobalContextFilenames...), "tags") {
		mustWriteFile(t, filepath.Join(root, f), "legacy content for "+f+"\n")
	}

	return root
}

// markDev creates the .build82/.indevelopment marker that makes the plugin at `pluginDir` count as
// a dev plugin, and returns the path of the .build82 directory.
func markDev(t *testing.T, pluginDir string) string {
	t.Helper()
	ctxDir := filepath.Join(pluginDir, generators.ContextDir)
	mustMkdirAll(t, ctxDir)
	mustWriteFile(t, filepath.Join(ctxDir, ".indevelopment"), "1\n")
	return ctxDir
}

// mustMkdirAll creates `path` and any missing parents, failing the test on error.
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

// callTool calls the tool `name` with `args` on `session` and returns the result. It fails the test
// when the call itself returns a protocol error; tool-level errors are reported through IsError.
func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	return result
}

// toolText returns the text of the only content block in `result`, failing the test when there is
// not exactly one text block.
func toolText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("expected exactly one content block, got %d: %+v", len(result.Content), result.Content)
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected a TextContent block, got %T", result.Content[0])
	}
	return tc.Text
}

// TestE2E_Phase8Invariants checks the server's core behavioral guarantees through the MCP tool
// layer, one subtest each, against the multi-plugin fixture.
//
// The subtests depend on their order: the legacy-migration subtest must run before any other
// subtest that triggers a full generators.GenerateAll, because that run migrates the legacy root
// files the subtest asserts on.
func TestE2E_Phase8Invariants(t *testing.T) {
	withIsolatedHome(t)
	root := buildE2EFixture(t)
	t.Setenv("BUILD82_MOODLE_PATH", root)

	_, session, cleanup := connectInMemory(t)
	defer cleanup()

	t.Run("invariant_6_path_traversal_guard", func(t *testing.T) {
		result := callTool(t, session, "generate_plugin_context", map[string]any{"plugin_path": "../../etc"})
		if !result.IsError {
			t.Fatal("expected IsError=true for a path traversal attempt")
		}
		if text := toolText(t, result); !strings.Contains(text, "Invalid plugin path") {
			t.Errorf("unexpected error message: %s", text)
		}
	})

	// The next two subtests verify that plugin_batch's "list" mode and release_plugin reject a
	// plugin identifier that resolves to an existing directory outside the Moodle root.
	t.Run("path_traversal_guard_plugin_batch_list", func(t *testing.T) {
		outside := t.TempDir()
		result := callTool(t, session, "plugin_batch", map[string]any{
			"mode": "list", "plugins": []string{outside},
		})
		if !result.IsError {
			t.Fatalf("expected IsError=true for a plugin_batch list identifier outside the Moodle root, got: %s", toolText(t, result))
		}
	})

	t.Run("path_traversal_guard_release_plugin", func(t *testing.T) {
		// release_plugin accepts absolute paths, so the containment check must reject this one.
		outside := t.TempDir()

		result := callTool(t, session, "release_plugin", map[string]any{"component": outside})
		if !result.IsError {
			t.Fatalf("expected IsError=true for a release_plugin component pointing outside the Moodle root, got: %s", toolText(t, result))
		}
		if text := toolText(t, result); !strings.Contains(text, "Invalid plugin path") {
			t.Errorf("expected rejection via the Moodle-root containment check (\"Invalid plugin path\"), got a different error: %s", text)
		}
	})

	// The next two subtests verify that get_plugin_info accepts component-style identifiers such as
	// "local_demo", and that a path outside the Moodle root yields a "not found" style response
	// without leaking its metadata.
	t.Run("get_plugin_info_component_syntax", func(t *testing.T) {
		result := callTool(t, session, "get_plugin_info", map[string]any{"plugin": "local_demo"})
		if result.IsError {
			t.Fatalf("expected success for a component-style identifier, got IsError: %s", toolText(t, result))
		}
	})

	t.Run("path_traversal_guard_get_plugin_info", func(t *testing.T) {
		outside := t.TempDir()
		mustWriteFile(t, filepath.Join(outside, "version.php"),
			"<?php\n$plugin->component = 'local_outside';\n$plugin->version = 2024010100;\n")

		result := callTool(t, session, "get_plugin_info", map[string]any{"plugin": outside})
		text := toolText(t, result)
		if strings.Contains(text, "local_outside") {
			t.Fatalf("path traversal leaked metadata from outside the Moodle root: %s", text)
		}
	})

	// The next two subtests verify the size limits on the search query length and on the number of
	// plugin identifiers accepted by plugin_batch's "list" mode.
	t.Run("dos_guard_search_query_too_long", func(t *testing.T) {
		result := callTool(t, session, "search_plugins", map[string]any{"query": strings.Repeat("a", 500)})
		if !result.IsError {
			t.Fatalf("expected IsError=true for an over-length query, got: %s", toolText(t, result))
		}
	})

	t.Run("dos_guard_plugin_batch_too_many_plugins", func(t *testing.T) {
		plugins := make([]string, 501)
		for i := range plugins {
			plugins[i] = "local_nonexistent"
		}
		result := callTool(t, session, "plugin_batch", map[string]any{"mode": "list", "plugins": plugins})
		if !result.IsError {
			t.Fatalf("expected IsError=true for a plugin_batch list with more than 500 plugins, got: %s", toolText(t, result))
		}
	})

	t.Run("invariant_3_single_text_block_and_iserror_semantics", func(t *testing.T) {
		result := callTool(t, session, "generate_plugin_context", map[string]any{"plugin_path": "local/does-not-exist"})
		if !result.IsError {
			t.Error("expected IsError=true for a nonexistent plugin path")
		}
		toolText(t, result) // fails the test unless there is exactly one text content block
	})

	t.Run("invariant_4_relative_paths_and_9_legacy_migration", func(t *testing.T) {
		legacyFiles := append(append([]string{}, generators.GlobalContextFilenames...), "tags")
		for _, f := range legacyFiles {
			if _, err := os.Stat(filepath.Join(root, f)); err != nil {
				t.Fatalf("precondition failed: legacy file %s missing before migration: %v", f, err)
			}
		}

		// update_indexes always runs the full generators.GenerateAll, which starts by migrating
		// the legacy root files.
		result := callTool(t, session, "update_indexes", nil)
		if result.IsError {
			t.Fatalf("expected success, got IsError: %s", toolText(t, result))
		}

		for _, f := range generators.GlobalContextFilenames {
			if _, err := os.Stat(filepath.Join(root, f)); err == nil {
				t.Errorf("invariant #9: expected legacy %s to be migrated away from the root", f)
			}
			if _, err := os.Stat(generators.GlobalOutputPath(root, f)); err != nil {
				t.Errorf("invariant #9: expected %s to exist under .build82/: %v", f, err)
			}
		}
		if _, err := os.Stat(filepath.Join(root, "tags")); err == nil {
			t.Error("invariant #9: expected legacy 'tags' to be migrated away from the root")
		}

		content, err := os.ReadFile(generators.GlobalOutputPath(root, "MOODLE_PLUGIN_INDEX.md"))
		if err != nil {
			t.Fatalf("read MOODLE_PLUGIN_INDEX.md: %v", err)
		}
		if strings.Contains(string(content), root) {
			t.Errorf("invariant #4: generated Markdown contains the absolute Moodle root path %q — paths must be root-relative", root)
		}
	})

	t.Run("invariant_7_component_first_underscore_split", func(t *testing.T) {
		result := callTool(t, session, "generate_plugin_context", map[string]any{
			"plugin_path": "mod/assign/submission/file", "format": "json",
		})
		if result.IsError {
			t.Fatalf("expected success, got IsError: %s", toolText(t, result))
		}
		var out struct {
			Component string `json:"component"`
			Type      string `json:"type"`
		}
		if err := json.Unmarshal([]byte(toolText(t, result)), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out.Component != "assignsubmission_file" {
			t.Errorf("expected component 'assignsubmission_file', got %q", out.Component)
		}
		if out.Type != "assignsubmission" {
			t.Errorf("expected type 'assignsubmission' (first underscore segment only, not 'assign'), got %q", out.Type)
		}
	})

	t.Run("invariant_8_never_abort_batch_on_one_bad_plugin", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the broken plugin relies on a read-only directory, which chmod cannot make on Windows")
		}
		result := callTool(t, session, "plugin_batch", map[string]any{"mode": "all", "format": "json"})
		if result.IsError {
			t.Fatalf("plugin_batch must not abort the whole batch — got IsError: %s", toolText(t, result))
		}

		var batch struct {
			Plugins []struct {
				Path      string
				Component string
				Failed    int
			}
		}
		if err := json.Unmarshal([]byte(toolText(t, result)), &batch); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		results := batch.Plugins
		if len(results) != 3 {
			t.Fatalf("expected 3 plugins (demo, assignsubmission_file, broken), got %d: %+v", len(results), results)
		}

		var failedCount, okCount int
		for _, r := range results {
			if r.Failed > 0 {
				failedCount++
				if r.Component != "local_broken" {
					t.Errorf("expected only local_broken to fail, but %s failed too", r.Component)
				}
			} else {
				okCount++
			}
		}
		if failedCount != 1 || okCount != 2 {
			t.Errorf("expected exactly 1 failed + 2 ok plugins, got %d failed + %d ok: %+v", failedCount, okCount, results)
		}

		// update_indexes include_plugins=true must show the same "one bad plugin doesn't abort
		// the run" behavior.
		updResult := callTool(t, session, "update_indexes", map[string]any{"include_plugins": true})
		if updResult.IsError {
			t.Fatalf("update_indexes must not abort — got IsError: %s", toolText(t, updResult))
		}
		// A failing plugin's line is marked with "⚠", not "✖".
		updText := toolText(t, updResult)
		if strings.Count(updText, "⚠ local_broken") != 1 {
			t.Errorf("expected exactly one failed-plugin line for local_broken, got:\n%s", updText)
		}
		if strings.Count(updText, "✔ ") != 2 {
			t.Errorf("expected exactly 2 successful plugin lines, got:\n%s", updText)
		}
	})

	// The text report includes cumulative cache statistics, so two runs are never byte-identical.
	// This subtest therefore compares the plugin order in the JSON response's Path field.
	t.Run("invariant_5_deterministic_ordering", func(t *testing.T) {
		fetchPaths := func() []string {
			result := callTool(t, session, "plugin_batch", map[string]any{"mode": "all", "format": "json"})
			var batch struct{ Plugins []struct{ Path string } }
			if err := json.Unmarshal([]byte(toolText(t, result)), &batch); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			results := batch.Plugins
			paths := make([]string, len(results))
			for i, r := range results {
				paths[i] = r.Path
			}
			return paths
		}

		first := fetchPaths()
		second := fetchPaths()

		if !sort.StringsAreSorted(first) {
			t.Errorf("expected plugin paths sorted ascending, got %v", first)
		}
		if strings.Join(first, "|") != strings.Join(second, "|") {
			t.Errorf("expected identical plugin ordering across repeated plugin_batch mode=all runs, got %v then %v", first, second)
		}
	})

	t.Run("invariant_2_persistent_cache_across_restart", func(t *testing.T) {
		// Warm the plugin-level cache; nothing is asserted on this call.
		callTool(t, session, "plugin_batch", map[string]any{"mode": "all"})

		// Simulate a process restart by replacing the global cache with an empty instance, so the
		// next run reloads the marks persisted in .build82/.cache.json.
		original := cache.Global
		cache.Global = cache.NewMtimeCache()
		defer func() { cache.Global = original }()

		result := callTool(t, session, "plugin_batch", map[string]any{"mode": "all", "format": "json"})
		if result.IsError {
			t.Fatalf("expected success after simulated restart, got IsError: %s", toolText(t, result))
		}
		var batch struct {
			Plugins []struct {
				Component          string
				Generated, Skipped int
			}
		}
		if err := json.Unmarshal([]byte(toolText(t, result)), &batch); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		results := batch.Plugins
		for _, r := range results {
			if r.Component == "local_broken" {
				continue // never produces successful output to cache
			}
			if r.Generated != 0 || r.Skipped == 0 {
				t.Errorf("expected %s to be a pure cache hit after the simulated restart, got %+v", r.Component, r)
			}
		}
	})
}
