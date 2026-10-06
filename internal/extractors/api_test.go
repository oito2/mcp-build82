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

// apiFixtureLib is a lib file with one public, one deprecated, one private and one undocumented
// function. Visibility classification itself is tested in internal/phpdoc.
const apiFixtureLib = `<?php
/**
 * Returns the number of widgets configured for a course.
 *
 * @param int $courseid Course ID
 * @return int Number of widgets
 */
function local_test_count_widgets($courseid) {
    return 0;
}

/**
 * Old helper kept for backwards compatibility.
 *
 * @deprecated since Moodle 4.0, use local_test_count_widgets() instead
 * @param int $courseid Course ID
 * @return int
 */
function local_test_legacy_count($courseid) {
    return 0;
}

/**
 * Internal helper not meant for external use.
 *
 * @access private
 * @deprecated no longer called anywhere
 */
function local_test_helper() {
    return true;
}

function local_test_no_doc_at_all($x) {
    return $x;
}
`

// TestExtractFunctionsFromPhpFile_Api verifies that every function in the fixture is returned
// with the expected visibility.
func TestExtractFunctionsFromPhpFile_Api(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lib.php")
	mustWriteFile(t, path, apiFixtureLib)

	fns := ExtractFunctionsFromPhpFile(path)
	if len(fns) != 4 {
		t.Fatalf("expected 4 functions, got %d: %+v", len(fns), fns)
	}
	want := map[string]ApiVisibility{
		"local_test_count_widgets": VisPublic,
		"local_test_legacy_count":  VisDeprecated,
		"local_test_helper":        VisPrivate,
		"local_test_no_doc_at_all": VisUnverified,
	}
	for _, fn := range fns {
		if got, ok := want[fn.Name]; !ok || got != fn.Visibility {
			t.Errorf("%s: got visibility %v, want %v", fn.Name, fn.Visibility, want[fn.Name])
		}
	}
}

// TestExtractFunctionsFromPhpFile_MissingFile verifies that a missing file yields nil.
func TestExtractFunctionsFromPhpFile_MissingFile(t *testing.T) {
	if got := ExtractFunctionsFromPhpFile("/nonexistent/lib.php"); got != nil {
		t.Errorf("expected nil for a missing file, got %+v", got)
	}
}

// TestExtractFunctionsFromPhpFile_SkipsMagicMethods verifies that names starting with "__" are
// skipped.
func TestExtractFunctionsFromPhpFile_SkipsMagicMethods(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lib.php")
	mustWriteFile(t, path, "<?php\nfunction __construct() {}\nfunction real_function() {}\n")

	fns := ExtractFunctionsFromPhpFile(path)
	if len(fns) != 1 || fns[0].Name != "real_function" {
		t.Errorf("expected magic methods to be skipped, got %+v", fns)
	}
}

// TestExtractFunctionsFromPhpFile_IndentedFunctionsNotMatched verifies that indented (nested)
// function declarations are not matched.
func TestExtractFunctionsFromPhpFile_IndentedFunctionsNotMatched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lib.php")
	mustWriteFile(t, path, "<?php\nfunction top_level() {\n    function nested() {}\n}\n")

	fns := ExtractFunctionsFromPhpFile(path)
	if len(fns) != 1 || fns[0].Name != "top_level" {
		t.Errorf("expected only the top-level (non-indented) function to match, got %+v", fns)
	}
}

// TestExtractMoodleApi verifies that only public and deprecated functions are returned, ordered
// public first, while Counts covers every visibility.
func TestExtractMoodleApi(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "lib"))
	mustWriteFile(t, filepath.Join(dir, "lib", "moodlelib.php"), apiFixtureLib)

	result := ExtractMoodleApi(dir)
	if len(result.Functions) != 2 {
		t.Fatalf("expected 2 functions (public+deprecated only), got %d: %+v", len(result.Functions), result.Functions)
	}
	// Public sorts before deprecated.
	if result.Functions[0].Visibility != VisPublic || result.Functions[1].Visibility != VisDeprecated {
		t.Errorf("expected public before deprecated in sort order, got %+v", result.Functions)
	}
	if result.Counts.Public != 1 || result.Counts.Deprecated != 1 || result.Counts.Private != 1 || result.Counts.Unverified != 1 {
		t.Errorf("counts mismatch (should count all visibilities pre-filter): %+v", result.Counts)
	}
}

// TestExtractFunctionsFromPhpFile_TreesitterBackendParity verifies that the tree-sitter backend
// returns the same names, visibilities, lines and files as the regex backend for the fixture.
func TestExtractFunctionsFromPhpFile_TreesitterBackendParity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lib.php")
	mustWriteFile(t, path, apiFixtureLib)

	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
	regexResult := ExtractFunctionsFromPhpFile(path)
	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
	tsResult := ExtractFunctionsFromPhpFile(path)

	regexByName := make(map[string]ApiFunction, len(regexResult))
	for _, f := range regexResult {
		regexByName[f.Name] = f
	}
	if len(regexResult) != len(tsResult) {
		t.Fatalf("function count mismatch: regex=%d treesitter=%d", len(regexResult), len(tsResult))
	}
	for _, tf := range tsResult {
		rf, ok := regexByName[tf.Name]
		if !ok {
			t.Errorf("function %q found by treesitter but not regex", tf.Name)
			continue
		}
		if rf.Visibility != tf.Visibility || rf.Line != tf.Line || rf.File != tf.File {
			t.Errorf("function %q mismatch: regex=%+v treesitter=%+v", tf.Name, rf, tf)
		}
	}
}

// TestExtractFunctionsFromPhpFile_TreesitterBackendParity_RealLibFiles compares both backends on
// every top-level lib/*.php file of the available Moodle installations, keyed by (file, name).
// Differences are logged rather than failed. The test is skipped when no file is found.
func TestExtractFunctionsFromPhpFile_TreesitterBackendParity_RealLibFiles(t *testing.T) {
	type key struct{ file, name string }

	var totalChecked int
	for _, root := range realMoodleRoots {
		libDir := filepath.Join("/srv/workspace/www/html/mdle", root, "lib")
		entries, err := os.ReadDir(libDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".php" {
				continue
			}
			path := filepath.Join(libDir, e.Name())

			t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
			regexFns := scanPhpFile(path)
			t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
			tsFns := scanPhpFile(path)

			regexByKey := make(map[key]ApiFunction, len(regexFns))
			for _, f := range regexFns {
				regexByKey[key{f.File, f.Name}] = f
			}
			tsByKey := make(map[key]ApiFunction, len(tsFns))
			for _, f := range tsFns {
				tsByKey[key{f.File, f.Name}] = f
			}

			// Disagreements are expected where tree-sitter is more accurate than the regex backend:
			// reference-return functions, PHP 8 attributes between a docblock and its function,
			// malformed doc continuation lines and non-standard closing markers.
			for k, rf := range regexByKey {
				totalChecked++
				tf, ok := tsByKey[k]
				if !ok {
					t.Logf("%s: function %q found by regex but not treesitter (unexpected direction — investigate if seen)", path, k.name)
					continue
				}
				if rf.Visibility != tf.Visibility {
					t.Logf("%s: function %q visibility differs (the backends classify docblock edge cases differently): regex=%v treesitter=%v", path, k.name, rf.Visibility, tf.Visibility)
				}
			}
			for k := range tsByKey {
				if _, ok := regexByKey[k]; !ok {
					t.Logf("%s: function %q found by treesitter but not regex (the regex backend misses some declarations, e.g. reference-return functions)", path, k.name)
				}
			}
		}
	}
	if totalChecked == 0 {
		t.Skip("no lib/*.php files found across real Moodle installations")
	}
	t.Logf("checked %d functions across all real lib/*.php files", totalChecked)
}

// TestExtractMoodleApi_PriorityFileOrdering verifies that getPhpFiles lists priority files in
// PriorityFiles order before any other file.
func TestExtractMoodleApi_PriorityFileOrdering(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "lib"))
	// A non-priority file must come after every priority file that is present.
	mustWriteFile(t, filepath.Join(dir, "lib", "accesslib.php"), "<?php\nfunction acc_fn() {}\n")
	mustWriteFile(t, filepath.Join(dir, "lib", "moodlelib.php"), "<?php\nfunction moo_fn() {}\n")
	mustWriteFile(t, filepath.Join(dir, "lib", "zzz_not_priority.php"), "<?php\nfunction zzz_fn() {}\n")

	files := getPhpFiles(filepath.Join(dir, "lib"))
	// moodlelib.php precedes accesslib.php in PriorityFiles, and both precede the other file.
	idxMoodle, idxAccess, idxZzz := -1, -1, -1
	for i, f := range files {
		switch filepath.Base(f) {
		case "moodlelib.php":
			idxMoodle = i
		case "accesslib.php":
			idxAccess = i
		case "zzz_not_priority.php":
			idxZzz = i
		}
	}
	if idxMoodle >= idxAccess || idxAccess >= idxZzz {
		t.Errorf("expected priority-file ordering moodlelib < accesslib < non-priority, got %v", files)
	}
}
