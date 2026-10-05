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
	"strings"

	"github.com/oito2/mcp-build82/internal/extractors/tsbackend"
	"github.com/oito2/mcp-build82/internal/phparray"
	"github.com/oito2/mcp-build82/internal/phptypes"
)

// ClassKind, PhpClass, RenamedClass, and ClassesExtraction are aliases for phptypes' types.
type ClassKind = phptypes.ClassKind
type PhpClass = phptypes.PhpClass
type RenamedClass = phptypes.RenamedClass
type ClassesExtraction = phptypes.ClassesExtraction

var (
	kindPattern       = regexp.MustCompile(`(?m)^(abstract\s+class|class|interface|trait|enum)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	namespacePattern  = regexp.MustCompile(`(?m)^namespace\s+([a-zA-Z0-9_\\]+)\s*;`)
	extendsPattern    = regexp.MustCompile(`\bextends\s+([a-zA-Z_\\][a-zA-Z0-9_\\]*)`)
	implementsPattern = regexp.MustCompile(`(?m)\bimplements\s+([a-zA-Z_\\][a-zA-Z0-9_,\\\s]+?)(?:\{|$)`)

	renamedClassEntryPattern = regexp.MustCompile(`'([a-zA-Z0-9_]+)'\s*=>\s*([^,\]]+)`)
)

const maxClassDeclContinuationLines = 8

// heredocStartPattern matches a heredoc/nowdoc opening marker at the exact position it starts —
// <<<IDENT, <<<"IDENT", or <<<'IDENT' — followed by the newline PHP requires to end the opening
// line. The identifier is captured; the (optional) quote characters are matched independently
// rather than with a backreference since Go's RE2 engine doesn't support them, which is fine here
// since this is a heuristic strip, not a syntax validator.
var heredocStartPattern = regexp.MustCompile(`^<<<[ \t]*['"]?([A-Za-z_][A-Za-z0-9_]*)['"]?[ \t]*\r?\n`)

// stripCommentsAndHeredocs returns content with every `/* ... */` block comment and the interior
// of every heredoc/nowdoc (`<<<IDENT ... IDENT;`) body replaced by blank filler — non-newline
// bytes become spaces, every '\n' is kept exactly where it was — so scanPhpClassFile's line-by-line
// kindPattern does not mistake a lookalike "class Foo {"-shaped line inside a comment or
// heredoc body for a real declaration, while every other part of the file (including line numbers
// anything downstream might derive from it) is left completely unshifted.
//
// Single- and double-quoted string literals are tracked (but copied through unchanged) purely so a
// string like "/* not a comment */" or "<<<NOTREAL" embedded in ordinary PHP code isn't mistaken
// for the start of a real block comment or heredoc; `//`/`#` line comments are also tracked (and
// copied through unchanged) so an apostrophe inside one (e.g. "// don't touch this") doesn't
// desynchronize the string-quote tracking for the rest of the file.
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
			out.WriteByte(c)
			if c == '\n' {
				lineComment = false
				atLineStart = true
			} else {
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
			out.WriteString("//")
			atLineStart = false
			i += 2

		case c == '#':
			lineComment = true
			out.WriteByte('#')
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

// ExtractClasses walks dirPath recursively for *.php files and extracts every class/interface/
// trait/enum declaration. rootPath is used to compute File as relative to the right root — plugin
// extractors pass the plugin root even when scanning the classes/ subdirectory, so File comes out
// as "classes/task/foo.php", not "task/foo.php". globPattern is accepted for API compatibility with
// other callers but this implementation always walks dirPath recursively (the only pattern ever used in
// practice is "**/*.php").
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

// ExtractClassesFromFiles scans exactly the given files instead of walking a directory recursively
// — callers that already resolved a restricted glob (e.g. **/classes/**/*.php across a whole Moodle
// install) avoid parsing every *.php in the tree just to discard everything outside it.
func ExtractClassesFromFiles(files []string, rootPath string) ClassesExtraction {
	if useTreesitter() {
		return tsbackend.ExtractClassesFromFiles(files, rootPath)
	}
	return extractClassesFromFilesRegex(files, rootPath)
}

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

// normalizeRenamedClassValue accepts either a bare `Ns\Class::class` reference (real PHP code —
// backslashes are already literal, single separators, nothing to unescape) or a quoted string
// literal (e.g. 'block_test\\helper') — in the quoted case, PHP single-quoted-string unescaping
// applies just like phparray.ExtractString (\\ -> \, then \' -> '), since this is source text, not
// already-evaluated PHP.
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

// ParseRenamedClassesPhp parses a db/renamedclasses.php file's $renamedclasses map. Returns nil if
// the file can't be read or doesn't define the array.
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

// ExtractPluginClasses scans pluginPath/classes for class declarations and pluginPath/db/renamedclasses.php
// for the plugin's renamed-class autoload map.
func ExtractPluginClasses(pluginPath string) ClassesExtraction {
	extraction := ExtractClasses(filepath.Join(pluginPath, "classes"), pluginPath, "**/*.php")
	extraction.RenamedClasses = ParseRenamedClassesPhp(filepath.Join(pluginPath, "db", "renamedclasses.php"))
	return extraction
}

// GetClassFQNs returns the sorted list of fully-qualified class names.
func GetClassFQNs(e ClassesExtraction) []string {
	names := make([]string, len(e.Classes))
	for i, c := range e.Classes {
		names[i] = c.FQN
	}
	sort.Strings(names)
	return names
}
