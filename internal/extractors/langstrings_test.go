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

func TestFindOwnGetStringCalls_Basic(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "lib.php"), `<?php
echo get_string('pluginname', 'local_test');
`)

	calls := FindOwnGetStringCalls(dir, "local_test")
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 call, got %d: %+v", len(calls), calls)
	}
	if calls[0].Identifier != "pluginname" || calls[0].Line != 2 || calls[0].File != "lib.php" {
		t.Errorf("unexpected call: %+v", calls[0])
	}
}

func TestFindOwnGetStringCalls_IgnoresOtherComponents(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "lib.php"), `<?php
echo get_string('save', 'core');
`)

	if calls := FindOwnGetStringCalls(dir, "local_test"); len(calls) != 0 {
		t.Errorf("expected no calls for a different component, got %+v", calls)
	}
}

// TestFindOwnGetStringCalls_MultilineArgumentsDetected verifies that a get_string() call whose
// arguments span multiple lines (identifier and component literals on different lines than the
// function name) is matched, with the line number derived from the match's byte offset.
func TestFindOwnGetStringCalls_MultilineArgumentsDetected(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "lib.php"), `<?php
function x() {
    echo get_string(
        'pluginname',
        'local_test'
    );
}
`)

	calls := FindOwnGetStringCalls(dir, "local_test")
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 flagged multiline get_string call, got %d: %+v", len(calls), calls)
	}
	if calls[0].Identifier != "pluginname" || calls[0].Line != 3 {
		t.Errorf("unexpected call: %+v", calls[0])
	}
}
