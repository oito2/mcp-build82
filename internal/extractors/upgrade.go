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
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/oito2/mcp-build82/internal/extractors/tsbackend"
	"github.com/oito2/mcp-build82/internal/phparray"
	"github.com/oito2/mcp-build82/internal/phptypes"
)

// UpgradeStep and UpgradeExtraction alias the phptypes types of the same name.
type UpgradeStep = phptypes.UpgradeStep
type UpgradeExtraction = phptypes.UpgradeExtraction

// upgradeFnPattern matches the declaration of the xmldb_<component>_upgrade() function.
var upgradeFnPattern = regexp.MustCompile(`function\s+xmldb_\w+_upgrade\s*\(`)

// extractFunctionBody returns the body of the xmldb_<component>_upgrade() function in `content`,
// without the outer braces. The boolean is false when the function or its closing brace is not
// found.
func extractFunctionBody(content string) (string, bool) {
	loc := upgradeFnPattern.FindStringIndex(content)
	if loc == nil {
		return "", false
	}
	braceStart := strings.Index(content[loc[0]:], "{")
	if braceStart == -1 {
		return "", false
	}
	braceStart += loc[0]

	end := phparray.FindBalancedEnd(content, braceStart, '{', '}')
	if end == -1 {
		return "", false
	}
	return content[braceStart+1 : end], true
}

// Patterns used to read upgrade steps: the version-gated `if ($oldversion < N) {` opening, an
// inline comment on that line, an xmldb_table reference, and any `//` comment.
var (
	stepPattern              = regexp.MustCompile(`if\s*\(\s*\$oldversion\s*<\s*(\d{10})\s*\)\s*\{`)
	stepInlineCommentPattern = regexp.MustCompile(`if\s*\([^)]+\)\s*\{?\s*//\s*(.+)`)
	xmldbTableRefPattern     = regexp.MustCompile(`new\s+xmldb_table\s*\(\s*['"]([^'"]+)['"]`)
	bareCommentPattern       = regexp.MustCompile(`//\s*(.+)`)
)

// truncate120 trims whitespace from `s` and cuts the result to at most 120 bytes,
// backing up to a UTF-8 rune boundary so no multi-byte character is split.
func truncate120(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		cut := 120
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		return s[:cut]
	}
	return s
}

// extractStepBlock returns the brace-balanced if-block, braces included, whose opening brace is the
// last character of the stepPattern match ending at `matchEnd` in `body`. When the brace never
// closes it returns a raw slice of up to 500 bytes so a malformed step is still described.
func extractStepBlock(body string, matchEnd int) string {
	braceStart := matchEnd - 1
	end := phparray.FindBalancedEnd(body, braceStart, '{', '}')
	if end == -1 {
		limit := braceStart + 500
		if limit > len(body) {
			limit = len(body)
		}
		return body[braceStart:limit]
	}
	return body[braceStart : end+1]
}

// extractDescription returns a short description of an upgrade step from `ifLineWindow` (the text
// of its if line) and `stepBlock` (its body). It uses, in order, an inline comment on the if line,
// the first `new xmldb_table('name')` reference as "xmldb_table: name", or the first `//` comment
// in the block, and returns "" when none exists.
func extractDescription(ifLineWindow, stepBlock string) string {
	if m := stepInlineCommentPattern.FindStringSubmatch(ifLineWindow); m != nil {
		return truncate120(m[1])
	}
	if m := xmldbTableRefPattern.FindStringSubmatch(stepBlock); m != nil {
		return "xmldb_table: " + m[1]
	}
	if m := bareCommentPattern.FindStringSubmatch(stepBlock); m != nil {
		return truncate120(m[1])
	}
	return ""
}

// ParseUpgradePhp parses the version-gated steps of the xmldb_<component>_upgrade() function in the
// db/upgrade.php file at `filePath`, sorted by ascending version. It returns nil when the file
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

	body, ok := extractFunctionBody(s)
	if !ok {
		return &UpgradeExtraction{File: filePath, Steps: []UpgradeStep{}}
	}

	steps := []UpgradeStep{}
	for _, m := range stepPattern.FindAllStringSubmatchIndex(body, -1) {
		version := body[m[2]:m[3]]
		matchEnd := m[1]

		lineEnd := strings.IndexByte(body[matchEnd:], '\n')
		var ifLineWindow string
		if lineEnd == -1 {
			ifLineWindow = body[m[0]:]
		} else {
			ifLineWindow = body[m[0] : matchEnd+lineEnd]
		}

		block := extractStepBlock(body, matchEnd)
		steps = append(steps, UpgradeStep{
			Version:     version,
			Description: extractDescription(ifLineWindow, block),
		})
	}

	sort.SliceStable(steps, func(i, j int) bool {
		vi, _ := strconv.Atoi(steps[i].Version)
		vj, _ := strconv.Atoi(steps[j].Version)
		return vi < vj
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
