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
	"testing"
)

// servicesFixtureWellFormed is a services.php with two web service functions.
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

// TestParseServicesPhp verifies the fields parsed from a well-formed services.php, including defaults.
func TestParseServicesPhp(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "services.php"), servicesFixtureWellFormed)

	result := ParseServicesPhp(filepath.Join(dir, "db", "services.php"))
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

// TestParseServicesPhp_EmptyArray verifies that an empty $functions array yields a non-nil extraction without functions.
func TestParseServicesPhp_EmptyArray(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "services.php"), "<?php\n$functions = [];\n")

	result := ParseServicesPhp(filepath.Join(dir, "db", "services.php"))
	if result == nil || len(result.Functions) != 0 {
		t.Fatalf("expected empty functions, got %+v", result)
	}
}

// TestParseServicesPhp_SkipsEntryWithNeitherClassnameNorDescription verifies that an entry with neither classname nor description is
// skipped.
func TestParseServicesPhp_SkipsEntryWithNeitherClassnameNorDescription(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	content := `<?php
$functions = [
    'not_a_real_function' => [
        'type' => 'read',
    ],
];`
	mustWriteFile(t, filepath.Join(dir, "db", "services.php"), content)

	result := ParseServicesPhp(filepath.Join(dir, "db", "services.php"))
	if result == nil || len(result.Functions) != 0 {
		t.Fatalf("expected the entry to be skipped, got %+v", result)
	}
}

// TestParseServicesPhp_TreesitterBackendParity verifies that both backends return identical
// functions for a well-formed services.php.
func TestParseServicesPhp_TreesitterBackendParity(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "services.php"), servicesFixtureWellFormed)
	path := filepath.Join(dir, "db", "services.php")

	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
	regexResult := ParseServicesPhp(path)
	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
	tsResult := ParseServicesPhp(path)

	if len(regexResult.Functions) != len(tsResult.Functions) {
		t.Fatalf("function count mismatch: regex=%d treesitter=%d", len(regexResult.Functions), len(tsResult.Functions))
	}
	for i := range regexResult.Functions {
		if regexResult.Functions[i] != tsResult.Functions[i] {
			t.Errorf("function %d mismatch: regex=%+v treesitter=%+v", i, regexResult.Functions[i], tsResult.Functions[i])
		}
	}
}

// TestParseServicesPhp_TreesitterBackendParity_RealFiles verifies that both backends agree on every
// db/services.php of the available Moodle installations. The test is skipped when none is found.
func TestParseServicesPhp_TreesitterBackendParity_RealFiles(t *testing.T) {
	var checked int
	for _, name := range realMoodleRoots {
		root := filepath.Join("/srv/workspace/www/html/mdle", name)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Base(path) != "services.php" || filepath.Base(filepath.Dir(path)) != "db" {
				return nil
			}
			checked++

			t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
			regexResult := ParseServicesPhp(path)
			t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
			tsResult := ParseServicesPhp(path)

			if len(regexResult.Functions) != len(tsResult.Functions) {
				t.Errorf("%s: function count mismatch: regex=%d treesitter=%d", path, len(regexResult.Functions), len(tsResult.Functions))
				return nil
			}
			for i := range regexResult.Functions {
				assertServiceFunctionParity(t, path, i, regexResult.Functions[i], tsResult.Functions[i])
			}
			return nil
		})
	}
	if checked == 0 {
		t.Skip("no db/services.php files found across real Moodle installations")
	}
}

// assertServiceFunctionParity fails the test unless the regex function `regex` and the tree-sitter
// function `ts`, entry `i` of the file `path`, agree on every field. Description and Capabilities
// differences are only logged, because the regex backend truncates strings containing an escaped
// apostrophe or `.` concatenation while tree-sitter returns the full text.
func assertServiceFunctionParity(t *testing.T, path string, i int, regex, ts WebServiceFunction) {
	t.Helper()
	if regex.Name != ts.Name || regex.ClassName != ts.ClassName || regex.MethodName != ts.MethodName ||
		regex.Type != ts.Type || regex.Ajax != ts.Ajax || regex.LoginRequired != ts.LoginRequired {
		t.Errorf("%s: function %d mismatch (non-string-truncation fields): regex=%+v treesitter=%+v", path, i, regex, ts)
		return
	}
	if regex.Description != ts.Description {
		t.Logf("%s: function %d Description differs (the backends read the service description differently): regex=%q treesitter=%q", path, i, regex.Description, ts.Description)
	}
	if regex.Capabilities != ts.Capabilities {
		t.Logf("%s: function %d Capabilities differs (the backends read the service capabilities differently): regex=%q treesitter=%q", path, i, regex.Capabilities, ts.Capabilities)
	}
}

// TestGetFunctionNames_NilInputDoesNotPanic verifies that a nil extraction yields nil.
func TestGetFunctionNames_NilInputDoesNotPanic(t *testing.T) {
	if got := GetFunctionNames(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// TestGetFunctionNames_Sorted verifies that function names are returned in sorted order.
func TestGetFunctionNames_Sorted(t *testing.T) {
	names := GetFunctionNames(&ServicesExtraction{Functions: []WebServiceFunction{
		{Name: "z_func"}, {Name: "a_func"},
	}})
	if names[0] != "a_func" || names[1] != "z_func" {
		t.Errorf("expected sorted names, got %v", names)
	}
}
