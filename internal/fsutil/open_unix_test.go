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

package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestReadRegular_RefusesFIFOWithoutBlocking verifies that a FIFO with no writer, in place of an
// expected file, is refused at once instead of blocking the read forever.
func TestReadRegular_RefusesFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "version.php")
	if err := syscall.Mkfifo(path, 0o644); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := ReadRegular(path, 0)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrNotRegular) {
			t.Errorf("err = %v, want ErrNotRegular", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ReadRegular blocked on a FIFO")
	}
}

// TestReadRegular_RefusesSymlink verifies that a symbolic link is never followed, even to a
// regular file.
func TestReadRegular_RefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "secret")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "lib.php")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if content, err := ReadRegular(link, 0); !errors.Is(err, ErrNotRegular) || content != nil {
		t.Errorf("got %q, %v; want ErrNotRegular", content, err)
	}
	if f, err := OpenRegular(link); !errors.Is(err, ErrNotRegular) {
		if f != nil {
			f.Close()
		}
		t.Errorf("OpenRegular err = %v, want ErrNotRegular", err)
	}
}
