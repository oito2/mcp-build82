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
	"path/filepath"
	"testing"
)

func TestFindDeprecatedApiUsage_FlagsBareCallsOnly(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "lib.php"), `<?php
function local_test_helper() {
    get_context_instance(CONTEXT_COURSE, 1); // bare call: should be flagged
    $x = $this->get_context_instance(1);     // method call: should be skipped
    self::get_context_instance();            // static call: should be skipped
    my_helper::get_context_instance();       // static call: should be skipped
}
`)

	deprecated := map[string]struct{}{"get_context_instance": {}}
	calls := FindDeprecatedApiUsage(dir, deprecated)

	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 flagged call, got %d: %+v", len(calls), calls)
	}
	if calls[0].Function != "get_context_instance" || calls[0].Line != 3 || calls[0].File != "lib.php" {
		t.Errorf("unexpected call: %+v", calls[0])
	}
}

func TestFindDeprecatedApiUsage_NoDeprecatedNames(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "lib.php"), `<?php
function f() { get_context_instance(); }
`)

	if calls := FindDeprecatedApiUsage(dir, nil); calls != nil {
		t.Errorf("expected nil with empty deprecated set, got %+v", calls)
	}
}

func TestFindDeprecatedApiUsage_CleanPluginReturnsNoCalls(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "lib.php"), `<?php
function local_test_helper() {
    context_course::instance(1);
}
`)

	deprecated := map[string]struct{}{"get_context_instance": {}}
	if calls := FindDeprecatedApiUsage(dir, deprecated); len(calls) != 0 {
		t.Errorf("expected no calls, got %+v", calls)
	}
}

// TestFindDeprecatedApiUsage_MultilineArgumentsDetected verifies that a call whose function name
// and opening paren land on different lines is matched, with the line number derived from the
// match's byte offset.
func TestFindDeprecatedApiUsage_MultilineArgumentsDetected(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "lib.php"), `<?php
function local_test_helper() {
    get_context_instance
        (CONTEXT_COURSE, 1);
}
`)

	deprecated := map[string]struct{}{"get_context_instance": {}}
	calls := FindDeprecatedApiUsage(dir, deprecated)
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 flagged multiline call, got %d: %+v", len(calls), calls)
	}
	if calls[0].Line != 3 {
		t.Errorf("expected the call reported at line 3 (where the function name appears), got %d", calls[0].Line)
	}
}

func TestFindDeprecatedApiUsage_WalksNestedDirectories(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "classes", "task"))
	mustWriteFile(t, filepath.Join(dir, "classes", "task", "cleanup.php"), `<?php
namespace local_test\task;
class cleanup {
    public function execute() { get_context_instance(); }
}
`)

	deprecated := map[string]struct{}{"get_context_instance": {}}
	calls := FindDeprecatedApiUsage(dir, deprecated)
	if len(calls) != 1 || calls[0].File != "classes/task/cleanup.php" {
		t.Fatalf("expected 1 call in classes/task/cleanup.php, got %+v", calls)
	}
}
