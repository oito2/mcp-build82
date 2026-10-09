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
	"syscall"
	"time"
)

// errorSharingViolation is the Windows ERROR_SHARING_VIOLATION code.
const errorSharingViolation syscall.Errno = 32

// renameRetryTimeout and renameRetryDelay bound how long, and how often, renameFile retries a
// rename that another process blocks.
const (
	renameRetryTimeout = time.Second
	renameRetryDelay   = 20 * time.Millisecond
)

// renameFile renames `oldpath` to `newpath` with os.Rename. Windows refuses to move a file that
// another process holds open without delete sharing (a reader or an antivirus scan) with a sharing
// violation or an access-denied error; such a rename is retried every renameRetryDelay for up to
// renameRetryTimeout. It returns the last error otherwise.
func renameFile(oldpath, newpath string) error {
	deadline := time.Now().Add(renameRetryTimeout)
	for {
		err := os.Rename(oldpath, newpath)
		if err == nil || !isRetryableRename(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(renameRetryDelay)
	}
}

// isRetryableRename reports whether `err` is a sharing violation or an access-denied error, the
// errors of a rename blocked by an open handle.
func isRetryableRename(err error) bool {
	return errors.Is(err, errorSharingViolation) || errors.Is(err, syscall.ERROR_ACCESS_DENIED)
}
