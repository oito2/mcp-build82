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

package generators

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWalkMoodleFiles_SkipsSymlinkedFiles verifies that walkMoodleFiles skips symlinked files.
// Callers read matched files' content, so following a symlink could pull in content from outside
// the scanned tree.
func TestWalkMoodleFiles_SkipsSymlinkedFiles(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "real.php"), "<?php\n")

	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.php")
	mustWriteFile(t, secret, "<?php\n// secret\n")
	if err := os.Symlink(secret, filepath.Join(root, "leaked.php")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	matches := globMoodleSuffix(root, ".php")
	var names []string
	for _, m := range matches {
		names = append(names, filepath.Base(m))
	}
	if !containsName(names, "real.php") {
		t.Errorf("expected real.php present, got %v", names)
	}
	if containsName(names, "leaked.php") {
		t.Errorf("expected the symlinked file to be skipped, got %v", names)
	}
}

// containsName reports whether `needle` is one of the strings in `haystack`.
func containsName(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
