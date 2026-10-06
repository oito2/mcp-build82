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
	"path/filepath"
	"regexp"
)

// MoodleInstallInfo describes a detected Moodle installation, read from its version.php.
type MoodleInstallInfo struct {
	Version string // human-readable, e.g. "4.3"
	Build   string // numeric, e.g. "2023110900"
	Branch  string // e.g. "403"
	Release string // full $release string
}

// Patterns that read the $release, $version and $branch values of Moodle's version.php.
var (
	releasePattern = regexp.MustCompile(`\$release\s*=\s*['"]([^'"]+)['"]`)
	versionPattern = regexp.MustCompile(`\$version\s*=\s*([\d.]+)`)
	branchPattern  = regexp.MustCompile(`\$branch\s*=\s*['"]([^'"]+)['"]`)
	// versionPrefixPattern extracts a leading "4.3" or "4.3+" style prefix from the $release string.
	versionPrefixPattern = regexp.MustCompile(`^(\d+\.\d+[+.]?\d*)`)
)

// DetectMoodleInstall reads `moodlePath`/version.php and returns its install metadata. Version is
// the numeric prefix of $release, falling back to the build number when $release has none. It
// returns nil when the file cannot be read.
func DetectMoodleInstall(moodlePath string) *MoodleInstallInfo {
	content, err := readFileCapped(filepath.Join(moodlePath, "version.php"))
	if err != nil {
		return nil
	}
	s := string(content)

	release := firstSubmatch(releasePattern, s)
	build := firstSubmatch(versionPattern, s)
	branch := firstSubmatch(branchPattern, s)

	versionNum := firstSubmatch(versionPrefixPattern, release)
	if versionNum == "" {
		versionNum = build
	}

	return &MoodleInstallInfo{Version: versionNum, Build: build, Branch: branch, Release: release}
}

// IsMoodleRoot reports whether `dirPath` looks like a Moodle installation root: it contains
// version.php, a lib directory, and config.php or config-dist.php.
func IsMoodleRoot(dirPath string) bool {
	return fileExists(filepath.Join(dirPath, "version.php")) &&
		dirExists(filepath.Join(dirPath, "lib")) &&
		(fileExists(filepath.Join(dirPath, "config.php")) || fileExists(filepath.Join(dirPath, "config-dist.php")))
}
