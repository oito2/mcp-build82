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
	"os"
	"path/filepath"
	"testing"
)

// TestReadFileCapped_OversizedFileReturnsErrorWithoutReading verifies that a file larger than
// maxReadableFileSize is rejected from its stat size, before its content is read.
func TestReadFileCapped_OversizedFileReturnsErrorWithoutReading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.php")
	// A sparse file avoids writing more than 8 MiB; os.Stat still reports its truncated size.
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxReadableFileSize + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := readFileCapped(path); err == nil {
		t.Error("expected an error for a file larger than maxReadableFileSize")
	}
}

// TestReadFileCapped_NormalFileReadsSuccessfully verifies that a file within the limit is read.
func TestReadFileCapped_NormalFileReadsSuccessfully(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "normal.php")
	want := []byte("<?php\n$a = 1;\n")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := readFileCapped(path)
	if err != nil {
		t.Fatalf("readFileCapped: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestReadFileCapped_AtSizeLimitStillReads verifies that a file of exactly the limit is read.
func TestReadFileCapped_AtSizeLimitStillReads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "atlimit.php")
	content := make([]byte, maxReadableFileSize)
	for i := range content {
		content[i] = ' '
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := readFileCapped(path); err != nil {
		t.Errorf("expected a file at exactly maxReadableFileSize to still read, got error: %v", err)
	}
}

// TestReadFileCapped_NonexistentReturnsError verifies that a missing file yields an error.
func TestReadFileCapped_NonexistentReturnsError(t *testing.T) {
	if _, err := readFileCapped(filepath.Join(t.TempDir(), "nonexistent.php")); err == nil {
		t.Error("expected an error for a nonexistent file")
	}
}
