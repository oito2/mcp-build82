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

package extractors

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// maxReadableFileSize caps how large a file readFileCapped will buffer into memory, bounding
// memory use when scanning a plugin's (potentially untrusted, third-party) PHP source.
const maxReadableFileSize = 8 << 20 // 8 MiB

// readFileCapped reads a file's content, refusing to buffer files larger than
// maxReadableFileSize to bound memory use when scanning untrusted plugin code.
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

func firstSubmatch(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}

// orDefault returns def when s is empty, s otherwise. Used for fields that fall back to a fixed
// default when a PHP array entry omits the key (e.g. captype defaulting "read", cron fields
// defaulting "*").
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// walkPhpFiles walks rootPath recursively and calls visit for every *.php file found, with that
// file's absolute path and its path relative to rootPath (forward-slash normalized). Walk errors,
// directories, and symlinked files are silently skipped — symlinked *files* specifically matter
// because os.ReadFile follows a symlink to wherever it points, so a symlinked "foo.php" planted
// inside a scanned plugin directory would otherwise get its target's content parsed and included
// in the scan. Shared by every *.php-tree scanner in this package (ExtractClasses,
// FindDeprecatedApiUsage, FindOwnCapabilityChecks, FindOwnGetStringCalls).
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

// splitAndTrimCommaList splits a comma-separated list of (possibly quoted) items, stripping
// surrounding quotes and whitespace from each and dropping any that end up empty.
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
