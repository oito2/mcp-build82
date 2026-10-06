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

	"github.com/oito2/mcp-build82/internal/phptypes"
)

// TestExtractClasses_Basic verifies classes are extracted with namespace, FQN, kind, parent and interfaces, sorted by namespace then name.
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

// TestExtractClasses_NoNamespaceDeclarationDespiteQualifiedNameReferences verifies that a file
// with no real namespace but a qualified reference does not have its namespace/FQN inferred from
// that reference. A bare "namespace_name" node also occurs inside any qualified_name (e.g. the
// `\some\iface` in `implements \some\iface`), not just inside an actual `namespace X;`
// declaration.
func TestExtractClasses_NoNamespaceDeclarationDespiteQualifiedNameReferences(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "classes"))
	mustWriteFile(t, filepath.Join(dir, "classes", "no_ns.php"), `<?php
class no_ns extends \some\other\base implements \another\iface {
}
`)
	result := ExtractClasses(filepath.Join(dir, "classes"), dir, "**/*.php")
	if len(result.Classes) != 1 {
		t.Fatalf("expected 1 class, got %d: %+v", len(result.Classes), result.Classes)
	}
	c := result.Classes[0]
	if c.Namespace != "" {
		t.Errorf("expected empty namespace, got %q", c.Namespace)
	}
	if c.FQN != `\no_ns` {
		t.Errorf("expected FQN \\no_ns (no namespace prefix), got %q", c.FQN)
	}
}

// TestExtractClasses_MultiLineDeclaration mirrors the regex backend's own test of the same name —
// tree-sitter needs no special-case logic for this at all (a class_declaration node's span
// naturally covers the whole signature regardless of line count), but the *result* must still
// match exactly.
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

// TestExtractClasses_AllKinds verifies class, abstract class, interface, trait and enum declarations are all recognized.
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
	kinds := map[string]phptypes.ClassKind{}
	for _, c := range result.Classes {
		kinds[c.Name] = c.Kind
	}
	if kinds["my_abstract"] != "abstract class" || kinds["my_interface"] != "interface" ||
		kinds["my_trait"] != "trait" || kinds["my_enum"] != "enum" {
		t.Errorf("kind mismatch: %+v", kinds)
	}
}

// TestParseRenamedClassesPhp verifies renamed-class entries given as quoted strings or `::class` references are normalized to bare FQNs.
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

// TestParseRenamedClassesPhp_MissingFile verifies a nonexistent file yields nil.
func TestParseRenamedClassesPhp_MissingFile(t *testing.T) {
	if got := ParseRenamedClassesPhp("/nonexistent/db/renamedclasses.php"); got != nil {
		t.Errorf("expected nil for a missing file, got %+v", got)
	}
}

// TestExtractClasses_RealFileRegression runs the tree-sitter backend against every real *.php
// file under a real Moodle installation's lib/classes/ tree (a large, real class-heavy corpus).
func TestExtractClasses_RealFileRegression(t *testing.T) {
	root := "/srv/workspace/www/html/mdle/dev-500/lib/classes"
	if _, err := os.Stat(root); err != nil {
		t.Skipf("real Moodle fixture tree not available: %v", err)
	}
	result := ExtractClasses(root, root, "**/*.php")
	if len(result.Classes) == 0 {
		t.Error("expected at least some classes to be found in lib/classes/")
	}
}

// TestParseRenamedClassesPhp_RealFileRegression runs against every real db/renamedclasses.php
// across all 4 real Moodle installations.
func TestParseRenamedClassesPhp_RealFileRegression(t *testing.T) {
	var found int
	for _, name := range realMoodleRoots {
		root := filepath.Join("/srv/workspace/www/html/mdle", name)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Base(path) != "renamedclasses.php" {
				return nil
			}
			found++
			if got := ParseRenamedClassesPhp(path); got == nil {
				t.Logf("%s: no renamed classes found (may legitimately be empty)", path)
			}
			return nil
		})
	}
	if found == 0 {
		t.Skip("no db/renamedclasses.php files found across real Moodle installations")
	}
}
