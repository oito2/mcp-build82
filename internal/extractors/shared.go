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
	"regexp"
	"sort"
	"strings"

	"github.com/oito2/mcp-build82/internal/fsutil"
)

// MaxReadableFileSize is the largest file, in bytes, that the extractors and generators buffer
// into memory when reading plugin or Moodle source. It bounds memory use when scanning potentially
// untrusted third-party PHP source.
const MaxReadableFileSize = 8 << 20 // 8 MiB

// readFileCapped returns the content of the regular file at `path`, never following a symbolic
// link or waiting on a FIFO (see fsutil.ReadRegular). It returns an error if the file cannot be
// opened or read, is not a regular file, or is larger than MaxReadableFileSize.
func readFileCapped(path string) ([]byte, error) {
	return fsutil.ReadRegular(path, MaxReadableFileSize)
}

// firstSubmatch returns the first capture group of the first match of `re` in `s`, or "" when
// there is no match. `re` must contain at least one capture group.
func firstSubmatch(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}

// orDefault returns `def` when `s` is empty and `s` otherwise. It supplies the fixed default for
// fields a PHP array entry may omit (e.g. captype defaulting to "read", cron fields to "*").
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// fileExists reports whether `path` exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dirExists reports whether `path` exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// walkPhpFiles calls `visit` for every *.php file under `rootPath` (fsutil.WalkFiles: symbolic
// links skipped), with its full path and its slash-separated path relative to `rootPath`. Both
// backends' *.php-tree scanners walk with fsutil.WalkFiles.
func walkPhpFiles(rootPath string, visit func(absPath, relSlash string)) {
	fsutil.WalkFiles(rootPath, ".php", visit)
}

// PhpFile is one *.php file found by ListPhpFiles: its full path and its path relative to the
// walked root, slash-separated.
type PhpFile struct {
	Path, Rel string
}

// ListPhpFiles returns the *.php files under `rootPath` as walkPhpFiles finds them (symbolic links
// skipped), so several scanners can share a single walk.
func ListPhpFiles(rootPath string) []PhpFile {
	var files []PhpFile
	walkPhpFiles(rootPath, func(absPath, relSlash string) {
		files = append(files, PhpFile{Path: absPath, Rel: relSlash})
	})
	return files
}

// splitAndTrimCommaList splits the comma-separated list `s` of (possibly quoted) items, strips
// surrounding quotes and whitespace from each item, and drops items that end up empty.
func splitAndTrimCommaList(s string) []string {
	var out []string
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, `'"`)
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

// precedingDocBlock returns the lines of the PHPDoc block ("/**" or "/*" to "*/") that immediately
// precedes the declaration at index `declIdx` of `lines`, joined with "\n". Between the block and
// the declaration it allows up to 3 blank lines and any PHP 8 attributes, single-line ("#[...]")
// or spanning several lines (see attributeStart). A closing line such as "**/" counts as the end
// of the block. ok is false when no such block precedes the declaration.
func precedingDocBlock(lines []string, declIdx int) (block string, ok bool) {
	blank, closeIdx := 0, -1
scan:
	for i := declIdx - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		switch {
		case trimmed == "":
			blank++
			if blank > 3 {
				return "", false
			}
		case strings.HasPrefix(trimmed, "#["):
			// A single-line attribute, or the first line of one.
		case strings.HasSuffix(trimmed, "]"):
			start := attributeStart(lines, i)
			if start < 0 {
				return "", false
			}
			i = start
		case strings.HasSuffix(trimmed, "*/"):
			closeIdx = i
			break scan
		default:
			return "", false
		}
	}
	if closeIdx < 0 {
		return "", false
	}
	for i := closeIdx; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "/**") || trimmed == "/*" {
			return strings.Join(lines[i:closeIdx+1], "\n"), true
		}
		if i != closeIdx && !strings.HasPrefix(trimmed, "*") && trimmed != "" {
			return "", false
		}
	}
	return "", false
}

// attributeStart returns the index of the "#[" line that opens the multi-line PHP 8 attribute
// ending at index `end` of `lines`, looking back at most 50 lines, or -1 when there is none or a
// line in between ends a statement or opens/closes a block (so the "]" did not end an attribute).
func attributeStart(lines []string, end int) int {
	for j := end; j >= 0 && j >= end-50; j-- {
		trimmed := strings.TrimSpace(lines[j])
		if strings.HasPrefix(trimmed, "#[") {
			return j
		}
		if strings.HasSuffix(trimmed, ";") || strings.ContainsAny(trimmed, "{}") {
			return -1
		}
	}
	return -1
}

// lineIndex holds the byte offsets of every line break of a text, in increasing order, so the line
// of any offset is found by binary search instead of counting line breaks from the start.
type lineIndex []int

// newLineIndex returns the lineIndex of `s`.
func newLineIndex(s string) lineIndex {
	idx := lineIndex{}
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			idx = append(idx, i)
		}
	}
	return idx
}

// line returns the 1-based line number of byte offset `pos`: one more than the number of line
// breaks before it.
func (idx lineIndex) line(pos int) int {
	return sort.SearchInts(idx, pos) + 1
}
