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

// Package fsutil holds two small file-I/O helpers shared by internal/cache, internal/config, and
// internal/installer: reading a file while treating its absence as an empty/optional state, and
// creating parent dirs then writing atomically.
package fsutil

import (
	"errors"
	"os"
	"path/filepath"
)

// ReadOptional reads the file at `path`. It returns (content, true, nil) on success and
// (nil, false, nil) when the file does not exist, so absence is reported as empty state rather
// than an error. Any other read failure (e.g. permission denied) returns (nil, false, err).
// The `ok` result reports whether the file was found.
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

// WriteAtomic writes `data` to `path` with permissions `perm`, creating the parent directory
// (mode derived from `perm`: execute bits added wherever read bits are set) if needed. The data goes to a temp file in the same directory, which is then renamed
// into place, so `path` never holds a truncated file if the process dies mid-write. It returns an
// error if directory creation, writing, chmod or rename fails; the temp file is removed on failure.
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, perm.Perm()|(perm.Perm()&0o444)>>2); err != nil {
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
