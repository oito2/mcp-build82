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
	"sort"
	"strings"

	"github.com/oito2/mcp-build82/internal/extractors/tsbackend"
	"github.com/oito2/mcp-build82/internal/moodletype"
	"github.com/oito2/mcp-build82/internal/phparray"
	"github.com/oito2/mcp-build82/internal/phpdoc"
	"github.com/oito2/mcp-build82/internal/phptypes"
)

// UpgradeStep and UpgradeExtraction alias the phptypes types of the same name.
type UpgradeStep = phptypes.UpgradeStep
type UpgradeExtraction = phptypes.UpgradeExtraction

// upgradeFnPattern matches the declaration of the xmldb_<component>_upgrade() function.
var upgradeFnPattern = regexp.MustCompile(`function\s+xmldb_\w+_upgrade\s*\(`)

// functionBodyBounds returns the byte range [start, end) of the body of the
// xmldb_<component>_upgrade() function in `content`, without the outer braces. ok is false when
// the function or its closing brace is not found.
func functionBodyBounds(content string) (start, end int, ok bool) {
	loc := upgradeFnPattern.FindStringIndex(content)
	if loc == nil {
		return 0, 0, false
	}
	braceStart := strings.Index(content[loc[0]:], "{")
	if braceStart == -1 {
		return 0, 0, false
	}
	braceStart += loc[0]

	closing := phparray.FindBalancedEnd(content, braceStart, '{', '}')
	if closing == -1 {
		return 0, 0, false
	}
	return braceStart + 1, closing, true
}

// Patterns used to describe upgrade steps: an inline comment on the step's if line, an
// xmldb_table reference, and any `//` comment.
var (
	stepInlineCommentPattern = regexp.MustCompile(`if\s*\([^)]+\)\s*\{?\s*//\s*(.+)`)
	xmldbTableRefPattern     = regexp.MustCompile(`new\s+xmldb_table\s*\(\s*['"]([^'"]+)['"]`)
	bareCommentPattern       = regexp.MustCompile(`//\s*(.+)`)
)

// extractDescription returns a short description of an upgrade step from `ifLineWindow` (the text
// of its if line) and `stepBlock` (its body). It uses, in order, an inline comment on the if line,
// the first `new xmldb_table('name')` reference as "xmldb_table: name", or the first `//` comment
// in the block, and returns "" when none exists.
func extractDescription(ifLineWindow, stepBlock string) string {
	if m := stepInlineCommentPattern.FindStringSubmatch(ifLineWindow); m != nil {
		return phpdoc.CommentSummary(m[1])
	}
	if m := xmldbTableRefPattern.FindStringSubmatch(stepBlock); m != nil {
		return "xmldb_table: " + m[1]
	}
	if m := bareCommentPattern.FindStringSubmatch(stepBlock); m != nil {
		return phpdoc.CommentSummary(m[1])
	}
	return ""
}

// ParseUpgradePhp parses the version-gated steps of the xmldb_<component>_upgrade() function in the
// db/upgrade.php file at `filePath`, sorted by ascending version (a ten-digit version, optionally
// with a decimal part). Steps are located by locateUpgradeSteps, the locator ParseUpgradeOperations
// uses, in the text with comments and heredoc bodies blanked out (stripCommentsAndHeredocs, which keeps every offset), so a
// commented-out step is not reported and a quote inside a heredoc cannot hide the steps after it;
// descriptions are read from the original text at the same offsets. It returns nil when the file
// cannot be read, and an empty extraction when the function is not found.
func ParseUpgradePhp(filePath string) *UpgradeExtraction {
	if useTreesitter() {
		return tsbackend.ParseUpgradePhp(filePath)
	}
	content, err := readFileCapped(filePath)
	if err != nil {
		return nil
	}
	s := string(content)
	stripped := stripCommentsAndHeredocs(s)

	bodyStart, bodyEnd, ok := functionBodyBounds(stripped)
	if !ok {
		return &UpgradeExtraction{File: filePath, Steps: []UpgradeStep{}}
	}
	steps := []UpgradeStep{}
	for _, step := range locateUpgradeSteps(stripped) {
		if step.start < bodyStart || step.bodyEnd > bodyEnd {
			continue // outside the upgrade function
		}
		// The if line, from the `if` to the end of its line, read from the original text so an
		// inline comment on it is kept.
		lineEnd := strings.IndexByte(s[step.bodyStart:], '\n')
		ifLineWindow := s[step.start:]
		if lineEnd != -1 {
			ifLineWindow = s[step.start : step.bodyStart+lineEnd]
		}
		steps = append(steps, UpgradeStep{
			Version:     step.version,
			Description: extractDescription(ifLineWindow, s[step.bodyStart-1:step.bodyEnd+1]),
		})
	}

	sort.SliceStable(steps, func(i, j int) bool {
		return moodletype.UpgradeVersionLess(steps[i].Version, steps[j].Version)
	})

	return &UpgradeExtraction{File: filePath, Steps: steps}
}

// ExtractPluginUpgrade parses `pluginPath`/db/upgrade.php. It returns nil when that file cannot be
// read.
func ExtractPluginUpgrade(pluginPath string) *UpgradeExtraction {
	return ParseUpgradePhp(filepath.Join(pluginPath, "db", "upgrade.php"))
}

// GetUpgradeVersions returns the step versions of `e` in their existing order. It returns nil when
// `e` is nil, which is the case for a plugin without db/upgrade.php.
func GetUpgradeVersions(e *UpgradeExtraction) []string {
	if e == nil {
		return nil
	}
	versions := make([]string, len(e.Steps))
	for i, s := range e.Steps {
		versions[i] = s.Version
	}
	return versions
}
