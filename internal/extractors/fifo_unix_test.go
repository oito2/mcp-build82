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

//go:build unix

package extractors

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestDetectPlugin_FIFOVersionFileDoesNotHang verifies that a plugin whose version.php is a FIFO
// fails detection at once instead of blocking the server on the read.
func TestDetectPlugin_FIFOVersionFileDoesNotHang(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "local", "trap")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "version.php"), 0o644); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := DetectPlugin(dir)
		done <- err
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("DetectPlugin blocked on a FIFO version.php")
	}
}
