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

package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestReadOptional_MissingFileReturnsOkFalseNoError verifies a missing file yields nil content, ok=false and no error.
func TestReadOptional_MissingFileReturnsOkFalseNoError(t *testing.T) {
	content, ok, err := ReadOptional(filepath.Join(t.TempDir(), "missing.txt"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected ok=false for a missing file")
	}
	if content != nil {
		t.Errorf("expected nil content, got %q", content)
	}
}

// TestReadOptional_ExistingFileReturnsContent verifies an existing file's content is returned with ok=true.
func TestReadOptional_ExistingFileReturnsContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "present.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	content, ok, err := ReadOptional(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || string(content) != "hello" {
		t.Errorf("expected ok=true content=%q, got ok=%v content=%q", "hello", ok, content)
	}
}

// TestReadOptional_PermissionDeniedReturnsError confirms a real read failure (not "doesn't exist")
// is surfaced via err, not silently treated the same as a missing file.
func TestReadOptional_PermissionDeniedReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits do not restrict reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root — permission checks don't apply")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(path, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) }) // let t.TempDir() clean up afterward

	_, ok, err := ReadOptional(path)
	if ok {
		t.Error("expected ok=false")
	}
	if err == nil {
		t.Error("expected a non-nil error for a permission-denied read, distinguishing it from a missing file")
	}
}

// TestWriteAtomic_CreatesParentDirAndWritesContent verifies missing parent directories are created and the content is written.
func TestWriteAtomic_CreatesParentDirAndWritesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")

	if err := WriteAtomic(path, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != `{"a":1}` {
		t.Errorf("unexpected content %q (err=%v)", content, err)
	}
}

// TestWriteAtomic_OverwritesExistingFile verifies an existing file is replaced by the new content.
func TestWriteAtomic_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, []byte("new"), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "new" {
		t.Errorf("expected 'new', got %q (err=%v)", content, err)
	}
}

// TestWriteAtomic_NoStrayTempFileLeftBehind confirms the temp file used internally doesn't leak
// into the target directory after a successful write.
func TestWriteAtomic_NoStrayTempFileLeftBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := WriteAtomic(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.json" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("expected exactly one file 'config.json', got %v", names)
	}
}

// TestWriteAtomic_SetsRequestedPermissions verifies the written file has the requested permission bits.
func TestWriteAtomic_SetsRequestedPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports only the read-only attribute as permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := WriteAtomic(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected mode 0600, got %v", info.Mode().Perm())
	}
}

// TestWriteAtomic_DirModeDerivedFromPerm verifies a 0600 file gets a 0700 parent directory.
func TestWriteAtomic_DirModeDerivedFromPerm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "f")
	if err := WriteAtomic(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o700 {
		t.Errorf("dir mode = %o, want 700", got)
	}
}

// TestReadRegular_ReadsRegularFileAndEnforcesMaxSize verifies that a regular file is read whole,
// that a positive maxSize refuses a larger file, and that a directory is refused.
func TestReadRegular_ReadsRegularFileAndEnforcesMaxSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "version.php")
	if err := os.WriteFile(path, []byte("<?php // 0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadRegular(path, 0); err != nil || string(got) != "<?php // 0123456789" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := ReadRegular(path, 5); err == nil || !strings.Contains(err.Error(), "exceeds max readable size") {
		t.Errorf("expected a size error, got %v", err)
	}
	if _, err := ReadRegular(dir, 0); !errors.Is(err, ErrNotRegular) {
		t.Errorf("directory: err = %v, want ErrNotRegular", err)
	}
	if _, err := ReadRegular(filepath.Join(dir, "missing"), 0); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file: err = %v, want os.ErrNotExist", err)
	}
}
