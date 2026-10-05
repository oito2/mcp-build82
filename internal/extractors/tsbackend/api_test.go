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

package tsbackend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oito2/mcp-build82/internal/phpdoc"
	"github.com/oito2/mcp-build82/internal/phptypes"
)

// apiFixtureLib is a well-formed PHP lib file, also used to compare against the regex backend.
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

func TestExtractFunctionsFromPhpFile_MatchesRegexBackendFixture(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lib.php")
	if err := os.WriteFile(path, []byte(apiFixtureLib), 0o644); err != nil {
		t.Fatal(err)
	}

	fns := ExtractFunctionsFromPhpFile(path)
	if len(fns) != 4 {
		t.Fatalf("expected 4 functions, got %d: %+v", len(fns), fns)
	}
	want := map[string]phpdoc.Visibility{
		"local_test_count_widgets": phpdoc.Public,
		"local_test_legacy_count":  phpdoc.Deprecated,
		"local_test_helper":        phpdoc.Private,
		"local_test_no_doc_at_all": phpdoc.Unverified,
	}
	for _, fn := range fns {
		if got, ok := want[fn.Name]; !ok || got != fn.Visibility {
			t.Errorf("%s: got visibility %v, want %v", fn.Name, fn.Visibility, want[fn.Name])
		}
	}
}

// TestExtractFunctionsFromPhpFile_LineIgnoresPrecedingPhp8Attribute verifies that the reported line
// is the `function` keyword's line, not the line of a preceding PHP 8 attribute (`#[...]`), which
// is part of the function_definition node's own span.
func TestExtractFunctionsFromPhpFile_LineIgnoresPrecedingPhp8Attribute(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lib.php")
	content := "<?php\n#[SomeAttribute]\nfunction attributed_fn() {\n    return 1;\n}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	fns := ExtractFunctionsFromPhpFile(path)
	if len(fns) != 1 || fns[0].Name != "attributed_fn" {
		t.Fatalf("expected exactly 1 function named attributed_fn, got %+v", fns)
	}
	// Line 2 is "#[SomeAttribute]"; line 3 is "function attributed_fn() {" — Line must report 3.
	if fns[0].Line != 3 {
		t.Errorf("got Line %d, want 3 (the `function` keyword's line, not the attribute's)", fns[0].Line)
	}
}

func TestExtractFunctionsFromPhpFile_MissingFile(t *testing.T) {
	if got := ExtractFunctionsFromPhpFile("/nonexistent/lib.php"); got != nil {
		t.Errorf("expected nil for a missing file, got %+v", got)
	}
}

func TestExtractFunctionsFromPhpFile_SkipsMagicMethods(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lib.php")
	if err := os.WriteFile(path, []byte("<?php\nfunction __construct() {}\nfunction real_function() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fns := ExtractFunctionsFromPhpFile(path)
	if len(fns) != 1 || fns[0].Name != "real_function" {
		t.Errorf("expected magic methods to be skipped, got %+v", fns)
	}
}

// TestExtractFunctionsFromPhpFile_IndentedFunctionsNotMatched verifies that a nested function
// (inside another function's body) is not found, only the top-level one. Tree-sitter achieves this
// by only scanning root's direct named children for function_definition, not a deep search.
func TestExtractFunctionsFromPhpFile_IndentedFunctionsNotMatched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lib.php")
	if err := os.WriteFile(path, []byte("<?php\nfunction top_level() {\n    function nested() {}\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fns := ExtractFunctionsFromPhpFile(path)
	if len(fns) != 1 || fns[0].Name != "top_level" {
		t.Errorf("expected only the top-level (non-indented) function to match, got %+v", fns)
	}
}

func TestExtractFunctionsFromPhpFile_CommentGapToleranceAndBlockCommentOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lib.php")
	// 3 blank lines between the doc block and the function must still count (regex tolerates up
	// to 3); a "//" line comment must never count as a PHPDoc block.
	content := "<?php\n" +
		"/**\n * Has some gap.\n */\n\n\n\nfunction gapped_fn() {}\n\n" +
		"// just a line comment\nfunction line_commented_fn() {}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	fns := ExtractFunctionsFromPhpFile(path)
	byName := map[string]phptypes.ApiFunction{}
	for _, fn := range fns {
		byName[fn.Name] = fn
	}
	// Note: a summary-only docblock (no @ tags at all) has a quirk in the shared
	// phpdoc.ParseDocBlock logic — the closing "*/" line's leftover "/" (after stripping the "* "
	// prefix) gets appended to the summary when no @-tag line has flipped inSummary off yet. This
	// test only cares that a Doc was found at all across the 3-blank-line gap, not the exact summary
	// text.
	if byName["gapped_fn"].Doc == nil {
		t.Errorf("expected gapped_fn to still pick up its doc block across 3 blank lines, got %+v", byName["gapped_fn"])
	}
	if byName["line_commented_fn"].Doc != nil {
		t.Errorf("expected a // line comment to never count as a PHPDoc block, got %+v", byName["line_commented_fn"].Doc)
	}
}

// TestExtractFunctionsFromPhpFile_RealFileRegression runs the tree-sitter backend against every
// real top-level *.php file under each installation's lib/ directory (a large, real corpus of
// exactly the kind of file this extractor targets).
func TestExtractFunctionsFromPhpFile_RealFileRegression(t *testing.T) {
	var found int
	for _, name := range realMoodleRoots {
		libDir := filepath.Join("/srv/workspace/www/html/mdle", name, "lib")
		entries, err := os.ReadDir(libDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".php" {
				continue
			}
			found++
			path := filepath.Join(libDir, e.Name())
			if ExtractFunctionsFromPhpFile(path) == nil {
				// nil is a valid "no functions found" result for a real file with only classes,
				// not necessarily an error — just confirm it doesn't panic (already implicit) and
				// note it for visibility.
				t.Logf("%s: no top-level functions found (may be a class-only file)", path)
			}
		}
	}
	if found == 0 {
		t.Skip("no lib/*.php files found across real Moodle installations")
	}
}
