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

//go:build windows

package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRenameFile_RetriesWhileAnotherHandleHoldsTheFile verifies that a rename blocked by an open
// handle (os.Open on Windows does not grant delete sharing) is retried, and succeeds once the
// handle is closed within renameRetryTimeout.
func TestRenameFile_RetriesWhileAnotherHandleHoldsTheFile(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(src, dst); err == nil || !isRetryableRename(err) {
		f.Close()
		t.Fatalf("a rename under an open handle should fail with a retryable error, got %v", err)
	}
	go func() {
		time.Sleep(renameRetryTimeout / 4)
		f.Close()
	}()

	if err := renameFile(src, dst); err != nil {
		t.Fatalf("renameFile: %v", err)
	}
	if b, err := os.ReadFile(dst); err != nil || string(b) != "data" {
		t.Errorf("destination = %q, %v", b, err)
	}
}

// TestRenameFile_DoesNotRetryOtherErrors verifies that an error other than a sharing violation or
// access denial (here, a missing source) is returned at once.
func TestRenameFile_DoesNotRetryOtherErrors(t *testing.T) {
	dir := t.TempDir()
	start := time.Now()
	err := renameFile(filepath.Join(dir, "missing"), filepath.Join(dir, "dst"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("renameFile = %v, want a not-exist error", err)
	}
	if elapsed := time.Since(start); elapsed >= renameRetryTimeout {
		t.Errorf("returned after %v, so the error was retried", elapsed)
	}
}
