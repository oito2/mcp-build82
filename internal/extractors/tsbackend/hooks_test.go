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

package tsbackend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hooksFixtureCallbacks is a well-formed db/hooks.php fixture.
const hooksFixtureCallbacks = `<?php
$callbacks = [
    [
        'hookname'       => '\\core\\hook\\output\\before_http_headers',
        'callback'       => '\\local_test\\hook_callbacks::before_headers',
        'priority'       => 500,
        'defaultenabled' => true,
    ],
];`

const hooksFixtureLegacyLib = `<?php
function local_test_before_footer() {
    // legacy callback
}
function local_test_some_other_function() {
    // not a known legacy callback
}
`

const hooksFixtureDefinition = `<?php
namespace local_test\hook;

/**
 * Fired when data is submitted.
 */
class data_submitted {
    public function get_hook_description(): string {
        return 'Fired when a user submits data to the plugin.';
    }

    public function get_hook_tags(): array {
        return ['data', 'submission'];
    }

    public static function get_replaces(): string {
        return '';
    }

    const DEPRECATED_CALLBACK = 'data_submitted_legacy';
}
`

// mustMkdirAll creates path and any missing parents, failing the test on error.
func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// mustWriteFile writes content to path, failing the test on error.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestExtractPluginHooks_Callbacks verifies db/hooks.php callbacks are parsed with their priority and default-enabled flag.
func TestExtractPluginHooks_Callbacks(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "hooks.php"), hooksFixtureCallbacks)

	result, err := ExtractPluginHooks(dir, "local_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Callbacks) != 1 {
		t.Fatalf("expected 1 callback, got %d", len(result.Callbacks))
	}
	if result.Callbacks[0].HookName != `\core\hook\output\before_http_headers` || result.Callbacks[0].Priority != 500 {
		t.Errorf("callback mismatch: %+v", result.Callbacks[0])
	}
	if !result.Callbacks[0].DefaultEnabled {
		t.Error("expected DefaultEnabled=true")
	}
}

// TestExtractPluginHooks_CallbackHookNameFallsBackToLegacyHookKey verifies the legacy "hook" key is used as the hook name when "hookname" is absent.
func TestExtractPluginHooks_CallbackHookNameFallsBackToLegacyHookKey(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	content := `<?php
$callbacks = [
    ['hook' => '\\core\\hook\\output\\before_footer', 'callback' => 'x::y'],
];`
	mustWriteFile(t, filepath.Join(dir, "db", "hooks.php"), content)

	result, err := ExtractPluginHooks(dir, "local_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Callbacks) != 1 || result.Callbacks[0].HookName != `\core\hook\output\before_footer` {
		t.Fatalf("expected fallback to the legacy 'hook' key, got %+v", result.Callbacks)
	}
	if !result.Callbacks[0].DefaultEnabled {
		t.Error("expected DefaultEnabled to default true when absent")
	}
}

// TestExtractPluginHooks_LegacyWarnings verifies legacy lib.php callbacks produce warnings naming their Hook API replacement.
func TestExtractPluginHooks_LegacyWarnings(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "lib.php"), hooksFixtureLegacyLib)

	result, err := ExtractPluginHooks(dir, "local_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.LegacyWarnings) != 1 {
		t.Fatalf("expected exactly 1 legacy warning, got %d: %+v", len(result.LegacyWarnings), result.LegacyWarnings)
	}
	w := result.LegacyWarnings[0]
	if w.LegacyFunction != "local_test_before_footer" {
		t.Errorf("legacy function mismatch: %q", w.LegacyFunction)
	}
	if !strings.Contains(w.ReplacedBy, "before_footer") {
		t.Errorf("expected ReplacedBy to reference before_footer, got %q", w.ReplacedBy)
	}
}

// TestExtractPluginHooks_MultipleLegacyWarningsAreSorted verifies that DetectLegacyCallbacks
// returns LegacyWarnings in a stable sorted order, even though it builds them from legacyhooks.Map
// (a Go map).
func TestExtractPluginHooks_MultipleLegacyWarningsAreSorted(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "lib.php"), "<?php\n"+
		"function local_test_cron() {}\n"+
		"function local_test_before_footer() {}\n")

	result, err := ExtractPluginHooks(dir, "local_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.LegacyWarnings) != 2 {
		t.Fatalf("expected 2 legacy warnings, got %d: %+v", len(result.LegacyWarnings), result.LegacyWarnings)
	}
	if result.LegacyWarnings[0].LegacyFunction != "local_test_before_footer" ||
		result.LegacyWarnings[1].LegacyFunction != "local_test_cron" {
		t.Errorf("expected LegacyWarnings sorted by LegacyFunction, got %+v", result.LegacyWarnings)
	}
}

// TestExtractPluginHooks_Definitions verifies hook definition classes are parsed into FQN, description, tags and replaced callback.
func TestExtractPluginHooks_Definitions(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "classes", "hook"))
	mustWriteFile(t, filepath.Join(dir, "classes", "hook", "data_submitted.php"), hooksFixtureDefinition)

	result, err := ExtractPluginHooks(dir, "local_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Definitions) != 1 {
		t.Fatalf("expected 1 hook definition, got %d: %+v", len(result.Definitions), result.Definitions)
	}
	def := result.Definitions[0]
	if def.ClassName != `\local_test\hook\data_submitted` {
		t.Errorf("ClassName mismatch: %q", def.ClassName)
	}
	if def.Description != "Fired when a user submits data to the plugin." {
		t.Errorf("Description mismatch: %q", def.Description)
	}
	if len(def.Tags) != 2 || def.Tags[0] != "data" || def.Tags[1] != "submission" {
		t.Errorf("Tags mismatch: %+v", def.Tags)
	}
	if def.Replaces != "data_submitted_legacy" {
		t.Errorf("Replaces mismatch: %q", def.Replaces)
	}
}

// TestExtractPluginHooks_DefinitionSkippedWithoutFQN verifies a hook definition file without a namespaced class is skipped.
func TestExtractPluginHooks_DefinitionSkippedWithoutFQN(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "classes", "hook"))
	mustWriteFile(t, filepath.Join(dir, "classes", "hook", "broken.php"), "<?php\nclass broken {}\n")

	result, err := ExtractPluginHooks(dir, "local_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Definitions) != 0 {
		t.Errorf("expected definitions without a resolvable FQN to be skipped, got %+v", result.Definitions)
	}
}

// TestExtractPluginHooks_DefinitionSkippedWithoutRealNamespace verifies that a hook definition
// file with no real namespace but a qualified reference elsewhere is skipped (no resolvable FQN)
// rather than resolved using that unrelated reference. A bare "namespace_name" node also occurs
// inside any qualified_name (e.g. an `implements \some\iface` clause), not just inside a real
// `namespace X;` declaration.
func TestExtractPluginHooks_DefinitionSkippedWithoutRealNamespace(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "classes", "hook"))
	mustWriteFile(t, filepath.Join(dir, "classes", "hook", "no_ns.php"), `<?php
class no_ns implements \some\iface {
}
`)
	result, err := ExtractPluginHooks(dir, "local_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Definitions) != 0 {
		t.Errorf("expected no definitions without a real namespace declaration, got %+v", result.Definitions)
	}
}

// TestParseHookCallbacks_MissingFile verifies a plugin without db/hooks.php yields no callbacks.
func TestParseHookCallbacks_MissingFile(t *testing.T) {
	if got := ParseHookCallbacks("/nonexistent/plugin"); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

// TestExtractPluginHooks_RealFileRegression runs the tree-sitter backend against every real
// plugin under the available Moodle installations; hooks.php/classes/hook aren't universal, so
// this exercises whatever real usage exists without requiring any.
func TestExtractPluginHooks_RealFileRegression(t *testing.T) {
	var checked int
	for _, name := range realMoodleRoots {
		root := filepath.Join("/srv/workspace/www/html/mdle", name)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Base(path) != "hooks.php" || filepath.Base(filepath.Dir(path)) != "db" {
				return nil
			}
			checked++
			pluginPath := filepath.Dir(filepath.Dir(path))
			if _, err := ExtractPluginHooks(pluginPath, "test_component"); err != nil {
				t.Errorf("%s: unexpected error: %v", pluginPath, err)
			}
			return nil
		})
	}
	if checked == 0 {
		t.Skip("no db/hooks.php files found across real Moodle installations")
	}
}
