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
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/oito2/mcp-build82/internal/extractors/tsbackend"
	"github.com/oito2/mcp-build82/internal/phparray"
	"github.com/oito2/mcp-build82/internal/phptypes"
)

// UpgradeStep and UpgradeExtraction are aliases for phptypes' types.
type UpgradeStep = phptypes.UpgradeStep
type UpgradeExtraction = phptypes.UpgradeExtraction

var upgradeFnPattern = regexp.MustCompile(`function\s+xmldb_\w+_upgrade\s*\(`)

// extractFunctionBody finds the xmldb_{component}_upgrade() function and returns its body
// (between the outer braces, exclusive). Returns ("", false) if the function or its closing brace
// can't be found.
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

var (
	stepPattern              = regexp.MustCompile(`if\s*\(\s*\$oldversion\s*<\s*(\d{10})\s*\)\s*\{`)
	stepInlineCommentPattern = regexp.MustCompile(`if\s*\([^)]+\)\s*\{?\s*//\s*(.+)`)
	xmldbTableRefPattern     = regexp.MustCompile(`new\s+xmldb_table\s*\(\s*['"]([^'"]+)['"]`)
	bareCommentPattern       = regexp.MustCompile(`//\s*(.+)`)
)

func truncate120(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		return s[:120]
	}
	return s
}

// extractStepBlock returns the bracket-balanced body of the if-block whose opening brace is the
// last character of the stepPattern match ending at matchEnd. Falls back to a 500-char raw slice if
// the brace never closes, so extractDescription still has something to scan for a malformed step
// instead of the step being dropped. (extractFunctionBody, by contrast, fails outright when its
// closing brace is missing.)
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

// extractDescription tries, in order: (1) an inline comment on the if line itself; (2) the first
// new xmldb_table('name') reference; (3) the first bare "// comment" anywhere in the block; else
// empty string.
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

// ParseUpgradePhp parses a db/upgrade.php file's xmldb_{component}_upgrade() function body for
// version-gated upgrade steps. Returns nil if the file can't be read.
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

	sort.Slice(steps, func(i, j int) bool {
		vi, _ := strconv.Atoi(steps[i].Version)
		vj, _ := strconv.Atoi(steps[j].Version)
		return vi < vj
	})

	return &UpgradeExtraction{File: filePath, Steps: steps}
}

// ExtractPluginUpgrade parses pluginPath/db/upgrade.php.
func ExtractPluginUpgrade(pluginPath string) *UpgradeExtraction {
	return ParseUpgradePhp(filepath.Join(pluginPath, "db", "upgrade.php"))
}

// GetUpgradeVersions returns the step versions in their already-sorted order, without re-sorting.
// Safe to call with a nil e (the plugin has no db/upgrade.php) — returns nil rather than panicking.
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
