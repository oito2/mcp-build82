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

// Package phpcstest installs a fake phpcs for tests; see testdata/fakephpcs for the environment
// variables that drive it.
package phpcstest

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// Install builds the fake phpcs into a temporary directory and makes that directory the whole
// PATH for the rest of the test, so no real tool is found. It returns the directory.
func Install(t *testing.T) string {
	t.Helper()
	_, self, _, _ := runtime.Caller(0)
	src := filepath.Join(filepath.Dir(self), "..", "testdata", "fakephpcs")
	dir := t.TempDir()
	name := "phpcs"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", filepath.Join(dir, name), ".")
	cmd.Dir = src
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the fake phpcs: %v\n%s", err, out)
	}
	t.Setenv("PATH", dir)
	return dir
}

// EmptyPath makes PATH an empty directory for the rest of the test, so phpcs is not found.
func EmptyPath(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}
