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
	"path/filepath"
	"testing"
)

// TestFindOwnCapabilityChecks_Basic verifies that an own-prefix call is reported with its
// capability, file and line.
func TestFindOwnCapabilityChecks_Basic(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "view.php"), `<?php
require_capability('local/test:view', $context);
`)

	calls := FindOwnCapabilityChecks(dir, "local/test")
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 call, got %d: %+v", len(calls), calls)
	}
	if calls[0].Capability != "local/test:view" || calls[0].Line != 2 || calls[0].File != "view.php" {
		t.Errorf("unexpected call: %+v", calls[0])
	}
}

// TestFindOwnCapabilityChecks_IgnoresOtherPrefixes verifies that calls for other capability
// prefixes are not reported.
func TestFindOwnCapabilityChecks_IgnoresOtherPrefixes(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "view.php"), `<?php
has_capability('moodle/course:view', $context);
`)

	if calls := FindOwnCapabilityChecks(dir, "local/test"); len(calls) != 0 {
		t.Errorf("expected no calls for a non-matching capability prefix, got %+v", calls)
	}
}

// TestFindOwnCapabilityChecks_MultilineArgumentsDetected verifies that a call whose capability
// literal is on a different line than the function name is matched and reported at the line of
// the function name.
func TestFindOwnCapabilityChecks_MultilineArgumentsDetected(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "view.php"), `<?php
function x() {
    has_capability(
        'local/test:view',
        $context
    );
}
`)

	calls := FindOwnCapabilityChecks(dir, "local/test")
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 flagged multiline capability check, got %d: %+v", len(calls), calls)
	}
	if calls[0].Capability != "local/test:view" || calls[0].Line != 3 {
		t.Errorf("unexpected call: %+v", calls[0])
	}
}
