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

package phparray

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// TestExtractArrayBody_SquareBrackets verifies the body of a `[...]` array assignment is returned.
func TestExtractArrayBody_SquareBrackets(t *testing.T) {
	content := `<?php
$observers = [
    ['eventname' => '\\core\\event\\user_loggedin'],
];
`
	body, ok := ExtractArrayBody(content, "observers")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !regexp.MustCompile(`user_loggedin`).MatchString(body) {
		t.Errorf("body missing expected content: %q", body)
	}
}

// TestExtractArrayBody_ArrayKeyword verifies the body of an `array(...)` assignment is returned.
func TestExtractArrayBody_ArrayKeyword(t *testing.T) {
	content := `<?php
$tasks = array(
    array('classname' => '\\local_test\\task\\cron_task'),
);
`
	body, ok := ExtractArrayBody(content, "tasks")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !regexp.MustCompile(`cron_task`).MatchString(body) {
		t.Errorf("body missing expected content: %q", body)
	}
}

// TestExtractArrayBody_NotFound verifies a missing variable yields ok=false.
func TestExtractArrayBody_NotFound(t *testing.T) {
	if _, ok := ExtractArrayBody(`<?php $other = [];`, "observers"); ok {
		t.Error("expected ok=false when the variable isn't assigned")
	}
}

// TestExtractArrayBody_UnclosedBracket verifies an array whose bracket never closes yields ok=false.
func TestExtractArrayBody_UnclosedBracket(t *testing.T) {
	if _, ok := ExtractArrayBody(`<?php $observers = [ 'a' => 1,`, "observers"); ok {
		t.Error("expected ok=false for a never-closed bracket")
	}
}

// TestExtractArrayBody_BracketInsideStringValue verifies that a stray '['/']'/'('/')' inside a
// string value (e.g. a help string like "range [0, 1)", or a regex literal) does not affect bracket
// depth, so the array body extends to its real closing bracket.
func TestExtractArrayBody_BracketInsideStringValue(t *testing.T) {
	content := `<?php
$observers = [
    ['eventname' => 'x', 'help' => 'a stray ] bracket and a [ pair too'],
    ['eventname' => 'y'],
];
`
	body, ok := ExtractArrayBody(content, "observers")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !regexp.MustCompile(`'eventname' => 'y'`).MatchString(body) {
		t.Errorf("body truncated before the second entry — bracket inside a string value broke depth tracking: %q", body)
	}
}

// TestExtractArrayBody_ApostropheInLineCommentDoesNotDesyncDepth verifies that an apostrophe inside
// a `//` line comment ("don't", "user's", etc.) is not read as the start of a string literal, so
// the real closing bracket on a following line is still found.
func TestExtractArrayBody_ApostropheInLineCommentDoesNotDesyncDepth(t *testing.T) {
	content := `<?php
$functions = [
    'first_fn' => [
        'readonlysession' => true, // We don't modify the session.
    ],
    'second_fn' => [
        'capabilities' => 'moodle/site:sendmessage',
    ],
];
`
	body, ok := ExtractArrayBody(content, "functions")
	if !ok {
		t.Fatal("expected ok=true")
	}
	blocks := SplitIntoBlocks(body)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 top-level blocks, got %d: %v", len(blocks), blocks)
	}
	if strings.Contains(blocks[0], "sendmessage") {
		t.Errorf("first block absorbed content from the second block past the comment apostrophe: %q", blocks[0])
	}
}

// TestSplitIntoBlocks verifies a sequential array body is split into its top-level blocks, keeping nested arrays intact.
func TestSplitIntoBlocks(t *testing.T) {
	body := `['a' => 1], ['b' => ['nested' => true]], ['c' => 3]`
	blocks := SplitIntoBlocks(body)
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d: %v", len(blocks), blocks)
	}
	if blocks[1] != `['b' => ['nested' => true]]` {
		t.Errorf("nested block split incorrectly: %q", blocks[1])
	}
}

// TestSplitIntoBlocks_BracketInsideStringValue verifies that a stray bracket inside a string value
// does not desynchronize depth tracking across block boundaries.
func TestSplitIntoBlocks_BracketInsideStringValue(t *testing.T) {
	body := `['a' => 'stray ] bracket'], ['b' => 2]`
	blocks := SplitIntoBlocks(body)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d: %v", len(blocks), blocks)
	}
	if blocks[1] != `['b' => 2]` {
		t.Errorf("second block corrupted: %q", blocks[1])
	}
}

// TestExtractString_Basic verifies a quoted value is extracted for a key.
func TestExtractString_Basic(t *testing.T) {
	block := `'classname' => 'send_reminders'`
	if got := ExtractString(block, "classname"); got != "send_reminders" {
		t.Errorf("got %q, want %q", got, "send_reminders")
	}
}

// TestExtractString_NotFound verifies a missing key yields an empty string.
func TestExtractString_NotFound(t *testing.T) {
	if got := ExtractString(`'other' => 'x'`, "classname"); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// TestExtractString_BackslashUnescaping verifies that FQNs like \local_myplugin\task\my_task come
// back with single backslashes, not doubled ones.
func TestExtractString_BackslashUnescaping(t *testing.T) {
	block := `'classname' => 'local_myplugin\\task\\my_task'`
	want := `local_myplugin\task\my_task`
	if got := ExtractString(block, "classname"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestExtractString_EscapedQuoteInValue verifies that an escaped single quote inside a
// single-quoted value is unescaped and does not end the value.
func TestExtractString_EscapedQuoteInValue(t *testing.T) {
	block := `'description' => 'it\'s a test'`
	want := "it's a test"
	if got := ExtractString(block, "description"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestUnescapeString verifies `\\` and `\'` escape sequences are unescaped.
func TestUnescapeString(t *testing.T) {
	tests := []struct{ raw, want string }{
		{`local_myplugin\\task\\my_task`, `local_myplugin\task\my_task`},
		{`it\'s`, `it's`},
		{`plain`, `plain`},
	}
	for _, tt := range tests {
		if got := UnescapeString(tt.raw); got != tt.want {
			t.Errorf("UnescapeString(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

// TestExtractInt verifies integer extraction, including negative values and the default for a missing key.
func TestExtractInt(t *testing.T) {
	if got := ExtractInt(`'priority' => 500`, "priority", 0); got != 500 {
		t.Errorf("got %d, want 500", got)
	}
	if got := ExtractInt(`'priority' => -1`, "priority", 0); got != -1 {
		t.Errorf("got %d, want -1", got)
	}
	if got := ExtractInt(`'other' => 1`, "priority", 42); got != 42 {
		t.Errorf("expected default 42, got %d", got)
	}
}

// TestExtractBool verifies boolean extraction of true/false/1/0 and the default for a missing key.
func TestExtractBool(t *testing.T) {
	cases := []struct {
		block string
		want  bool
	}{
		{`'defaultenabled' => true`, true},
		{`'defaultenabled' => TRUE`, true},
		{`'defaultenabled' => false`, false},
		{`'defaultenabled' => 1`, true},
		{`'defaultenabled' => 0`, false},
	}
	for _, c := range cases {
		if got := ExtractBool(c.block, "defaultenabled", false); got != c.want {
			t.Errorf("ExtractBool(%q) = %v, want %v", c.block, got, c.want)
		}
	}
	if got := ExtractBool(`'other' => true`, "defaultenabled", true); got != true {
		t.Errorf("expected default true when key absent, got %v", got)
	}
}

// TestExtractBool_MultiDigitValueDoesNotFalsePositive verifies that a multi-digit integer (e.g.
// 100) is not matched as the boolean "1".
func TestExtractBool_MultiDigitValueDoesNotFalsePositive(t *testing.T) {
	if got := ExtractBool(`'count' => 100`, "count", false); got != false {
		t.Errorf("expected default false for a multi-digit value that isn't a real bool literal, got %v", got)
	}
	if got := ExtractBool(`'count' => 10`, "count", false); got != false {
		t.Errorf("expected default false for '10', got %v", got)
	}
	// Genuine single-digit/keyword values must still match.
	if got := ExtractBool(`'flag' => 1`, "flag", false); got != true {
		t.Errorf("expected true for a genuine '1' value, got %v", got)
	}
}

var testKeyPattern = regexp.MustCompile(`['"]([a-zA-Z0-9_]+)['"]\s*=>\s*(\[|array\s*\()`)

// TestSplitKeyedEntries verifies keyed entries are split into key and bracket-balanced body.
func TestSplitKeyedEntries(t *testing.T) {
	body := `
'core_get_data' => [
    'classname' => 'core_external',
    'methodname' => 'get_data',
],
'core_set_data' => array(
    'classname' => 'core_external',
    'methodname' => 'set_data',
),
`
	entries := SplitKeyedEntries(body, testKeyPattern)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Key != "core_get_data" || ExtractString(entries[0].Body, "methodname") != "get_data" {
		t.Errorf("entry 0 mismatch: %+v", entries[0])
	}
	if entries[1].Key != "core_set_data" || ExtractString(entries[1].Body, "methodname") != "set_data" {
		t.Errorf("entry 1 mismatch: %+v", entries[1])
	}
}

// TestSplitKeyedEntries_NestedArrayDoesNotBreakBoundary verifies a nested array inside an entry does not end that entry early.
func TestSplitKeyedEntries_NestedArrayDoesNotBreakBoundary(t *testing.T) {
	body := `'archetypes' => ['editingteacher' => 'CAP_ALLOW', 'student' => 'CAP_PREVENT'],
'other' => ['x' => 1],`
	entries := SplitKeyedEntries(body, testKeyPattern)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(entries), entries)
	}
	if !reflect.DeepEqual(entries[0].Key, "archetypes") {
		t.Errorf("expected first key 'archetypes', got %q", entries[0].Key)
	}
}

// TestSplitKeyedEntries_BracketInsideStringValue verifies that a stray bracket inside a string
// value does not corrupt the boundary of the entry itself or of the next one.
func TestSplitKeyedEntries_BracketInsideStringValue(t *testing.T) {
	body := `'core_get_data' => [
    'classname' => 'core_external',
    'help' => 'a stray ] bracket',
],
'core_set_data' => [
    'classname' => 'core_external',
],`
	entries := SplitKeyedEntries(body, testKeyPattern)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(entries), entries)
	}
	if entries[1].Key != "core_set_data" || ExtractString(entries[1].Body, "classname") != "core_external" {
		t.Errorf("second entry corrupted by stray bracket in first entry's string value: %+v", entries[1])
	}
}

func TestExtractString_QuotesInsideValue(t *testing.T) {
	cases := map[string]string{
		`'k' => 'it\'s fine',`:  `it's fine`,
		`'k' => "say 'hi'",`:    `say 'hi'`,
		`'k' => "say \"hi\"",`:  `say "hi"`,
		`'k' => 'back\\slash',`: `back\slash`,
		`'k' => 'plain',`:       `plain`,
	}
	for in, want := range cases {
		if got := ExtractString(in, "k"); got != want {
			t.Errorf("ExtractString(%s) = %q, want %q", in, got, want)
		}
	}
}
