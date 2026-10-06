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
	"path/filepath"
	"testing"
)

// malformedPhpFixtures are deliberately broken PHP snippets (an unclosed brace, an unterminated
// string and a non-UTF8 byte) for arbitrary files that are read without validation. Both backends
// must handle them without panicking; the shape of the result is not asserted.
var malformedPhpFixtures = map[string]string{
	"unclosed_brace":      "<?php\n$observers = [\n    ['eventname' => '\\core\\event\\x', 'callback' => 'y::z'\n",
	"unterminated_string": "<?php\nfunction xmldb_local_test_upgrade($oldversion) {\n    if ($oldversion < 2024010100) {\n        $s = 'unterminated\n    }\n",
	"non_utf8_byte":       "<?php\nclass local_test_\xffclass {}\n",
}

// extractorBackends lists the values of BUILD82_EXTRACTOR_BACKEND selecting the regex backend
// (empty) and the tree-sitter backend.
var extractorBackends = []string{"", "treesitter"}

// TestParseEventsPhp_MalformedInputDoesNotPanic verifies that malformed input does not panic ParseEventsPhp on either backend.
func TestParseEventsPhp_MalformedInputDoesNotPanic(t *testing.T) {
	for name, content := range malformedPhpFixtures {
		for _, backend := range extractorBackends {
			t.Run(name+"/"+backendLabel(backend), func(t *testing.T) {
				t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
				dir := t.TempDir()
				mustMkdirAll(t, filepath.Join(dir, "db"))
				path := filepath.Join(dir, "db", "events.php")
				mustWriteFile(t, path, content)

				result := ParseEventsPhp(path) // must not panic; nil/empty result is fine
				_ = result
			})
		}
	}
}

// TestParseUpgradePhp_MalformedInputDoesNotPanic verifies that malformed input does not panic ParseUpgradePhp on either backend.
func TestParseUpgradePhp_MalformedInputDoesNotPanic(t *testing.T) {
	for name, content := range malformedPhpFixtures {
		for _, backend := range extractorBackends {
			t.Run(name+"/"+backendLabel(backend), func(t *testing.T) {
				t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
				dir := t.TempDir()
				mustMkdirAll(t, filepath.Join(dir, "db"))
				path := filepath.Join(dir, "db", "upgrade.php")
				mustWriteFile(t, path, content)

				result := ParseUpgradePhp(path) // must not panic; nil/empty result is fine
				_ = result
			})
		}
	}
}

// TestExtractClasses_MalformedInputDoesNotPanic verifies that malformed input does not panic ExtractClasses on either backend.
func TestExtractClasses_MalformedInputDoesNotPanic(t *testing.T) {
	for name, content := range malformedPhpFixtures {
		for _, backend := range extractorBackends {
			t.Run(name+"/"+backendLabel(backend), func(t *testing.T) {
				t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
				dir := t.TempDir()
				mustMkdirAll(t, filepath.Join(dir, "classes"))
				path := filepath.Join(dir, "classes", "broken.php")
				mustWriteFile(t, path, content)

				result := ExtractClasses(dir, dir, "**/*.php") // must not panic
				_ = result
			})
		}
	}
}

// backendLabel returns a subtest label for the BUILD82_EXTRACTOR_BACKEND value `backend`.
func backendLabel(backend string) string {
	if backend == "" {
		return "regex"
	}
	return backend
}

// TestParseAccessPhp_MalformedInputDoesNotPanic verifies that malformed input does not panic ParseAccessPhp on either backend.
func TestParseAccessPhp_MalformedInputDoesNotPanic(t *testing.T) {
	for name, content := range malformedPhpFixtures {
		for _, backend := range extractorBackends {
			t.Run(name+"/"+backendLabel(backend), func(t *testing.T) {
				t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
				dir := t.TempDir()
				mustMkdirAll(t, filepath.Join(dir, "db"))
				path := filepath.Join(dir, "db", "access.php")
				mustWriteFile(t, path, content)

				result := ParseAccessPhp(path) // must not panic; nil/empty result is fine
				_ = result
			})
		}
	}
}

// TestParseServicesPhp_MalformedInputDoesNotPanic verifies that malformed input does not panic ParseServicesPhp on either backend.
func TestParseServicesPhp_MalformedInputDoesNotPanic(t *testing.T) {
	for name, content := range malformedPhpFixtures {
		for _, backend := range extractorBackends {
			t.Run(name+"/"+backendLabel(backend), func(t *testing.T) {
				t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
				dir := t.TempDir()
				mustMkdirAll(t, filepath.Join(dir, "db"))
				path := filepath.Join(dir, "db", "services.php")
				mustWriteFile(t, path, content)

				result := ParseServicesPhp(path) // must not panic; nil/empty result is fine
				_ = result
			})
		}
	}
}

// TestParseTasksPhp_MalformedInputDoesNotPanic verifies that malformed input does not panic ParseTasksPhp on either backend.
func TestParseTasksPhp_MalformedInputDoesNotPanic(t *testing.T) {
	for name, content := range malformedPhpFixtures {
		for _, backend := range extractorBackends {
			t.Run(name+"/"+backendLabel(backend), func(t *testing.T) {
				t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
				dir := t.TempDir()
				mustMkdirAll(t, filepath.Join(dir, "db"))
				path := filepath.Join(dir, "db", "tasks.php")
				mustWriteFile(t, path, content)

				result := ParseTasksPhp(path) // must not panic; nil/empty result is fine
				_ = result
			})
		}
	}
}

// TestParseSettingsPhp_MalformedInputDoesNotPanic verifies that malformed input does not panic
// ParseSettingsPhp. It does not loop over backends because ParseSettingsPhp always uses a regex
// scan.
func TestParseSettingsPhp_MalformedInputDoesNotPanic(t *testing.T) {
	for name, content := range malformedPhpFixtures {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "settings.php")
			mustWriteFile(t, path, content)

			result := ParseSettingsPhp(path) // must not panic; nil/empty result is fine
			_ = result
		})
	}
}

// TestExtractPluginHooks_MalformedInputDoesNotPanic verifies that malformed input in db/hooks.php,
// classes/hook/*.php and lib.php does not panic ExtractPluginHooks on either backend.
func TestExtractPluginHooks_MalformedInputDoesNotPanic(t *testing.T) {
	for name, content := range malformedPhpFixtures {
		for _, backend := range extractorBackends {
			t.Run("callbacks/"+name+"/"+backendLabel(backend), func(t *testing.T) {
				t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
				dir := t.TempDir()
				mustMkdirAll(t, filepath.Join(dir, "db"))
				mustWriteFile(t, filepath.Join(dir, "db", "hooks.php"), content)

				result, err := ExtractPluginHooks(dir, "local_test") // must not panic
				_ = result
				_ = err
			})

			t.Run("definitions/"+name+"/"+backendLabel(backend), func(t *testing.T) {
				t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
				dir := t.TempDir()
				mustMkdirAll(t, filepath.Join(dir, "classes", "hook"))
				mustWriteFile(t, filepath.Join(dir, "classes", "hook", "broken.php"), content)

				result, err := ExtractPluginHooks(dir, "local_test") // must not panic
				_ = result
				_ = err
			})

			t.Run("legacylib/"+name+"/"+backendLabel(backend), func(t *testing.T) {
				t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
				dir := t.TempDir()
				mustWriteFile(t, filepath.Join(dir, "lib.php"), content)

				result, err := ExtractPluginHooks(dir, "local_test") // must not panic
				_ = result
				_ = err
			})
		}
	}
}

// malformedSubpluginsJSONFixtures are deliberately broken db/subplugins.json contents, kept
// separate from the PHP fixtures because that file is parsed as JSON.
var malformedSubpluginsJSONFixtures = map[string]string{
	"empty_file":           "",
	"truncated_object":     `{"plugintypes": {"workshopform": "mod/workshop/form"`,
	"wrong_top_level_type": `["not", "an", "object"]`,
	"wrong_value_type":     `{"plugintypes": {"workshopform": 123}}`,
	"non_json_garbage":     "<?php not even json {{{",
}

// TestExtractSubplugins_MalformedJsonDoesNotPanic verifies that malformed db/subplugins.json content does not panic ExtractSubplugins.
func TestExtractSubplugins_MalformedJsonDoesNotPanic(t *testing.T) {
	for name, content := range malformedSubpluginsJSONFixtures {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			mustMkdirAll(t, filepath.Join(dir, "db"))
			mustWriteFile(t, filepath.Join(dir, "db", "subplugins.json"), content)

			result := ExtractSubplugins(dir) // must not panic; nil/partial result is fine
			_ = result
		})
	}
}

// TestExtractSubplugins_MalformedLegacyPhpDoesNotPanic verifies that malformed db/subplugins.php
// content, read when db/subplugins.json is absent, does not panic ExtractSubplugins.
func TestExtractSubplugins_MalformedLegacyPhpDoesNotPanic(t *testing.T) {
	for name, content := range malformedPhpFixtures {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			mustMkdirAll(t, filepath.Join(dir, "db"))
			mustWriteFile(t, filepath.Join(dir, "db", "subplugins.php"), content)

			result := ExtractSubplugins(dir) // must not panic; nil/partial result is fine
			_ = result
		})
	}
}
