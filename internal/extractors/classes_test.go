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

func TestExtractClasses_Basic(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "classes", "task"))
	mustWriteFile(t, filepath.Join(dir, "classes", "task", "send_reminders.php"), `<?php
namespace local_test\task;

class send_reminders extends \core\task\scheduled_task {
    public function execute() {}
}
`)
	mustWriteFile(t, filepath.Join(dir, "classes", "event_observer.php"), `<?php
class event_observer implements \some\iface {
    public static function handle() {}
}
`)

	result := ExtractClasses(filepath.Join(dir, "classes"), dir, "**/*.php")
	if len(result.Classes) != 2 {
		t.Fatalf("expected 2 classes, got %d: %+v", len(result.Classes), result.Classes)
	}

	// Sorted by namespace then name: "" (event_observer) sorts before "local_test\task" (send_reminders).
	first, second := result.Classes[0], result.Classes[1]
	if first.Name != "event_observer" || first.FQN != `\event_observer` {
		t.Errorf("first class mismatch: %+v", first)
	}
	if len(first.Implements) != 1 || first.Implements[0] != `\some\iface` {
		t.Errorf("implements mismatch: %+v", first)
	}

	if second.Name != "send_reminders" || second.FQN != `\local_test\task\send_reminders` {
		t.Errorf("second class mismatch: %+v", second)
	}
	if second.Extends != `\core\task\scheduled_task` {
		t.Errorf("extends mismatch: %+v", second)
	}
	if second.File != "classes/task/send_reminders.php" {
		t.Errorf("expected File relative to root, got %q", second.File)
	}
}

func TestExtractClasses_MultiLineDeclaration(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "classes"))
	mustWriteFile(t, filepath.Join(dir, "classes", "foo.php"), `<?php
class foo extends
    bar implements
    baz, qux
{
    public function x() {}
}
`)

	result := ExtractClasses(filepath.Join(dir, "classes"), dir, "**/*.php")
	if len(result.Classes) != 1 {
		t.Fatalf("expected 1 class, got %d: %+v", len(result.Classes), result.Classes)
	}
	c := result.Classes[0]
	if c.Extends != "bar" {
		t.Errorf("expected multi-line extends to resolve to 'bar', got %q", c.Extends)
	}
	if len(c.Implements) != 2 || c.Implements[0] != "baz" || c.Implements[1] != "qux" {
		t.Errorf("expected multi-line implements ['baz','qux'], got %+v", c.Implements)
	}
}

func TestExtractClasses_AllKinds(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "classes"))
	mustWriteFile(t, filepath.Join(dir, "classes", "kinds.php"), `<?php
abstract class my_abstract {}
interface my_interface {}
trait my_trait {}
enum my_enum {}
`)

	result := ExtractClasses(filepath.Join(dir, "classes"), dir, "**/*.php")
	if len(result.Classes) != 4 {
		t.Fatalf("expected 4 declarations, got %d: %+v", len(result.Classes), result.Classes)
	}
	kinds := map[string]ClassKind{}
	for _, c := range result.Classes {
		kinds[c.Name] = c.Kind
	}
	if kinds["my_abstract"] != "abstract class" || kinds["my_interface"] != "interface" ||
		kinds["my_trait"] != "trait" || kinds["my_enum"] != "enum" {
		t.Errorf("kind mismatch: %+v", kinds)
	}
}

func TestParseRenamedClassesPhp(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	content := `<?php
$renamedclasses = [
    'block_test' => \block_test\output\main::class,
    'block_test_old_helper' => 'block_test\\helper',
];`
	mustWriteFile(t, filepath.Join(dir, "db", "renamedclasses.php"), content)

	renamed := ParseRenamedClassesPhp(filepath.Join(dir, "db", "renamedclasses.php"))
	if len(renamed) != 2 {
		t.Fatalf("expected 2 renamed classes, got %d: %+v", len(renamed), renamed)
	}
	if renamed[0].OldName != "block_test" || renamed[0].NewName != `block_test\output\main` {
		t.Errorf("entry 0 mismatch: %+v", renamed[0])
	}
	if renamed[1].OldName != "block_test_old_helper" || renamed[1].NewName != `block_test\helper` {
		t.Errorf("entry 1 mismatch: %+v", renamed[1])
	}
}

func TestParseRenamedClassesPhp_MissingFile(t *testing.T) {
	if got := ParseRenamedClassesPhp("/nonexistent/db/renamedclasses.php"); got != nil {
		t.Errorf("expected nil for a missing file, got %+v", got)
	}
}

func TestExtractPluginClasses_IncludesRenamedClasses(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "classes"))
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "renamedclasses.php"), `<?php
$renamedclasses = ['local_test_old' => \local_test\helper::class];`)

	extraction := ExtractPluginClasses(dir)
	if len(extraction.RenamedClasses) != 1 || extraction.RenamedClasses[0].OldName != "local_test_old" {
		t.Errorf("expected renamed classes to be attached, got %+v", extraction.RenamedClasses)
	}
}

// TestExtractClasses_TreesitterBackendParity confirms BUILD82_EXTRACTOR_BACKEND=treesitter
// produces identical output to the regex backend for well-formed fixtures, including the
// multi-line-declaration case (both backends must reach the same answer even though only the
// regex backend needs special-case logic to get there).
func TestExtractClasses_TreesitterBackendParity(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "classes"))
	mustWriteFile(t, filepath.Join(dir, "classes", "foo.php"), `<?php
namespace local_test;

abstract class foo extends
    bar implements
    baz, qux
{
    public function x() {}
}
`)

	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
	regexResult := ExtractClasses(filepath.Join(dir, "classes"), dir, "**/*.php")
	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
	tsResult := ExtractClasses(filepath.Join(dir, "classes"), dir, "**/*.php")

	if len(regexResult.Classes) != 1 || len(tsResult.Classes) != 1 {
		t.Fatalf("expected 1 class each: regex=%+v treesitter=%+v", regexResult.Classes, tsResult.Classes)
	}
	if !phpClassEqual(regexResult.Classes[0], tsResult.Classes[0]) {
		t.Errorf("class mismatch: regex=%+v treesitter=%+v", regexResult.Classes[0], tsResult.Classes[0])
	}
}

// phpClassEqual compares two PhpClass values field-by-field (PhpClass isn't comparable with == due
// to its []string Implements field).
func phpClassEqual(a, b PhpClass) bool {
	if a.Name != b.Name || a.Namespace != b.Namespace || a.FQN != b.FQN || a.Kind != b.Kind ||
		a.File != b.File || a.Extends != b.Extends || len(a.Implements) != len(b.Implements) {
		return false
	}
	for i := range a.Implements {
		if a.Implements[i] != b.Implements[i] {
			return false
		}
	}
	return true
}

// assertPhpClassParity compares one class's fields, with an exception for Implements: the regex
// backend's line-anchored implements pattern truncates a multi-line implements clause to its first
// interface, while tree-sitter returns the full list; the difference is logged, not failed. Every
// other field is asserted exactly.
func assertPhpClassParity(t *testing.T, fqn string, regex, ts PhpClass) {
	t.Helper()
	if regex.Name != ts.Name || regex.Namespace != ts.Namespace || regex.FQN != ts.FQN ||
		regex.Kind != ts.Kind || regex.File != ts.File || regex.Extends != ts.Extends {
		t.Errorf("class %q mismatch (non-implements fields): regex=%+v treesitter=%+v", fqn, regex, ts)
		return
	}
	implementsEqual := len(regex.Implements) == len(ts.Implements)
	if implementsEqual {
		for i := range regex.Implements {
			if regex.Implements[i] != ts.Implements[i] {
				implementsEqual = false
				break
			}
		}
	}
	if !implementsEqual {
		t.Logf("class %q Implements differs (expected per KNOWN_DIVERGENCES.md #5): regex=%v treesitter=%v", fqn, regex.Implements, ts.Implements)
	}
}

// TestExtractClasses_TreesitterBackendParity_RealFiles runs both backends against a real,
// class-heavy corpus (Moodle core's own lib/classes/ tree) and confirms they agree, matched by
// FQN rather than positional index (a class inside a comment, or a "final class" the regex
// backend's line-anchored kindPattern might miss, could make counts legitimately differ).
func TestExtractClasses_TreesitterBackendParity_RealFiles(t *testing.T) {
	root := "/srv/workspace/www/html/mdle/dev-500/lib/classes"
	if _, err := os.Stat(root); err != nil {
		t.Skipf("real Moodle fixture tree not available: %v", err)
	}

	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
	regexResult := ExtractClasses(root, root, "**/*.php")
	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
	tsResult := ExtractClasses(root, root, "**/*.php")

	regexByFQN := make(map[string]PhpClass, len(regexResult.Classes))
	for _, c := range regexResult.Classes {
		regexByFQN[c.FQN] = c
	}
	tsByFQN := make(map[string]PhpClass, len(tsResult.Classes))
	for _, c := range tsResult.Classes {
		tsByFQN[c.FQN] = c
	}

	for fqn, rc := range regexByFQN {
		tc, ok := tsByFQN[fqn]
		if !ok {
			t.Logf("class %q found by regex but not treesitter", fqn)
			continue
		}
		assertPhpClassParity(t, fqn, rc, tc)
	}
	for fqn := range tsByFQN {
		if _, ok := regexByFQN[fqn]; !ok {
			t.Logf("class %q found by treesitter but not regex (expected per KNOWN_DIVERGENCES.md #7 — e.g. a \"final class\" the regex backend's line-anchored pattern never detects at all)", fqn)
		}
	}
}

// TestParseRenamedClassesPhp_TreesitterBackendParity_RealFiles runs both backends against every
// real db/renamedclasses.php across all 4 real Moodle installations.
func TestParseRenamedClassesPhp_TreesitterBackendParity_RealFiles(t *testing.T) {
	var checked int
	for _, name := range realMoodleRoots {
		root := filepath.Join("/srv/workspace/www/html/mdle", name)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Base(path) != "renamedclasses.php" {
				return nil
			}
			checked++

			t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
			regexResult := ParseRenamedClassesPhp(path)
			t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
			tsResult := ParseRenamedClassesPhp(path)

			// Compare by OldName, not count/position: the regex backend's key pattern can't match an old
			// class name containing a namespace separator, so tree-sitter finding strictly more entries here
			// is expected.
			regexByOld := make(map[string]RenamedClass, len(regexResult))
			for _, r := range regexResult {
				regexByOld[r.OldName] = r
			}
			tsByOld := make(map[string]RenamedClass, len(tsResult))
			for _, r := range tsResult {
				tsByOld[r.OldName] = r
			}
			for old, rc := range regexByOld {
				tc, ok := tsByOld[old]
				if !ok {
					t.Errorf("%s: renamed class %q found by regex but not treesitter (unexpected)", path, old)
					continue
				}
				if rc != tc {
					t.Errorf("%s: renamed class %q mismatch: regex=%+v treesitter=%+v", path, old, rc, tc)
				}
			}
			for old := range tsByOld {
				if _, ok := regexByOld[old]; !ok {
					t.Logf("%s: renamed class %q found by treesitter but not regex (expected per KNOWN_DIVERGENCES.md #6)", path, old)
				}
			}
			return nil
		})
	}
	if checked == 0 {
		t.Skip("no db/renamedclasses.php files found across real Moodle installations")
	}
}

// TestExtractClasses_SkipsSymlinkedFiles verifies that symlinked files are skipped:
// os.ReadFile follows a symlink to wherever it points, so a symlinked "*.php" file planted inside a
// scanned directory would otherwise get its target's content parsed and included in the generated
// classes index — even when the target lives entirely outside the plugin (or Moodle) tree.
func TestExtractClasses_SkipsSymlinkedFiles(t *testing.T) {
	for _, backend := range extractorBackends {
		t.Run(backendLabel(backend), func(t *testing.T) {
			t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)

			dir := t.TempDir()
			classesDir := filepath.Join(dir, "classes")
			mustMkdirAll(t, classesDir)
			mustWriteFile(t, filepath.Join(classesDir, "real.php"), "<?php\nclass real_class {}\n")

			outside := t.TempDir()
			secret := filepath.Join(outside, "secret.php")
			mustWriteFile(t, secret, "<?php\nclass secret_leaked_class {}\n")
			if err := os.Symlink(secret, filepath.Join(classesDir, "leaked.php")); err != nil {
				t.Fatalf("symlink: %v", err)
			}

			extraction := ExtractClasses(classesDir, dir, "**/*.php")
			var names []string
			for _, c := range extraction.Classes {
				names = append(names, c.Name)
			}
			if !containsStr(names, "real_class") {
				t.Errorf("expected real_class present, got %v", names)
			}
			if containsStr(names, "secret_leaked_class") {
				t.Errorf("expected the symlinked file's class to be skipped, got %v", names)
			}
		})
	}
}

func containsStr(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// TestExtractClasses_IgnoresClassesInsideBlockCommentsAndHeredocs verifies that a line starting
// with "class Foo {" inside a /* ... */ block comment or inside a heredoc/nowdoc body is not
// reported as a declaration. The fixture has 1 real class, 1 lookalike inside a block comment, and
// 1 lookalike inside a heredoc body — only the real one must be found.
func TestExtractClasses_IgnoresClassesInsideBlockCommentsAndHeredocs(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "classes"))
	mustWriteFile(t, filepath.Join(dir, "classes", "mixed.php"), `<?php
namespace local_test;

class real_class {
    public function x() {}
}

/*
class commented_out_class {
    public function y() {}
}
*/

$sample = <<<PHP
class heredoc_class {
    public function z() {}
}
PHP;
`)

	result := ExtractClasses(filepath.Join(dir, "classes"), dir, "**/*.php")
	if len(result.Classes) != 1 {
		t.Fatalf("expected only the real class, got %d: %+v", len(result.Classes), result.Classes)
	}
	if result.Classes[0].Name != "real_class" {
		t.Errorf("expected real_class, got %+v", result.Classes[0])
	}
}

// TestExtractClasses_BlockCommentAndHeredocStrippingPreservesUnrelatedCode confirms
// stripCommentsAndHeredocs doesn't disturb ordinary code, quoted strings containing lookalike
// sequences ("/*", "<<<"), or a class declaration that legitimately follows a comment/heredoc in
// the same file.
func TestExtractClasses_BlockCommentAndHeredocStrippingPreservesUnrelatedCode(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "classes"))
	mustWriteFile(t, filepath.Join(dir, "classes", "afterward.php"), `<?php
// A line comment mentioning /* and <<<NOTREAL is not a real comment or heredoc start.
$note = "this string contains /* and <<<NOTREAL literally";

/* a block comment */
$sample = <<<EOT
just some text
EOT;

class after_all {
    public function ok() {}
}
`)

	result := ExtractClasses(filepath.Join(dir, "classes"), dir, "**/*.php")
	if len(result.Classes) != 1 || result.Classes[0].Name != "after_all" {
		t.Fatalf("expected exactly 1 class (after_all), got %d: %+v", len(result.Classes), result.Classes)
	}
}

func TestGetClassFQNs_Sorted(t *testing.T) {
	fqns := GetClassFQNs(ClassesExtraction{Classes: []PhpClass{{FQN: `\z`}, {FQN: `\a`}}})
	if fqns[0] != `\a` || fqns[1] != `\z` {
		t.Errorf("expected sorted FQNs, got %v", fqns)
	}
}
