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

// Package fsutil holds small file-I/O helpers shared by the rest of build82: reading a file while
// treating its absence as an empty/optional state, opening and reading only regular files (never a
// FIFO, device or symbolic link), creating parent dirs then writing atomically, renaming with a
// retry for files briefly locked on Windows, and syncing a directory after a rename.
package fsutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

// Rename renames `oldpath` to `newpath` like os.Rename. On Windows a rename blocked by another
// process holding the file open (a sharing violation or an access-denied error, typically an
// antivirus scan) is retried for up to one second; on other systems it is os.Rename. It returns
// the last error when the rename does not succeed.
func Rename(oldpath, newpath string) error {
	return renameFile(oldpath, newpath)
}

// SyncDir flushes the directory entries of `dir` to disk, so a rename or a new file in it survives
// a crash. It is best-effort: errors are ignored, and it does nothing on Windows, where a directory
// cannot be synced this way.
func SyncDir(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}

// ErrNotRegular reports a path that names something other than a regular file: a symbolic link,
// a FIFO, a device, a socket or a directory.
var ErrNotRegular = errors.New("not a regular file")

// OpenRegular opens the file at `path` for reading only when it is a regular file. A symbolic link
// at the last path element is never followed, and a FIFO is never waited on: on unix the open
// itself refuses a link and does not block, and the opened file is then checked with Stat, so a
// file swapped after any earlier check cannot slip through. It returns an error wrapping
// ErrNotRegular for anything other than a regular file, or the open error.
func OpenRegular(path string) (*os.File, error) {
	if refuseSymlinkFirst {
		if info, err := os.Lstat(path); err != nil {
			return nil, err
		} else if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
		}
	}
	f, err := os.OpenFile(path, os.O_RDONLY|openFlags, 0)
	if err != nil {
		if info, lerr := os.Lstat(path); lerr == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
		}
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	return f, nil
}

// ReadRegular returns the content of the regular file at `path`, opened with OpenRegular. When
// `maxSize` is positive, a file larger than that many bytes is refused. It returns the error of
// OpenRegular, a size error, or the read error.
func ReadRegular(path string, maxSize int64) ([]byte, error) {
	f, err := OpenRegular(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if maxSize > 0 {
		if info, err := f.Stat(); err == nil && info.Size() > maxSize {
			return nil, fmt.Errorf("file %s exceeds max readable size (%d > %d bytes)", path, info.Size(), maxSize)
		}
	}
	return io.ReadAll(f)
}
