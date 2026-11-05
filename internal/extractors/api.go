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
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/oito2/mcp-build82/internal/extractors/tsbackend"
	"github.com/oito2/mcp-build82/internal/phpdoc"
	"github.com/oito2/mcp-build82/internal/phptypes"
)

// ApiVisibility is an alias for phpdoc.Visibility. The PHPDoc parsing and visibility classification
// live in internal/phpdoc, shared with the tree-sitter backend.
type ApiVisibility = phpdoc.Visibility

// Visibility classes an API function can have; they alias the phpdoc constants.
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

// ApiCounts holds how many scanned functions fall into each visibility class.
type ApiCounts struct {
	Public, Deprecated, Internal, Private, Unverified int
}

// ApiExtraction is the result of scanning Moodle's lib directory: the scanned directory, the
// public and deprecated functions found, and the counts for every visibility class.
type ApiExtraction struct {
	Directory string
	Functions []ApiFunction
	Counts    ApiCounts
}

// findDocBlock returns the parsed PHPDoc block that immediately precedes the function declaration
// at index `funcLineIndex` of `lines` (see precedingDocBlock: blank lines and PHP 8 attributes in
// between are allowed), or nil when there is none.
func findDocBlock(lines []string, funcLineIndex int) *PhpDocBlock {
	raw, ok := precedingDocBlock(lines, funcLineIndex)
	if !ok {
		return nil
	}
	doc := phpdoc.ParseDocBlock(raw)
	return &doc
}

// funcPattern matches only top-level (non-indented) function declarations.
var funcPattern = regexp.MustCompile(`^function\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)

// scanPhpFile returns every top-level function declared in the PHP file `filePath`, with its
// classified visibility and 1-based line number. Functions whose names start with "__" are
// skipped. It returns nil when the file cannot be read or exceeds the size cap.
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

// getPhpFiles returns the absolute paths of the .php files directly inside `dirPath`, sorted by
// name, without recursing into subdirectories. It returns nil when `dirPath` is not a directory
// or cannot be read. ExtractMoodleApi sorts its own output, so the order only makes the scan
// deterministic.
func getPhpFiles(dirPath string) []string {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".php") {
			files = append(files, filepath.Join(dirPath, entry.Name()))
		}
	}
	return files
}

// visibilityOrder gives the sort rank of each visibility class in ExtractMoodleApi's output.
var visibilityOrder = map[ApiVisibility]int{
	VisPublic: 0, VisDeprecated: 1, VisUnverified: 2, VisInternal: 3, VisPrivate: 4,
}

// ExtractMoodleApi scans every .php file directly inside `moodlePath`/lib and classifies every
// function. Counts covers all functions, while Functions keeps only the
// public and deprecated ones, sorted by visibility, file and name.
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

// ExtractFunctionsFromPhpFile returns every top-level function declared in the PHP file
// `filePath`, regardless of visibility (unlike ExtractMoodleApi). It returns nil when the file does
// not exist.
func ExtractFunctionsFromPhpFile(filePath string) []ApiFunction {
	if !fileExists(filePath) {
		return nil
	}
	return scanPhpFile(filePath)
}
