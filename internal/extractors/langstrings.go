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
	"slices"

	"github.com/oito2/mcp-build82/internal/moodletype"
)

// langStringKeyPattern matches the key of every $string['key'] = ... declaration in a lang file.
var langStringKeyPattern = regexp.MustCompile(`\$string\[['"]([a-zA-Z0-9_]+)['"]\]\s*=`)

// ExtractLangStrings returns the set of string identifiers declared in the English language file
// of the plugin `component`, `pluginPath`/lang/en/<moodletype.LangFileName(component)>.php. The set
// is empty, never nil, when the file cannot be read or declares nothing.
func ExtractLangStrings(pluginPath, component string) map[string]struct{} {
	declared := map[string]struct{}{}
	content, err := readFileCapped(filepath.Join(pluginPath, "lang", "en", moodletype.LangFileName(component)+".php"))
	if err != nil {
		return declared
	}
	for _, m := range langStringKeyPattern.FindAllStringSubmatch(string(content), -1) {
		declared[m[1]] = struct{}{}
	}
	return declared
}

// GetStringCall is one get_string() call site in a plugin's PHP source whose component argument is
// a string literal naming the plugin itself (any of moodletype.ComponentNames), i.e. a read of one
// of its own lang strings.
type GetStringCall struct {
	Identifier string
	File       string // relative to pluginPath, forward slashes
	Line       int    // 1-based
}

// getStringCallPattern matches a get_string() call with two string-literal arguments, capturing the
// identifier and the component. `\s*` lets the arguments span lines.
var getStringCallPattern = regexp.MustCompile(`\bget_string\s*\(\s*['"]([a-zA-Z0-9_]+)['"]\s*,\s*['"]([^'"]+)['"]`)

// FindOwnGetStringCalls walks `pluginPath` recursively for *.php files and returns every
// get_string() call whose component argument is a string literal naming `component` (any of
// moodletype.ComponentNames, so both "mod_forum" and "forum" for a module). Calls with
// no component argument, another component, or a non-literal component are skipped. Like
// FindDeprecatedApiUsage, it always uses a regex scan regardless of BUILD82_EXTRACTOR_BACKEND.
func FindOwnGetStringCalls(pluginPath, component string) []GetStringCall {
	return FindOwnGetStringCallsIn(ListPhpFiles(pluginPath), component)
}

// FindOwnGetStringCallsIn is FindOwnGetStringCalls over `files`, already listed with ListPhpFiles.
func FindOwnGetStringCallsIn(files []PhpFile, component string) []GetStringCall {
	names := moodletype.ComponentNames(component)
	var calls []GetStringCall
	for _, f := range files {
		if s, ok := f.text(); ok {
			calls = append(calls, scanForOwnGetStringCalls(s, f.Rel, names)...)
		}
	}
	return calls
}

// scanFileForOwnGetStringCalls returns the get_string() calls in the file at `path` whose component
// is one of `names`, recording `relFile` as their file. It returns nil when the file cannot be read.
func scanFileForOwnGetStringCalls(path, relFile string, names []string) []GetStringCall {
	content, err := readFileCapped(path)
	if err != nil {
		return nil
	}
	return scanForOwnGetStringCalls(string(content), relFile, names)
}

// scanForOwnGetStringCalls is scanFileForOwnGetStringCalls over the file content `s`.
func scanForOwnGetStringCalls(s, relFile string, names []string) []GetStringCall {

	// The pattern runs over the whole file rather than line by line so arguments on different
	// lines still match; the line number comes from the match's byte offset.
	var calls []GetStringCall
	var lines lineIndex
	for _, m := range getStringCallPattern.FindAllStringSubmatchIndex(s, -1) {
		if slices.Contains(names, s[m[4]:m[5]]) {
			if lines == nil {
				lines = newLineIndex(s)
			}
			calls = append(calls, GetStringCall{
				Identifier: s[m[2]:m[3]],
				File:       relFile,
				Line:       lines.line(m[0]),
			})
		}
	}
	return calls
}
