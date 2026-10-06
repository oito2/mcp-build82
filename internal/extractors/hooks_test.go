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

package extractors

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hooksFixtureCallbacks is a db/hooks.php with one callback.
const hooksFixtureCallbacks = `<?php
$callbacks = [
    [
        'hookname'       => '\\core\\hook\\output\\before_http_headers',
        'callback'       => '\\local_test\\hook_callbacks::before_headers',
        'priority'       => 500,
        'defaultenabled' => true,
    ],
];`

// hooksFixtureLegacyLib is a lib.php with one legacy hook callback and one unrelated function.
const hooksFixtureLegacyLib = `<?php
function local_test_before_footer() {
    // legacy callback
}
function local_test_some_other_function() {
    // not a known legacy callback
}
`

// TestExtractPluginHooks_Callbacks verifies the hook name, priority and enabled flag of a registered callback.
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

// TestExtractPluginHooks_CallbackHookNameFallsBackToLegacyHookKey verifies that the "hook" key is used when "hookname" is absent and that
// defaultenabled defaults to true.
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

// TestExtractPluginHooks_LegacyWarnings verifies that a declared legacy callback produces one warning naming its replacement.
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

// TestExtractPluginHooks_MultipleLegacyWarningsAreSorted verifies that several legacy callbacks
// produce warnings sorted by function name, although they are derived from a map.
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

// TestExtractPluginHooks_Definitions verifies the class name, description, tags and replaced callback of a hook definition.
func TestExtractPluginHooks_Definitions(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "classes", "hook"))
	content := `<?php
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
	mustWriteFile(t, filepath.Join(dir, "classes", "hook", "data_submitted.php"), content)

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

// TestExtractPluginHooks_DefinitionSkippedWithoutFQN verifies that a hook class without a resolvable FQN is skipped.
func TestExtractPluginHooks_DefinitionSkippedWithoutFQN(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "classes", "hook"))
	// Without a namespace declaration there is no FQN, so the class must be skipped.
	mustWriteFile(t, filepath.Join(dir, "classes", "hook", "broken.php"), "<?php\nclass broken {}\n")

	result, err := ExtractPluginHooks(dir, "local_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Definitions) != 0 {
		t.Errorf("expected definitions without a resolvable FQN to be skipped, got %+v", result.Definitions)
	}
}

// TestExtractPluginHooks_TreesitterBackendParity verifies that both backends return the same
// callbacks, definitions and legacy warnings for well-formed fixtures.
func TestExtractPluginHooks_TreesitterBackendParity(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustMkdirAll(t, filepath.Join(dir, "classes", "hook"))
	mustWriteFile(t, filepath.Join(dir, "db", "hooks.php"), hooksFixtureCallbacks)
	mustWriteFile(t, filepath.Join(dir, "classes", "hook", "data_submitted.php"), `<?php
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

    const DEPRECATED_CALLBACK = 'data_submitted_legacy';
}
`)
	mustWriteFile(t, filepath.Join(dir, "lib.php"), hooksFixtureLegacyLib)

	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
	regexResult, err := ExtractPluginHooks(dir, "local_test")
	if err != nil {
		t.Fatalf("regex backend error: %v", err)
	}
	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
	tsResult, err := ExtractPluginHooks(dir, "local_test")
	if err != nil {
		t.Fatalf("treesitter backend error: %v", err)
	}

	if len(regexResult.Callbacks) != len(tsResult.Callbacks) || regexResult.Callbacks[0] != tsResult.Callbacks[0] {
		t.Errorf("callbacks mismatch: regex=%+v treesitter=%+v", regexResult.Callbacks, tsResult.Callbacks)
	}
	if len(regexResult.Definitions) != 1 || len(tsResult.Definitions) != 1 {
		t.Fatalf("expected 1 definition each: regex=%+v treesitter=%+v", regexResult.Definitions, tsResult.Definitions)
	}
	rd, td := regexResult.Definitions[0], tsResult.Definitions[0]
	if rd.ClassName != td.ClassName || rd.Description != td.Description || rd.Replaces != td.Replaces {
		t.Errorf("definition mismatch: regex=%+v treesitter=%+v", rd, td)
	}
	if len(rd.Tags) != len(td.Tags) {
		t.Errorf("tags length mismatch: regex=%+v treesitter=%+v", rd.Tags, td.Tags)
	} else {
		for i := range rd.Tags {
			if rd.Tags[i] != td.Tags[i] {
				t.Errorf("tag %d mismatch: regex=%q treesitter=%q", i, rd.Tags[i], td.Tags[i])
			}
		}
	}
	if len(regexResult.LegacyWarnings) != len(tsResult.LegacyWarnings) || regexResult.LegacyWarnings[0] != tsResult.LegacyWarnings[0] {
		t.Errorf("legacy warnings mismatch: regex=%+v treesitter=%+v", regexResult.LegacyWarnings, tsResult.LegacyWarnings)
	}
}

// TestExtractPluginHooks_TreesitterBackendParity_RealFiles verifies that both backends agree on
// every plugin with a db/hooks.php in the available Moodle installations. The test is skipped when
// none is found.
func TestExtractPluginHooks_TreesitterBackendParity_RealFiles(t *testing.T) {
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

			t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
			regexResult, rErr := ExtractPluginHooks(pluginPath, "test_component")
			t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
			tsResult, tErr := ExtractPluginHooks(pluginPath, "test_component")
			if rErr != nil || tErr != nil {
				t.Errorf("%s: regex err=%v treesitter err=%v", pluginPath, rErr, tErr)
				return nil
			}

			if len(regexResult.Callbacks) != len(tsResult.Callbacks) {
				t.Errorf("%s: callback count mismatch: regex=%d treesitter=%d", pluginPath, len(regexResult.Callbacks), len(tsResult.Callbacks))
			} else {
				for i := range regexResult.Callbacks {
					if regexResult.Callbacks[i] != tsResult.Callbacks[i] {
						t.Errorf("%s: callback %d mismatch: regex=%+v treesitter=%+v", pluginPath, i, regexResult.Callbacks[i], tsResult.Callbacks[i])
					}
				}
			}
			if len(regexResult.LegacyWarnings) != len(tsResult.LegacyWarnings) {
				t.Errorf("%s: legacy warning count mismatch: regex=%d treesitter=%d", pluginPath, len(regexResult.LegacyWarnings), len(tsResult.LegacyWarnings))
			}
			return nil
		})
	}
	if checked == 0 {
		t.Skip("no db/hooks.php files found across real Moodle installations")
	}
}

// TestExtractPluginHooks_DescriptionAndTagsDoNotLeakFromLaterMethod verifies that when
// get_hook_description() and get_hook_tags() return computed values, the search for a literal
// `return` stays within each method body and does not pick up a later method's return value. The
// description then falls back to the class docblock summary and the tags stay empty.
func TestExtractPluginHooks_DescriptionAndTagsDoNotLeakFromLaterMethod(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "classes", "hook"))
	content := `<?php
namespace local_test\hook;

/**
 * Real docblock description for the class.
 */
class computed_hook {
    public function get_hook_description(): string {
        $prefix = 'Computed: ';
        return $prefix . get_config('local_test', 'label');
    }

    public function get_hook_tags(): array {
        $tags = ['x'];
        return $tags;
    }

    public static function get_replaces(): string {
        return 'leaked_replaces_value';
    }

    public static function other_helper(): array {
        return ['leaked', 'tags'];
    }
}
`
	mustWriteFile(t, filepath.Join(dir, "classes", "hook", "computed_hook.php"), content)

	result, err := ExtractPluginHooks(dir, "local_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Definitions) != 1 {
		t.Fatalf("expected 1 hook definition, got %d: %+v", len(result.Definitions), result.Definitions)
	}
	def := result.Definitions[0]
	if def.Description != "Real docblock description for the class." {
		t.Errorf("expected the class's own docblock summary (get_hook_description has no literal return), got %q", def.Description)
	}
	if len(def.Tags) != 0 {
		t.Errorf("expected no tags (get_hook_tags has no literal array return), got %+v", def.Tags)
	}
}

// TestExtractPluginHooks_DocblockPicksClassSummaryNotFileHeader verifies that the description comes
// from the docblock directly above the class, not from a file-header docblock before it.
func TestExtractPluginHooks_DocblockPicksClassSummaryNotFileHeader(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "classes", "hook"))
	content := `<?php
/**
 * This file is part of Moodle - http://moodle.org/
 *
 * @package    local_test
 * @copyright  2026 someone
 */

namespace local_test\hook;

/**
 * The real class summary that should be extracted.
 */
class no_literal_return {
    public function get_hook_description(): string {
        $computed = 'a' . 'b';
        return $computed;
    }
}
`
	mustWriteFile(t, filepath.Join(dir, "classes", "hook", "no_literal_return.php"), content)

	result, err := ExtractPluginHooks(dir, "local_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Definitions) != 1 {
		t.Fatalf("expected 1 hook definition, got %d: %+v", len(result.Definitions), result.Definitions)
	}
	if got := result.Definitions[0].Description; got != "The real class summary that should be extracted." {
		t.Errorf("expected the class's own docblock summary, not the file header, got %q", got)
	}
}

// TestPluginUsesHookApi verifies that only callbacks and definitions, not legacy warnings, count as Hook API
// usage.
func TestPluginUsesHookApi(t *testing.T) {
	if PluginUsesHookApi(HooksExtraction{LegacyWarnings: []LegacyCallbackWarning{{LegacyFunction: "x"}}}) {
		t.Error("expected legacy warnings alone to not count as using the Hook API")
	}
	if !PluginUsesHookApi(HooksExtraction{Callbacks: []HookCallback{{HookName: "x"}}}) {
		t.Error("expected a registered callback to count as using the Hook API")
	}
	if !PluginUsesHookApi(HooksExtraction{Definitions: []HookDefinition{{ClassName: "x"}}}) {
		t.Error("expected a hook definition to count as using the Hook API")
	}
}
