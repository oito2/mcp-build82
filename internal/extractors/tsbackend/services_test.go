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
	"testing"
)

// servicesFixtureWellFormed is a well-formed db/services.php fixture.
const servicesFixtureWellFormed = `<?php
$functions = [
    'local_test_get_data' => [
        'classname'    => '\\local_test\\external\\get_data',
        'methodname'   => 'execute',
        'description'  => 'Returns plugin data.',
        'type'         => 'read',
        'ajax'         => true,
        'capabilities' => 'local/test:view',
    ],
    'local_test_save_data' => [
        'classname'    => '\\local_test\\external\\save_data',
        'description'  => 'Saves plugin data.',
        'type'         => 'write',
        'ajax'         => false,
    ],
];`

// writeServicesPhp writes content to a temporary db/services.php and returns its path.
func writeServicesPhp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "services.php")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestParseServicesPhp_MatchesRegexBackendFixture verifies the parsed functions for a well-formed services file match the regex backend's output.
func TestParseServicesPhp_MatchesRegexBackendFixture(t *testing.T) {
	path := writeServicesPhp(t, servicesFixtureWellFormed)
	result := ParseServicesPhp(path)
	if result == nil || len(result.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %+v", result)
	}
	fn, fn2 := result.Functions[0], result.Functions[1]

	if fn.Name != "local_test_get_data" || fn.Type != "read" || !fn.Ajax || fn.Capabilities != "local/test:view" {
		t.Errorf("first function mismatch: %+v", fn)
	}
	if fn2.Name != "local_test_save_data" || fn2.Ajax || fn2.Type != "write" {
		t.Errorf("second function mismatch: %+v", fn2)
	}
	if !fn2.LoginRequired {
		t.Error("expected LoginRequired to default true")
	}
	if fn2.MethodName != "execute" {
		t.Errorf("expected MethodName to default to 'execute', got %q", fn2.MethodName)
	}
}

// TestParseServicesPhp_EmptyArray verifies a file without a $functions array yields an empty, non-nil function list.
func TestParseServicesPhp_EmptyArray(t *testing.T) {
	path := writeServicesPhp(t, "<?php\n$functions = [];\n")
	result := ParseServicesPhp(path)
	if result == nil || len(result.Functions) != 0 {
		t.Fatalf("expected empty functions, got %+v", result)
	}
}

// TestParseServicesPhp_SkipsEntryWithNeitherClassnameNorDescription verifies an entry with neither classname nor description is dropped.
func TestParseServicesPhp_SkipsEntryWithNeitherClassnameNorDescription(t *testing.T) {
	path := writeServicesPhp(t, `<?php
$functions = [
    'not_a_real_function' => [
        'type' => 'read',
    ],
];`)
	result := ParseServicesPhp(path)
	if result == nil || len(result.Functions) != 0 {
		t.Fatalf("expected the entry to be skipped, got %+v", result)
	}
}

// TestParseServicesPhp_MissingFile verifies a nonexistent file yields nil.
func TestParseServicesPhp_MissingFile(t *testing.T) {
	if ParseServicesPhp("/nonexistent/db/services.php") != nil {
		t.Error("expected nil for missing file")
	}
}

// TestParseServicesPhp_RealFileRegression runs the tree-sitter backend against every real
// db/services.php across all 4 real Moodle installations.
func TestParseServicesPhp_RealFileRegression(t *testing.T) {
	var found int
	for _, name := range realMoodleRoots {
		root := filepath.Join("/srv/workspace/www/html/mdle", name)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Base(path) != "services.php" || filepath.Base(filepath.Dir(path)) != "db" {
				return nil
			}
			found++
			if result := ParseServicesPhp(path); result == nil {
				t.Errorf("%s: expected a non-nil result", path)
			}
			return nil
		})
	}
	if found == 0 {
		t.Skip("no db/services.php files found across real Moodle installations")
	}
}
