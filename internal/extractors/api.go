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
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/oito2/mcp-build82/internal/extractors/tsbackend"
	"github.com/oito2/mcp-build82/internal/phpdoc"
	"github.com/oito2/mcp-build82/internal/phptypes"
)

// ApiVisibility is an alias for phpdoc.Visibility. The parseDocBlock/classifyVisibility logic lives
// in internal/phpdoc, shared with the tree-sitter backend. Aliasing keeps references to
// extractors.ApiVisibility and extractors.PhpDocBlock working unchanged.
type ApiVisibility = phpdoc.Visibility

const (
	VisPublic     = phpdoc.Public
	VisDeprecated = phpdoc.Deprecated
	VisInternal   = phpdoc.Internal
	VisPrivate    = phpdoc.Private
	VisUnverified = phpdoc.Unverified
)

// PhpDocBlock is an alias for phpdoc.PhpDocBlock.
type PhpDocBlock = phpdoc.PhpDocBlock

// ApiFunction is an alias for phptypes.ApiFunction; the tree-sitter backend returns this same
// type directly.
type ApiFunction = phptypes.ApiFunction

type ApiCounts struct {
	Public, Deprecated, Internal, Private, Unverified int
}

type ApiExtraction struct {
	Directory string
	Functions []ApiFunction
	Counts    ApiCounts
}

// findDocBlock walks backward from the function declaration line at funcLineIndex to find the
// immediately preceding PHPDoc block, tolerating up to 3 blank lines. Returns nil if none is found.
func findDocBlock(lines []string, funcLineIndex int) *PhpDocBlock {
	blankCount := 0
	closeIndex := -1

	for i := funcLineIndex - 1; i >= 0 && i >= funcLineIndex-5; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			blankCount++
			if blankCount > 3 {
				break
			}
			continue
		}
		if trimmed == "*/" {
			closeIndex = i
			break
		}
		break
	}
	if closeIndex == -1 {
		return nil
	}

	openIndex := -1
	for i := closeIndex - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "/**") || trimmed == "/*" {
			openIndex = i
			break
		}
		if !strings.HasPrefix(trimmed, "*") && trimmed != "" {
			break
		}
	}
	if openIndex == -1 {
		return nil
	}

	raw := strings.Join(lines[openIndex:closeIndex+1], "\n")
	doc := phpdoc.ParseDocBlock(raw)
	return &doc
}

// funcPattern matches only top-level (non-indented) function declarations.
var funcPattern = regexp.MustCompile(`^function\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)

func scanPhpFile(filePath string) []ApiFunction {
	if useTreesitter() {
		return tsbackend.ExtractFunctionsFromPhpFile(filePath)
	}
	content, err := readFileCapped(filePath)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(content), "\n")
	file := filepath.Base(filePath)

	var results []ApiFunction
	for i, line := range lines {
		m := funcPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		if strings.HasPrefix(name, "__") {
			continue
		}
		doc := findDocBlock(lines, i)
		results = append(results, ApiFunction{
			Name: name, File: file, FilePath: filePath,
			Visibility: phpdoc.ClassifyVisibility(name, doc),
			Doc:        doc,
			Line:       i + 1,
		})
	}
	return results
}

// PriorityFiles is a fixed, ordered list of well-known Moodle core lib filenames, scanned first
// (in this order), which determines the grouping order in the generated API index.
var PriorityFiles = []string{
	"moodlelib.php", "accesslib.php", "filelib.php", "weblib.php", "gradelib.php",
	"completionlib.php", "enrollib.php", "grouplib.php", "datalib.php", "outputlib.php",
	"navigationlib.php", "formslib.php", "filterlib.php", "messagelib.php", "badgeslib.php",
	"blocklib.php", "cronlib.php", "dmllib.php", "eventslib.php", "externallib.php",
	"grade/gradelib.php",
}

// getPhpFiles returns every .php file directly inside dirPath (never recursing into
// subdirectories — ExtractMoodleApi's top-level {moodlePath}/lib scan is the only caller, and it
// deliberately excludes lib/classes/* and other subdirectories), with PriorityFiles listed first.
func getPhpFiles(dirPath string) []string {
	if !dirExists(dirPath) {
		return nil
	}

	var files []string
	seen := map[string]struct{}{}

	for _, pf := range PriorityFiles {
		full := filepath.Join(dirPath, pf)
		if fileExists(full) {
			files = append(files, full)
			seen[full] = struct{}{}
		}
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return files
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".php") {
			full := filepath.Join(dirPath, entry.Name())
			if _, ok := seen[full]; !ok {
				files = append(files, full)
			}
		}
	}
	return files
}

var visibilityOrder = map[ApiVisibility]int{
	VisPublic: 0, VisDeprecated: 1, VisUnverified: 2, VisInternal: 3, VisPrivate: 4,
}

// ExtractMoodleApi scans every .php file in {moodlePath}/lib (priority-ordered), classifies every
// function, computes Counts before filtering, then returns only public+deprecated functions —
// private, internal, and unverified are always excluded from Functions.
func ExtractMoodleApi(moodlePath string) ApiExtraction {
	libPath := filepath.Join(moodlePath, "lib")
	phpFiles := getPhpFiles(libPath)

	var all []ApiFunction
	for _, f := range phpFiles {
		all = append(all, scanPhpFile(f)...)
	}

	var counts ApiCounts
	for _, f := range all {
		switch f.Visibility {
		case VisPublic:
			counts.Public++
		case VisDeprecated:
			counts.Deprecated++
		case VisInternal:
			counts.Internal++
		case VisPrivate:
			counts.Private++
		case VisUnverified:
			counts.Unverified++
		}
	}

	functions := make([]ApiFunction, 0, len(all))
	for _, f := range all {
		if f.Visibility == VisPublic || f.Visibility == VisDeprecated {
			functions = append(functions, f)
		}
	}

	sort.SliceStable(functions, func(i, j int) bool {
		a, b := functions[i], functions[j]
		if visibilityOrder[a.Visibility] != visibilityOrder[b.Visibility] {
			return visibilityOrder[a.Visibility] < visibilityOrder[b.Visibility]
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Name < b.Name
	})

	return ApiExtraction{Directory: libPath, Functions: functions, Counts: counts}
}

// ExtractFunctionsFromPhpFile scans a single PHP file for top-level function declarations,
// returning every function unfiltered (unlike ExtractMoodleApi).
func ExtractFunctionsFromPhpFile(filePath string) []ApiFunction {
	if !fileExists(filePath) {
		return nil
	}
	return scanPhpFile(filePath)
}
