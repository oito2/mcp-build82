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
	"slices"
	"strconv"
	"testing"
)

// upgradeFixtureWellFormed is an upgrade.php with steps out of order, covering each description
// source.
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

// TestParseUpgradePhp verifies that steps are sorted by version and described from an inline comment or an
// xmldb_table reference.
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

// TestParseUpgradePhp_MissingFile verifies that a missing file yields nil.
func TestParseUpgradePhp_MissingFile(t *testing.T) {
	if ParseUpgradePhp("/nonexistent/db/upgrade.php") != nil {
		t.Error("expected nil for missing file")
	}
}

// TestParseUpgradePhp_NoUpgradeFunction verifies that a file without the upgrade function yields an empty extraction.
func TestParseUpgradePhp_NoUpgradeFunction(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "upgrade.php"), "<?php\n// nothing here\n")

	result := ParseUpgradePhp(filepath.Join(dir, "db", "upgrade.php"))
	if result == nil || len(result.Steps) != 0 {
		t.Fatalf("expected empty steps when no upgrade function is found, got %+v", result)
	}
}

// TestParseUpgradePhp_NoStepsInFunction verifies that an upgrade function without steps yields an empty extraction.
func TestParseUpgradePhp_NoStepsInFunction(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "upgrade.php"), "<?php\nfunction xmldb_local_test_upgrade($oldversion) {\n    return true;\n}\n")

	result := ParseUpgradePhp(filepath.Join(dir, "db", "upgrade.php"))
	if result == nil || len(result.Steps) != 0 {
		t.Fatalf("expected 0 steps, got %+v", result)
	}
}

// TestParseUpgradePhp_TreesitterBackendParity verifies that both backends return identical steps
// and descriptions for the fixture.
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

// TestParseUpgradePhp_TreesitterBackendParity_RealFiles verifies that both backends agree on every
// db/upgrade.php of the available Moodle installations. The test is skipped when none is found.
func TestParseUpgradePhp_TreesitterBackendParity_RealFiles(t *testing.T) {
	var checked int
	for _, name := range realMoodleRoots {
		root := filepath.Join(realMoodleDir(), name)
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

// TestGetUpgradeVersions_NilInputDoesNotPanic verifies that a nil extraction yields nil.
func TestGetUpgradeVersions_NilInputDoesNotPanic(t *testing.T) {
	if got := GetUpgradeVersions(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// TestGetUpgradeVersions_NoReSort verifies that versions are returned in the order of the steps.
func TestGetUpgradeVersions_NoReSort(t *testing.T) {
	e := &UpgradeExtraction{Steps: []UpgradeStep{{Version: "2024010100"}, {Version: "2023010100"}}}
	versions := GetUpgradeVersions(e)
	// The existing order of Steps must be preserved.
	if versions[0] != "2024010100" || versions[1] != "2023010100" {
		t.Errorf("expected GetUpgradeVersions to preserve Steps order, got %v", versions)
	}
}

// TestParseUpgradePhp_EqualVersionsKeepSourceOrder verifies that steps with the same version keep
// their source order after sorting.
func TestParseUpgradePhp_EqualVersionsKeepSourceOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "upgrade.php")
	body := "<?php\nfunction xmldb_local_test_upgrade($oldversion) {\n"
	vers := []string{"2024010102", "2024010101", "2024010100"}
	var all []string
	for i := 0; i < 60; i++ {
		d := "step" + strconv.Itoa(i)
		all = append(all, d)
		body += "    if ($oldversion < " + vers[i%3] + ") {\n        // " + d + "\n    }\n"
	}
	body += "    return true;\n}\n"
	mustWriteFile(t, path, body)

	got := ParseUpgradePhp(path)
	// Expected order: grouped by version ascending, source order inside each group.
	var want []string
	for _, r := range []int{2, 1, 0} {
		for i, d := range all {
			if i%3 == r {
				want = append(want, d)
			}
		}
	}
	if got == nil || len(got.Steps) != len(want) {
		t.Fatalf("unexpected extraction: %+v", got)
	}
	for i, w := range want {
		if got.Steps[i].Description != w {
			t.Errorf("step %d: got %q, want %q", i, got.Steps[i].Description, w)
		}
	}
}

// TestParseUpgradePhp_IgnoresCommentsAndHeredocs verifies, on both backends, that a step inside a
// comment is not reported and that a quote inside a heredoc does not hide the steps after it.
func TestParseUpgradePhp_IgnoresCommentsAndHeredocs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.php")
	if err := os.WriteFile(path, []byte(`<?php
function xmldb_local_demo_upgrade($oldversion) {
    // if ($oldversion < 2023010100) { old step }
    /*
    if ($oldversion < 2023020200) {
    }
    */
    $note = <<<EOT
Contact O'Brien before upgrading.
EOT;
    if ($oldversion < 2024010100) { // Add the logs table.
        upgrade_plugin_savepoint(true, 2024010100, 'local', 'demo');
    }
    return true;
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, backend := range []string{"", "treesitter"} {
		t.Run("backend="+backend, func(t *testing.T) {
			t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
			got := ParseUpgradePhp(path)
			if got == nil || len(got.Steps) != 1 || got.Steps[0].Version != "2024010100" {
				t.Fatalf("got %+v, want only step 2024010100", got)
			}
			if got.Steps[0].Description != "Add the logs table." {
				t.Errorf("description = %q, want the inline comment", got.Steps[0].Description)
			}
		})
	}
}

// TestParseUpgradePhp_VersionRuleSharedByBothBackendsAndOperations verifies that both backends and
// ParseUpgradeOperations accept the same step versions — ten digits, optionally with a decimal
// part — ignore a step comparing with any other number, and order decimal versions numerically.
func TestParseUpgradePhp_VersionRuleSharedByBothBackendsAndOperations(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "upgrade.php"), `<?php
function xmldb_local_test_upgrade($oldversion) {
    if ($oldversion < 2024010100.01) { // second
    }
    if ($oldversion < 2024010100) { // first
    }
    if ($oldversion < 5) { // not a version
    }
    return true;
}
`)
	want := []string{"2024010100", "2024010100.01"}
	for _, backend := range []string{"", "treesitter"} {
		t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
		if got := GetUpgradeVersions(ExtractPluginUpgrade(dir)); !slices.Equal(got, want) {
			t.Errorf("backend %q: versions %v, want %v", backend, got, want)
		}
	}
	var ops []string
	for _, b := range ParseUpgradeOperations(dir).Blocks {
		ops = append(ops, b.Version)
	}
	if want := []string{"2024010100.01", "2024010100"}; !slices.Equal(ops, want) {
		t.Errorf("operations blocks %v, want %v (file order)", ops, want)
	}
}
