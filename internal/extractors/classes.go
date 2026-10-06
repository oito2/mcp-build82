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
	"github.com/oito2/mcp-build82/internal/phparray"
	"github.com/oito2/mcp-build82/internal/phptypes"
)

// ClassKind, PhpClass, RenamedClass and ClassesExtraction alias the phptypes types of the same name.
type ClassKind = phptypes.ClassKind
type PhpClass = phptypes.PhpClass
type RenamedClass = phptypes.RenamedClass
type ClassesExtraction = phptypes.ClassesExtraction

// Patterns used by the regex class scanner: the declaration keyword and name, the namespace
// statement, the extends and implements clauses, and one entry of a renamed-classes array.
var (
	kindPattern       = regexp.MustCompile(`(?m)^(abstract\s+class|class|interface|trait|enum)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	namespacePattern  = regexp.MustCompile(`(?m)^namespace\s+([a-zA-Z0-9_\\]+)\s*;`)
	extendsPattern    = regexp.MustCompile(`\bextends\s+([a-zA-Z_\\][a-zA-Z0-9_\\]*)`)
	implementsPattern = regexp.MustCompile(`(?m)\bimplements\s+([a-zA-Z_\\][a-zA-Z0-9_,\\\s]+?)(?:\{|$)`)

	renamedClassEntryPattern = regexp.MustCompile(`'([a-zA-Z0-9_]+)'\s*=>\s*([^,\]]+)`)
)

// maxClassDeclContinuationLines is how many lines after a declaration line are joined to find
// its extends/implements clauses when the opening brace is not on the declaration line.
const maxClassDeclContinuationLines = 8

// heredocStartPattern matches, anchored at the start of its input, a heredoc/nowdoc opening marker
// (<<<IDENT, <<<"IDENT" or <<<'IDENT') and the newline that ends the opening line, capturing the
// identifier. The optional quotes are matched independently because RE2 has no backreferences;
// this is a heuristic strip, not a syntax validator.
var heredocStartPattern = regexp.MustCompile(`^<<<[ \t]*['"]?([A-Za-z_][A-Za-z0-9_]*)['"]?[ \t]*\r?\n`)

// stripCommentsAndHeredocs returns `content` with every `/* ... */` block comment, every `//` or
// `#` line comment (the newline is kept) and every heredoc/nowdoc body replaced by blank filler: non-newline bytes become spaces and every '\n' is
// kept in place. This stops scanPhpClassFile's line-by-line kindPattern from matching a
// declaration-shaped line inside a comment or heredoc, while all other bytes and line numbers stay
// unchanged.
//
// Single- and double-quoted strings are tracked, but copied through unchanged, so text such as
// "/* not a comment */" or "<<<NOTREAL" inside a string is not taken for a comment or heredoc.
// A `#[` sequence starts a PHP 8 attribute, not a comment, and is copied through like code so
// quotes inside it keep the string tracking in sync.
func stripCommentsAndHeredocs(content string) string {
	var out strings.Builder
	out.Grow(len(content))

	var (
		quote          byte
		lineComment    bool
		blockComment   bool
		heredocClosing *regexp.Regexp // non-nil while inside a heredoc/nowdoc body
	)
	atLineStart := true
	n := len(content)

	for i := 0; i < n; {
		c := content[i]

		switch {
		case heredocClosing != nil:
			if atLineStart {
				if m := heredocClosing.FindStringIndex(content[i:]); m != nil && m[0] == 0 {
					out.WriteString(content[i+m[0] : i+m[1]])
					i += m[1]
					heredocClosing = nil
					atLineStart = false
					continue
				}
			}
			if c == '\n' {
				out.WriteByte('\n')
				atLineStart = true
			} else {
				out.WriteByte(' ')
				atLineStart = false
			}
			i++

		case quote != 0:
			if c == '\\' && i+1 < n {
				out.WriteByte(c)
				out.WriteByte(content[i+1])
				i += 2
				atLineStart = false
				continue
			}
			out.WriteByte(c)
			if c == quote {
				quote = 0
			}
			atLineStart = false
			i++

		case lineComment:
			if c == '\n' {
				out.WriteByte('\n')
				lineComment = false
				atLineStart = true
			} else {
				out.WriteByte(' ')
				atLineStart = false
			}
			i++

		case blockComment:
			if c == '*' && i+1 < n && content[i+1] == '/' {
				out.WriteString("  ")
				blockComment = false
				i += 2
				atLineStart = false
				continue
			}
			if c == '\n' {
				out.WriteByte('\n')
				atLineStart = true
			} else {
				out.WriteByte(' ')
				atLineStart = false
			}
			i++

		case c == '\'' || c == '"':
			quote = c
			out.WriteByte(c)
			atLineStart = false
			i++

		case c == '/' && i+1 < n && content[i+1] == '/':
			lineComment = true
			out.WriteString("  ")
			atLineStart = false
			i += 2

		case c == '#' && i+1 < n && content[i+1] == '[':
			out.WriteByte(c)
			atLineStart = false
			i++

		case c == '#':
			lineComment = true
			out.WriteByte(' ')
			atLineStart = false
			i++

		case c == '/' && i+1 < n && content[i+1] == '*':
			blockComment = true
			out.WriteString("  ")
			atLineStart = false
			i += 2

		case c == '<':
			if m := heredocStartPattern.FindStringSubmatchIndex(content[i:]); m != nil && m[0] == 0 {
				ident := content[i+m[2] : i+m[3]]
				out.WriteString(content[i : i+m[1]])
				i += m[1]
				heredocClosing = regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(ident) + `\b`)
				atLineStart = true
				continue
			}
			out.WriteByte(c)
			atLineStart = false
			i++

		default:
			out.WriteByte(c)
			atLineStart = c == '\n'
			i++
		}
	}
	return out.String()
}

// scanPhpClassFile returns the class, interface, trait and enum declarations found in the PHP file
// at `path`, recording `relFile` as their file. The namespace comes from the file's namespace
// statement, and the extends/implements clauses are read from the declaration line plus up to
// maxClassDeclContinuationLines following lines. It returns nil when the file cannot be read.
func scanPhpClassFile(path, relFile string) []PhpClass {
	content, err := readFileCapped(path)
	if err != nil {
		return nil
	}
	s := string(content)
	namespace := firstSubmatch(namespacePattern, s)

	lines := strings.Split(stripCommentsAndHeredocs(s), "\n")
	var classes []PhpClass

	for i := 0; i < len(lines); i++ {
		m := kindPattern.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		kind, name := ClassKind(m[1]), m[2]

		decl := lines[i]
		for j := 1; !strings.Contains(decl, "{") && j <= maxClassDeclContinuationLines && i+j < len(lines); j++ {
			decl += "\n" + lines[i+j]
		}

		extends := firstSubmatch(extendsPattern, decl)
		var implements []string
		if im := implementsPattern.FindStringSubmatch(decl); im != nil {
			for _, part := range strings.Split(im[1], ",") {
				part = strings.TrimSpace(part)
				if part != "" {
					implements = append(implements, part)
				}
			}
		}

		fqn := "\\" + name
		if namespace != "" {
			fqn = "\\" + namespace + "\\" + name
		}

		classes = append(classes, PhpClass{
			Name: name, Namespace: namespace, FQN: fqn, Kind: kind,
			File: relFile, Extends: extends, Implements: implements,
		})
	}
	return classes
}

// ExtractClasses walks `dirPath` recursively for *.php files and extracts every class, interface,
// trait and enum declaration. Each class's File is relative to `rootPath` (defaulting to `dirPath`
// when empty), so a plugin passing its own root while scanning classes/ gets paths such as
// "classes/task/foo.php". `globPattern` is only forwarded to the tree-sitter backend; the regex
// backend always walks `dirPath` recursively.
func ExtractClasses(dirPath, rootPath, globPattern string) ClassesExtraction {
	if useTreesitter() {
		return tsbackend.ExtractClasses(dirPath, rootPath, globPattern)
	}
	if rootPath == "" {
		rootPath = dirPath
	}

	var files []string
	walkPhpFiles(dirPath, func(absPath, _ string) {
		files = append(files, absPath)
	})
	return extractClassesFromFilesRegex(files, rootPath)
}

// ExtractClassesFromFiles extracts class declarations from exactly the PHP files in `files`
// instead of walking a directory, so callers that already resolved a restricted file set avoid
// parsing every *.php in the tree. Each class's File is relative to `rootPath`.
func ExtractClassesFromFiles(files []string, rootPath string) ClassesExtraction {
	if useTreesitter() {
		return tsbackend.ExtractClassesFromFiles(files, rootPath)
	}
	return extractClassesFromFilesRegex(files, rootPath)
}

// extractClassesFromFilesRegex scans `files` with the regex backend and returns the classes found,
// sorted by namespace and then name, with File relative to `rootPath`.
func extractClassesFromFilesRegex(files []string, rootPath string) ClassesExtraction {
	var classes []PhpClass
	for _, path := range files {
		rel, relErr := filepath.Rel(rootPath, path)
		if relErr != nil {
			rel = path
		}
		classes = append(classes, scanPhpClassFile(path, filepath.ToSlash(rel))...)
	}

	sort.Slice(classes, func(i, j int) bool {
		if classes[i].Namespace != classes[j].Namespace {
			return classes[i].Namespace < classes[j].Namespace
		}
		return classes[i].Name < classes[j].Name
	})

	return ClassesExtraction{Classes: classes}
}

// normalizeRenamedClassValue converts the right-hand side `raw` of a renamed-classes entry into a
// class name without a leading backslash. It accepts a bare `Ns\Class::class` reference, whose
// backslashes are already literal, or a quoted string literal such as 'block_test\\helper', which
// is unescaped like phparray.ExtractString (\\ to \, then \' to ').
func normalizeRenamedClassValue(raw string) string {
	v := strings.TrimSpace(raw)
	v = strings.TrimSuffix(v, "::class")
	v = strings.TrimSpace(v)

	if len(v) >= 2 && (v[0] == '\'' || v[0] == '"') && v[len(v)-1] == v[0] {
		v = v[1 : len(v)-1]
		v = phparray.UnescapeString(v)
	}

	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, `\`)
	return v
}

// ParseRenamedClassesPhp parses the $renamedclasses map of the db/renamedclasses.php file at
// `filePath` into old-name/new-name pairs, skipping entries with an empty new name. It returns nil
// when the file cannot be read or does not define the array.
func ParseRenamedClassesPhp(filePath string) []RenamedClass {
	if useTreesitter() {
		return tsbackend.ParseRenamedClassesPhp(filePath)
	}
	content, err := readFileCapped(filePath)
	if err != nil {
		return nil
	}
	body, ok := phparray.ExtractArrayBody(string(content), "renamedclasses")
	if !ok {
		return nil
	}

	var out []RenamedClass
	for _, m := range renamedClassEntryPattern.FindAllStringSubmatch(body, -1) {
		newName := normalizeRenamedClassValue(m[2])
		if newName == "" {
			continue
		}
		out = append(out, RenamedClass{OldName: m[1], NewName: newName})
	}
	return out
}

// ExtractPluginClasses extracts the class declarations under `pluginPath`/classes and the
// renamed-class map from `pluginPath`/db/renamedclasses.php.
func ExtractPluginClasses(pluginPath string) ClassesExtraction {
	extraction := ExtractClasses(filepath.Join(pluginPath, "classes"), pluginPath, "**/*.php")
	extraction.RenamedClasses = ParseRenamedClassesPhp(filepath.Join(pluginPath, "db", "renamedclasses.php"))
	return extraction
}

// GetClassFQNs returns the fully-qualified names of the classes in `e`, sorted.
func GetClassFQNs(e ClassesExtraction) []string {
	names := make([]string, len(e.Classes))
	for i, c := range e.Classes {
		names[i] = c.FQN
	}
	sort.Strings(names)
	return names
}
