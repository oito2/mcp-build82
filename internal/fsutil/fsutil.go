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

// Package fsutil holds two small file-I/O helpers shared by internal/cache, internal/config, and
// internal/installer: reading a file while treating its absence as an empty/optional state, and
// creating parent dirs then writing atomically.
package fsutil

import (
	"errors"
	"os"
	"path/filepath"
)

// ReadOptional reads path, returning (nil, false, nil) if it doesn't exist — the common "not
// configured/cached yet" case every caller here treats as empty state, not an error — and
// (nil, false, err) for any other read failure (e.g. permission denied), which callers should
// generally treat differently from a simple absence.
func ReadOptional(path string) (content []byte, ok bool, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

// WriteAtomic creates path's parent directory if needed, then writes data to path atomically: to a
// temp file in the same directory (so the final rename is same-filesystem and instantaneous), then
// renamed into place. This avoids ever leaving a truncated/corrupt file at path if the process
// dies mid-write (a crash, kill -9, power loss).
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
