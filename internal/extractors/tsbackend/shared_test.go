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

	"github.com/odvcencio/gotreesitter"
)

// parseSnippet parses a PHP snippet and returns the root node and source bytes, failing the test on a parse error.
func parseSnippet(t *testing.T, php string) (*gotreesitter.Node, []byte) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "snippet.php")
	if err := os.WriteFile(path, []byte(php), 0o644); err != nil {
		t.Fatal(err)
	}
	tree, src, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	return tree.RootNode(), src
}

// TestParseFile_NonexistentReturnsError verifies ParseFile returns an error for a nonexistent path.
func TestParseFile_NonexistentReturnsError(t *testing.T) {
	if _, _, err := ParseFile("/nonexistent/file.php"); err == nil {
		t.Error("expected an error for a nonexistent file")
	}
}

// TestParseFile_OversizedFileReturnsErrorWithoutReading verifies that ParseFile enforces its size
// cap before reading or parsing, since parsing allocates a large multiple of the source size.
func TestParseFile_OversizedFileReturnsErrorWithoutReading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.php")
	oversized := make([]byte, maxParseFileSize+1)
	if err := os.WriteFile(path, oversized, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ParseFile(path); err == nil {
		t.Error("expected an error for a file larger than maxParseFileSize")
	}
}

// TestParseFile_AtSizeLimitStillParses confirms the cap doesn't accidentally reject files at or
// just under the boundary.
func TestParseFile_AtSizeLimitStillParses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "atlimit.php")
	content := []byte("<?php\n$a = 1;\n")
	padding := make([]byte, maxParseFileSize-len(content))
	for i := range padding {
		padding[i] = ' '
	}
	if err := os.WriteFile(path, append(content, padding...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ParseFile(path); err != nil {
		t.Errorf("expected a file at exactly maxParseFileSize to still parse, got error: %v", err)
	}
}

// TestFindAssignment_MatchesByLHSText verifies FindAssignment returns the right-hand side of the assignment whose left-hand side matches.
func TestFindAssignment_MatchesByLHSText(t *testing.T) {
	root, src := parseSnippet(t, "<?php\n$a = 1;\n$b = 2;\n")
	v := FindAssignment(root, func(lhs *gotreesitter.Node) bool { return lhs.Text(src) == "$b" })
	if v == nil {
		t.Fatal("expected a match")
	}
	if got, _ := IntValue(v, src); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}

// TestFindAssignment_NoMatchReturnsNil verifies FindAssignment returns nil when no assignment matches.
func TestFindAssignment_NoMatchReturnsNil(t *testing.T) {
	root, src := parseSnippet(t, "<?php\n$a = 1;\n")
	if v := FindAssignment(root, func(lhs *gotreesitter.Node) bool { return lhs.Text(src) == "$nonexistent" }); v != nil {
		t.Error("expected nil for no match")
	}
}

// TestStringValue_Simple verifies a single-quoted literal is returned as its inner text.
func TestStringValue_Simple(t *testing.T) {
	root, src := parseSnippet(t, "<?php\n$a = 'hello';\n")
	v := FindAssignment(root, func(lhs *gotreesitter.Node) bool { return lhs.Text(src) == "$a" })
	got, ok := StringValue(v, src)
	if !ok || got != "hello" {
		t.Errorf("got (%q, %v), want (\"hello\", true)", got, ok)
	}
}

// TestStringValue_EscapedBackslashAndQuote verifies that FQNs like \local_myplugin\task\my_task
// come back with single backslashes, and that an escaped quote comes back unescaped.
func TestStringValue_EscapedBackslashAndQuote(t *testing.T) {
	root, src := parseSnippet(t, `<?php
$a = 'local_myplugin\\task\\my_task';
$b = 'it\'s here';
`)
	v := FindAssignment(root, func(lhs *gotreesitter.Node) bool { return lhs.Text(src) == "$a" })
	if got, _ := StringValue(v, src); got != `local_myplugin\task\my_task` {
		t.Errorf("got %q, want %q", got, `local_myplugin\task\my_task`)
	}

	v = FindAssignment(root, func(lhs *gotreesitter.Node) bool { return lhs.Text(src) == "$b" })
	if got, _ := StringValue(v, src); got != "it's here" {
		t.Errorf("got %q, want %q", got, "it's here")
	}
}

// TestStringValue_DoubleQuoted verifies that double-quoted strings, which parse as
// `encapsed_string` (a different node type than single-quoted `string`), are accepted.
func TestStringValue_DoubleQuoted(t *testing.T) {
	root, src := parseSnippet(t, `<?php
$a = ["classname" => 'x'];
`)
	arr := FindAssignment(root, func(lhs *gotreesitter.Node) bool { return lhs.Text(src) == "$a" })
	key, _ := KeyValue(ArrayElements(arr)[0])
	got, ok := StringValue(key, src)
	if !ok || got != "classname" {
		t.Errorf("got (%q, %v), want (\"classname\", true)", got, ok)
	}
}

// TestStringValue_Concatenation verifies that `.`-concatenated string fragments are joined,
// including 3+ fragment chains (left-associative binary_expression nesting). The regex backend
// only captures the first fragment.
func TestStringValue_Concatenation(t *testing.T) {
	root, src := parseSnippet(t, "<?php\n$a = 'Fetch the data, ' . 'creating the item';\n")
	v := FindAssignment(root, func(lhs *gotreesitter.Node) bool { return lhs.Text(src) == "$a" })
	got, ok := StringValue(v, src)
	if !ok || got != "Fetch the data, creating the item" {
		t.Errorf("got (%q, %v), want (\"Fetch the data, creating the item\", true)", got, ok)
	}
}

// TestStringValue_ConcatenationThreeFragments verifies a chain of three `.`-joined fragments is joined in order.
func TestStringValue_ConcatenationThreeFragments(t *testing.T) {
	root, src := parseSnippet(t, "<?php\n$a = 'a, ' . 'b, ' . 'c';\n")
	v := FindAssignment(root, func(lhs *gotreesitter.Node) bool { return lhs.Text(src) == "$a" })
	got, ok := StringValue(v, src)
	if !ok || got != "a, b, c" {
		t.Errorf("got (%q, %v), want (\"a, b, c\", true)", got, ok)
	}
}

// buildConcatChain returns PHP source assigning $a a string built from n single-character
// fragments joined with `.` (e.g. n=3 -> `$a = 'a'.'a'.'a';`), plus the string StringValue should
// produce for it.
func buildConcatChain(n int) (php string, want string) {
	var src strings.Builder
	var expect strings.Builder
	src.WriteString("<?php\n$a = ")
	for i := 0; i < n; i++ {
		if i > 0 {
			src.WriteString(".")
		}
		src.WriteString("'a'")
		expect.WriteString("a")
	}
	src.WriteString(";\n")
	return src.String(), expect.String()
}

// TestStringValue_LongConcatenationChain verifies that a long chain of `.`-joined fragments is
// resolved correctly and quickly without growing the Go call stack. A PHP file consisting of a
// single string built from a very large number of tiny fragments fits within the 8 MiB
// maxParseFileSize cap, and resolving it by recursion proportional to the chain length would
// exhaust the goroutine stack (a fatal error that cannot be recovered). The 9000-fragment chain
// built here is under 40 KiB of source and just under maxConcatFragments, large enough to be
// meaningful while still quick to build and parse in a unit test.
func TestStringValue_LongConcatenationChain(t *testing.T) {
	php, want := buildConcatChain(9000)
	root, src := parseSnippet(t, php)
	v := FindAssignment(root, func(lhs *gotreesitter.Node) bool { return lhs.Text(src) == "$a" })
	got, ok := StringValue(v, src)
	if !ok || got != want {
		t.Errorf("got (%d bytes, %v), want (%d bytes, true)", len(got), ok, len(want))
	}
}

// TestStringValue_ConcatenationExceedsFragmentCap confirms maxConcatFragments actually bounds
// StringValue's work: a chain longer than the cap must fail cleanly (ok=false) rather than
// process every fragment regardless of length.
func TestStringValue_ConcatenationExceedsFragmentCap(t *testing.T) {
	php, _ := buildConcatChain(maxConcatFragments + 1)
	root, src := parseSnippet(t, php)
	v := FindAssignment(root, func(lhs *gotreesitter.Node) bool { return lhs.Text(src) == "$a" })
	if _, ok := StringValue(v, src); ok {
		t.Error("expected ok=false for a concatenation chain longer than maxConcatFragments")
	}
}

// TestStringValue_ComplexInterpolationPreservesBraces verifies that PHP's "curly"/complex
// interpolation syntax (`{$expr}` inside a double-quoted string), which parses as encapsed_string
// with the `{` and `}` delimiters as anonymous children, keeps those braces literally:
// "...{$obj->prop}...", not "...$obj->prop...".
func TestStringValue_ComplexInterpolationPreservesBraces(t *testing.T) {
	root, src := parseSnippet(t, `<?php
$a = "prefix {$obj->prop} suffix";
`)
	v := FindAssignment(root, func(lhs *gotreesitter.Node) bool { return lhs.Text(src) == "$a" })
	want := "prefix {$obj->prop} suffix"
	got, ok := StringValue(v, src)
	if !ok || got != want {
		t.Errorf("got (%q, %v), want (%q, true)", got, ok, want)
	}
}

// TestStringValue_ComplexInterpolationAtStringBoundary confirms that only the outermost
// opening/closing quote characters are excluded, even when `{$expr}` sits at the very start or end
// of the string, adjacent to the quote itself.
func TestStringValue_ComplexInterpolationAtStringBoundary(t *testing.T) {
	root, src := parseSnippet(t, `<?php
$a = "{$obj->prop}";
`)
	v := FindAssignment(root, func(lhs *gotreesitter.Node) bool { return lhs.Text(src) == "$a" })
	want := "{$obj->prop}"
	got, ok := StringValue(v, src)
	if !ok || got != want {
		t.Errorf("got (%q, %v), want (%q, true)", got, ok, want)
	}
}

// TestStringValue_WrongNodeKindReturnsFalse verifies a non-string node yields ok=false.
func TestStringValue_WrongNodeKindReturnsFalse(t *testing.T) {
	root, src := parseSnippet(t, "<?php\n$a = 42;\n")
	v := FindAssignment(root, func(lhs *gotreesitter.Node) bool { return lhs.Text(src) == "$a" })
	if _, ok := StringValue(v, src); ok {
		t.Error("expected ok=false for an integer node")
	}
}

// TestIntValue verifies IntValue accepts integer nodes and rejects other node kinds.
func TestIntValue(t *testing.T) {
	root, src := parseSnippet(t, "<?php\n$a = 2024010100;\n")
	v := FindAssignment(root, func(lhs *gotreesitter.Node) bool { return lhs.Text(src) == "$a" })
	got, ok := IntValue(v, src)
	if !ok || got != 2024010100 {
		t.Errorf("got (%d, %v), want (2024010100, true)", got, ok)
	}
}

// TestArrayElementsAndKeyValue_Keyed verifies keyed array elements expose both key and value nodes.
func TestArrayElementsAndKeyValue_Keyed(t *testing.T) {
	root, src := parseSnippet(t, "<?php\n$a = ['x' => 1, 'y' => 2];\n")
	v := FindAssignment(root, func(lhs *gotreesitter.Node) bool { return lhs.Text(src) == "$a" })
	elements := ArrayElements(v)
	if len(elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elements))
	}
	key, value := KeyValue(elements[0])
	if key == nil {
		t.Fatal("expected a non-nil key for a keyed element")
	}
	if k, _ := StringValue(key, src); k != "x" {
		t.Errorf("got key %q, want \"x\"", k)
	}
	if val, _ := IntValue(value, src); val != 1 {
		t.Errorf("got value %d, want 1", val)
	}
}

// TestArrayElementsAndKeyValue_Positional verifies positional array elements expose a nil key and a value node.
func TestArrayElementsAndKeyValue_Positional(t *testing.T) {
	root, src := parseSnippet(t, "<?php\n$a = ['x', 'y'];\n")
	v := FindAssignment(root, func(lhs *gotreesitter.Node) bool { return lhs.Text(src) == "$a" })
	elements := ArrayElements(v)
	if len(elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elements))
	}
	key, value := KeyValue(elements[0])
	if key != nil {
		t.Errorf("expected a nil key for a positional element, got %+v", key)
	}
	if val, _ := StringValue(value, src); val != "x" {
		t.Errorf("got value %q, want \"x\"", val)
	}
}

// TestArrayElements_WrongNodeKindReturnsNil verifies ArrayElements returns nil for a node that is not an array literal.
func TestArrayElements_WrongNodeKindReturnsNil(t *testing.T) {
	root, src := parseSnippet(t, "<?php\n$a = 1;\n")
	v := FindAssignment(root, func(lhs *gotreesitter.Node) bool { return lhs.Text(src) == "$a" })
	if got := ArrayElements(v); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}
