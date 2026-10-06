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
	"unicode/utf8"
)

// upgradeFixtureWellFormed is a well-formed db/upgrade.php fixture.
const upgradeFixtureWellFormed = `<?php
function xmldb_local_test_upgrade($oldversion) {
    global $DB;

    if ($oldversion < 2024020100) { // Add index for performance
        $table = new xmldb_table('local_test_widgets');
        upgrade_plugin_savepoint(true, 2024020100, 'local', 'test');
    }

    if ($oldversion < 2023010100) {
        $table = new xmldb_table('local_test_legacy');
        upgrade_plugin_savepoint(true, 2023010100, 'local', 'test');
    }

    if ($oldversion < 2024010100) {
        // Add a new table for widgets
        $table = new xmldb_table('local_test_widgets');
        upgrade_plugin_savepoint(true, 2024010100, 'local', 'test');
    }

    return true;
}
`

// writeUpgradePhp writes content to a temporary db/upgrade.php and returns its path.
func writeUpgradePhp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "upgrade.php")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestParseUpgradePhp_MatchesRegexBackendFixture verifies upgrade steps and their descriptions for a well-formed upgrade file match the regex backend's output.
func TestParseUpgradePhp_MatchesRegexBackendFixture(t *testing.T) {
	path := writeUpgradePhp(t, upgradeFixtureWellFormed)
	result := ParseUpgradePhp(path)
	if result == nil || len(result.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %+v", result)
	}

	if result.Steps[0].Version != "2023010100" || result.Steps[1].Version != "2024010100" || result.Steps[2].Version != "2024020100" {
		t.Fatalf("expected ascending sort, got %+v", result.Steps)
	}

	// Step at 2024010100: comment is on the line *after* the if — rule 1 must NOT match; falls
	// through to rule 2 (xmldb_table reference).
	step2 := result.Steps[1]
	if step2.Description != "xmldb_table: local_test_widgets" {
		t.Errorf("expected fallback to xmldb_table reference, got %q", step2.Description)
	}

	// Step at 2024020100: comment IS on the if line itself — rule 1 must match.
	step3 := result.Steps[2]
	if step3.Description != "Add index for performance" {
		t.Errorf("expected inline if-line comment, got %q", step3.Description)
	}
}

// TestParseUpgradePhp_MissingFile verifies a nonexistent file yields nil.
func TestParseUpgradePhp_MissingFile(t *testing.T) {
	if ParseUpgradePhp("/nonexistent/db/upgrade.php") != nil {
		t.Error("expected nil for missing file")
	}
}

// TestParseUpgradePhp_NoUpgradeFunction verifies a file without an xmldb_*_upgrade function yields an empty, non-nil step list.
func TestParseUpgradePhp_NoUpgradeFunction(t *testing.T) {
	path := writeUpgradePhp(t, "<?php\n// nothing here\n")
	result := ParseUpgradePhp(path)
	if result == nil || len(result.Steps) != 0 {
		t.Fatalf("expected empty steps when no upgrade function is found, got %+v", result)
	}
}

// TestParseUpgradePhp_NoStepsInFunction verifies an upgrade function without version-gated blocks yields an empty step list.
func TestParseUpgradePhp_NoStepsInFunction(t *testing.T) {
	path := writeUpgradePhp(t, "<?php\nfunction xmldb_local_test_upgrade($oldversion) {\n    return true;\n}\n")
	result := ParseUpgradePhp(path)
	if result == nil || len(result.Steps) != 0 {
		t.Fatalf("expected 0 steps, got %+v", result)
	}
}

// TestParseUpgradePhp_RequiresExactlyTenDigits verifies that a version number that isn't exactly
// 10 digits is not treated as a step, like the regex backend's `\d{10}` restriction.
func TestParseUpgradePhp_RequiresExactlyTenDigits(t *testing.T) {
	path := writeUpgradePhp(t, "<?php\nfunction xmldb_local_test_upgrade($oldversion) {\n    if ($oldversion < 123) {\n        return true;\n    }\n}\n")
	result := ParseUpgradePhp(path)
	if result == nil || len(result.Steps) != 0 {
		t.Fatalf("expected a non-10-digit version to be ignored, got %+v", result)
	}
}

// TestParseUpgradePhp_XmldbTableSkipsVariableArgument verifies that when a step block has an
// earlier `new xmldb_table($variable)` call (variable argument, no useful name) before a later
// `new xmldb_table('literal_name')`, the search skips the non-literal call and keeps looking.
func TestParseUpgradePhp_XmldbTableSkipsVariableArgument(t *testing.T) {
	path := writeUpgradePhp(t, `<?php
function xmldb_local_test_upgrade($oldversion) {
    if ($oldversion < 2024010100) {
        $oldname = 'legacy_table';
        $oldtable = new xmldb_table($oldname);
        $table = new xmldb_table('real_table_name');
    }
}
`)
	result := ParseUpgradePhp(path)
	if result == nil || len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %+v", result)
	}
	if got := result.Steps[0].Description; got != "xmldb_table: real_table_name" {
		t.Errorf("expected the search to skip the variable-argument call and find the literal one, got %q", got)
	}
}

// TestParseUpgradePhp_RealFileRegression runs the tree-sitter backend against every real
// db/upgrade.php across all 4 real Moodle installations.
func TestParseUpgradePhp_RealFileRegression(t *testing.T) {
	var found int
	for _, name := range realMoodleRoots {
		root := filepath.Join("/srv/workspace/www/html/mdle", name)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Base(path) != "upgrade.php" || filepath.Base(filepath.Dir(path)) != "db" {
				return nil
			}
			found++
			if result := ParseUpgradePhp(path); result == nil {
				t.Errorf("%s: expected a non-nil result", path)
			}
			return nil
		})
	}
	if found == 0 {
		t.Skip("no db/upgrade.php files found across real Moodle installations")
	}
}

func TestTruncate120_RuneSafe(t *testing.T) {
	in := strings.Repeat("a", 119) + "é" + "tail"
	got := truncate120(in)
	if !utf8.ValidString(got) || len(got) > 120 {
		t.Fatalf("truncate120 = %q (len %d), want valid UTF-8 within 120 bytes", got, len(got))
	}
	if got != strings.Repeat("a", 119) {
		t.Errorf("truncate120 = %q, want the 119-byte prefix", got)
	}
}

func TestParseUpgradePhp_EqualVersionsKeepSourceOrder(t *testing.T) {
	path := writeUpgradePhp(t, `<?php
function xmldb_local_test_upgrade($oldversion) {
    if ($oldversion < 2024010100) { // first
    }
    if ($oldversion < 2024010100) { // second
    }
    return true;
}
`)
	got := ParseUpgradePhp(path)
	if len(got.Steps) != 2 || got.Steps[0].Description != "first" || got.Steps[1].Description != "second" {
		t.Errorf("steps = %+v, want source order first, second", got.Steps)
	}
}
