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
	"testing"
)

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

func TestParseUpgradePhp(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "upgrade.php"), upgradeFixtureWellFormed)

	result := ParseUpgradePhp(filepath.Join(dir, "db", "upgrade.php"))
	if result == nil || len(result.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %+v", result)
	}

	// Ascending numeric sort, regardless of source order.
	if result.Steps[0].Version != "2023010100" || result.Steps[1].Version != "2024010100" || result.Steps[2].Version != "2024020100" {
		versions := GetUpgradeVersions(result)
		t.Fatalf("expected ascending sort, got %v", versions)
	}

	// Step at 2024010100: comment is on the line *after* the if — rule 1 (inline comment on the
	// if line itself) must NOT match; falls through to rule 2 (xmldb_table reference).
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

func TestParseUpgradePhp_MissingFile(t *testing.T) {
	if ParseUpgradePhp("/nonexistent/db/upgrade.php") != nil {
		t.Error("expected nil for missing file")
	}
}

func TestParseUpgradePhp_NoUpgradeFunction(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "upgrade.php"), "<?php\n// nothing here\n")

	result := ParseUpgradePhp(filepath.Join(dir, "db", "upgrade.php"))
	if result == nil || len(result.Steps) != 0 {
		t.Fatalf("expected empty steps when no upgrade function is found, got %+v", result)
	}
}

func TestParseUpgradePhp_NoStepsInFunction(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "upgrade.php"), "<?php\nfunction xmldb_local_test_upgrade($oldversion) {\n    return true;\n}\n")

	result := ParseUpgradePhp(filepath.Join(dir, "db", "upgrade.php"))
	if result == nil || len(result.Steps) != 0 {
		t.Fatalf("expected 0 steps, got %+v", result)
	}
}

// TestParseUpgradePhp_TreesitterBackendParity confirms BUILD82_EXTRACTOR_BACKEND=treesitter
// produces identical output to the regex backend for the exact fixture, including both
// description-fallback tiers.
func TestParseUpgradePhp_TreesitterBackendParity(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "upgrade.php"), upgradeFixtureWellFormed)
	path := filepath.Join(dir, "db", "upgrade.php")

	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
	regexResult := ParseUpgradePhp(path)
	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
	tsResult := ParseUpgradePhp(path)

	if len(regexResult.Steps) != len(tsResult.Steps) {
		t.Fatalf("step count mismatch: regex=%d treesitter=%d", len(regexResult.Steps), len(tsResult.Steps))
	}
	for i := range regexResult.Steps {
		if regexResult.Steps[i] != tsResult.Steps[i] {
			t.Errorf("step %d mismatch: regex=%+v treesitter=%+v", i, regexResult.Steps[i], tsResult.Steps[i])
		}
	}
}

// TestParseUpgradePhp_TreesitterBackendParity_RealFiles runs both backends against every real
// db/upgrade.php across all 4 real Moodle installations and confirms they agree.
func TestParseUpgradePhp_TreesitterBackendParity_RealFiles(t *testing.T) {
	var checked int
	for _, name := range realMoodleRoots {
		root := filepath.Join("/srv/workspace/www/html/mdle", name)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Base(path) != "upgrade.php" || filepath.Base(filepath.Dir(path)) != "db" {
				return nil
			}
			checked++

			t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
			regexResult := ParseUpgradePhp(path)
			t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
			tsResult := ParseUpgradePhp(path)

			if len(regexResult.Steps) != len(tsResult.Steps) {
				t.Errorf("%s: step count mismatch: regex=%d treesitter=%d", path, len(regexResult.Steps), len(tsResult.Steps))
				return nil
			}
			for i := range regexResult.Steps {
				if regexResult.Steps[i] != tsResult.Steps[i] {
					t.Errorf("%s: step %d mismatch: regex=%+v treesitter=%+v", path, i, regexResult.Steps[i], tsResult.Steps[i])
				}
			}
			return nil
		})
	}
	if checked == 0 {
		t.Skip("no db/upgrade.php files found across real Moodle installations")
	}
}

// TestGetUpgradeVersions_NilInputDoesNotPanic confirms GetUpgradeVersions handles a nil
// *UpgradeExtraction (a plugin with no db/upgrade.php, the common case) by returning nil instead of
// panicking on a nil pointer dereference.
func TestGetUpgradeVersions_NilInputDoesNotPanic(t *testing.T) {
	if got := GetUpgradeVersions(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestGetUpgradeVersions_NoReSort(t *testing.T) {
	e := &UpgradeExtraction{Steps: []UpgradeStep{{Version: "2024010100"}, {Version: "2023010100"}}}
	versions := GetUpgradeVersions(e)
	// Preserves whatever order Steps is already in — does not re-sort.
	if versions[0] != "2024010100" || versions[1] != "2023010100" {
		t.Errorf("expected GetUpgradeVersions to preserve Steps order, got %v", versions)
	}
}
