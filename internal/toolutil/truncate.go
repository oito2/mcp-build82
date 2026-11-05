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

package toolutil

import (
	"errors"
	"io"
	"unicode/utf8"

	"github.com/oito2/mcp-build82/internal/fsutil"
)

// ReadFileTruncated returns at most the first `maxChars` bytes of the file at `path`, without
// reading the rest of a large file into memory. When the file is cut, a UTF-8 character split by
// the cut is dropped instead of returned as invalid bytes. It returns "" when the file cannot be
// opened or read; callers treat that as "not available" rather than as an error. It never panics.
func ReadFileTruncated(path string, maxChars int) string {
	f, err := fsutil.OpenRegular(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	buf := make([]byte, maxChars)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return ""
	}
	content := buf[:n]
	if n == maxChars {
		content = trimSplitRune(content)
	}
	return string(content)
}

// trimSplitRune drops from the end of `b` the bytes of a UTF-8 character that is cut short: at most
// utf8.UTFMax-1 bytes, so invalid bytes elsewhere are left as they are.
func trimSplitRune(b []byte) []byte {
	for i := 0; i < utf8.UTFMax-1 && len(b) > 0; i++ {
		if r, size := utf8.DecodeLastRune(b); r != utf8.RuneError || size != 1 {
			break
		}
		b = b[:len(b)-1]
	}
	return b
}
