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

package extractors

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// TestExtractPluginHooks_MultipleLegacyWarningsAreSorted verifies that a plugin with 2+ legacy
// callbacks gets its LegacyWarnings in a stable sorted order across runs, even though
// detectLegacyCallbacks builds them from legacyhooks.Map (a Go map). "cron" and "before_footer"
// are both entries in legacy_hooks.json.
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

func TestExtractPluginHooks_DefinitionSkippedWithoutFQN(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "classes", "hook"))
	// No namespace declaration -> no resolvable FQN -> must be skipped.
	mustWriteFile(t, filepath.Join(dir, "classes", "hook", "broken.php"), "<?php\nclass broken {}\n")

	result, err := ExtractPluginHooks(dir, "local_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Definitions) != 0 {
		t.Errorf("expected definitions without a resolvable FQN to be skipped, got %+v", result.Definitions)
	}
}

// TestExtractPluginHooks_TreesitterBackendParity confirms BUILD82_EXTRACTOR_BACKEND=treesitter
// produces identical output to the regex backend across all three sub-scans for well-formed
// fixtures (callbacks, hook-definition class, legacy lib.php).
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

// TestExtractPluginHooks_TreesitterBackendParity_RealFiles runs both backends against every real
// plugin with a db/hooks.php across all 4 real Moodle installations and confirms they agree.
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
// get_hook_description()/get_hook_tags() computes a value instead of returning a literal directly,
// the return-literal search stays scoped to that method's own brace-balanced body (via
// phparray.FindBalancedEnd) and does not capture the first literal `return` found later in the
// file — here, get_replaces()'s and other_helper()'s unrelated return values. Description falls back
// to the class's own docblock summary, and Tags stays empty.
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

// TestExtractPluginHooks_DocblockPicksClassSummaryNotFileHeader verifies that a file-header
// docblock (license/@package) placed before the class's own docblock is not used as the summary:
// the docblock search is anchored to the class declaration line and scans backward.
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
