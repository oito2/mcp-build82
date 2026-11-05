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
	"slices"
	"sort"
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
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports only the read-only attribute as permission bits")
	}
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

// TestWriteAtomicAndReplace_ReplaceSymlinkInsteadOfFollowing verifies that both atomic writers put
// a regular file in place of a symbolic link at the destination and leave the link target intact.
func TestWriteAtomicAndReplace_ReplaceSymlinkInsteadOfFollowing(t *testing.T) {
	for name, write := range map[string]func(string, []byte, os.FileMode) error{"WriteAtomic": WriteAtomic, "WriteReplace": WriteReplace} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(t.TempDir(), "target")
			if err := os.WriteFile(target, []byte("keep"), 0o644); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "out.md")
			if err := os.Symlink(target, path); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if err := write(path, []byte("new"), 0o644); err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(target); string(got) != "keep" {
				t.Errorf("the link target was overwritten: %q", got)
			}
			if fi, err := os.Lstat(path); err != nil || !fi.Mode().IsRegular() {
				t.Fatalf("expected a regular file at %s, got %v (%v)", path, fi, err)
			}
			if got, _ := os.ReadFile(path); string(got) != "new" {
				t.Errorf("content = %q", got)
			}
		})
	}
}

// TestRefuseSymlinkedDir verifies that only an existing symbolic link is refused, and that the
// error names only the last path element.
func TestRefuseSymlinkedDir(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RefuseSymlinkedDir(real); err != nil {
		t.Errorf("real directory refused: %v", err)
	}
	if err := RefuseSymlinkedDir(filepath.Join(base, "missing")); err != nil {
		t.Errorf("missing directory refused: %v", err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	err := RefuseSymlinkedDir(link)
	if !errors.Is(err, ErrSymlinkedDir) || strings.Contains(err.Error(), base) {
		t.Errorf("expected a refusal naming only %q, got %v", "link", err)
	}
}

// TestReadLimited_StopsPastTheLimit verifies that content longer than the limit, as a file that
// grew after its size was checked yields, is refused after reading at most one byte past it.
func TestReadLimited_StopsPastTheLimit(t *testing.T) {
	if got, err := readLimited(strings.NewReader("12345"), "f", 5); err != nil || string(got) != "12345" {
		t.Errorf("content at the limit: got %q, %v", got, err)
	}
	r := strings.NewReader("123456789")
	if _, err := readLimited(r, "f", 5); err == nil {
		t.Error("expected a size error for content past the limit")
	}
	if read := 9 - r.Len(); read != 6 {
		t.Errorf("read %d bytes, want at most limit+1 = 6", read)
	}
}

// TestWalkFiles_MatchesSuffixAndSkipsLinks verifies that WalkFiles visits every file with the
// suffix at any depth, with its slash-separated relative path, and skips other files and symbolic
// links.
func TestWalkFiles_MatchesSuffixAndSkipsLinks(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"lib.php", "classes/a/b.php", "readme.md", "x.phpx"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("<?php\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(filepath.Join(root, "lib.php"), filepath.Join(root, "link.php")); err != nil {
			t.Fatal(err)
		}
	}

	var got []string
	WalkFiles(root, ".php", func(path, relSlash string) {
		if path != filepath.Join(root, filepath.FromSlash(relSlash)) {
			t.Errorf("path %q does not match relative %q", path, relSlash)
		}
		got = append(got, relSlash)
	})
	sort.Strings(got)
	if want := []string{"classes/a/b.php", "lib.php"}; !slices.Equal(got, want) {
		t.Errorf("visited %v, want %v", got, want)
	}
}
