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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// maxReadableFileSize is the largest file, in bytes, that readFileCapped will buffer into memory.
// It bounds memory use when scanning potentially untrusted third-party PHP source.
const maxReadableFileSize = 8 << 20 // 8 MiB

// readFileCapped returns the content of the file at `path`. It returns an error if the file cannot
// be stat'ed or read, or if it is larger than maxReadableFileSize.
func readFileCapped(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxReadableFileSize {
		return nil, fmt.Errorf("file %s exceeds max readable size (%d > %d bytes)", path, info.Size(), maxReadableFileSize)
	}
	return os.ReadFile(path)
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

// walkPhpFiles walks `rootPath` recursively and calls `visit` for every *.php file found, passing
// the file's absolute path and its path relative to `rootPath` (forward-slash normalized). Walk
// errors, directories and symlinked files are silently skipped; symlinks are skipped because
// os.ReadFile follows them, so a planted "foo.php" link would otherwise pull its target's content
// into the scan. It is shared by every *.php-tree scanner in this package.
func walkPhpFiles(rootPath string, visit func(absPath, relSlash string)) {
	_ = filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(d.Name(), ".php") {
			return nil
		}
		rel, relErr := filepath.Rel(rootPath, path)
		if relErr != nil {
			rel = path
		}
		visit(path, filepath.ToSlash(rel))
		return nil
	})
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
