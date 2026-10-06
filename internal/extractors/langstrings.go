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
	"strings"
)

// langStringKeyPattern matches the key of every $string['key'] = ... declaration in a lang file.
var langStringKeyPattern = regexp.MustCompile(`\$string\[['"]([a-zA-Z0-9_]+)['"]\]\s*=`)

// ExtractLangStrings returns the set of string identifiers declared in
// `pluginPath`/lang/en/`component`.php. The set is empty, never nil, when the file cannot be read
// or declares nothing.
func ExtractLangStrings(pluginPath, component string) map[string]struct{} {
	declared := map[string]struct{}{}
	content, err := readFileCapped(filepath.Join(pluginPath, "lang", "en", component+".php"))
	if err != nil {
		return declared
	}
	for _, m := range langStringKeyPattern.FindAllStringSubmatch(string(content), -1) {
		declared[m[1]] = struct{}{}
	}
	return declared
}

// GetStringCall is one get_string() call site in a plugin's PHP source whose component argument is
// a string literal equal to the plugin's own component, i.e. a read of one of its own lang strings.
type GetStringCall struct {
	Identifier string
	File       string // relative to pluginPath, forward slashes
	Line       int    // 1-based
}

// getStringCallPattern matches a get_string() call with two string-literal arguments, capturing the
// identifier and the component. `\s*` lets the arguments span lines.
var getStringCallPattern = regexp.MustCompile(`\bget_string\s*\(\s*['"]([a-zA-Z0-9_]+)['"]\s*,\s*['"]([^'"]+)['"]`)

// FindOwnGetStringCalls walks `pluginPath` recursively for *.php files and returns every
// get_string() call whose component argument is a string literal equal to `component`. Calls with
// no component argument, another component, or a non-literal component are skipped. Like
// FindDeprecatedApiUsage, it always uses a regex scan regardless of BUILD82_EXTRACTOR_BACKEND.
func FindOwnGetStringCalls(pluginPath, component string) []GetStringCall {
	var calls []GetStringCall
	walkPhpFiles(pluginPath, func(absPath, relSlash string) {
		calls = append(calls, scanFileForOwnGetStringCalls(absPath, relSlash, component)...)
	})
	return calls
}

// scanFileForOwnGetStringCalls returns the get_string() calls in the file at `path` whose component
// is `component`, recording `relFile` as their file. It returns nil when the file cannot be read.
func scanFileForOwnGetStringCalls(path, relFile, component string) []GetStringCall {
	content, err := readFileCapped(path)
	if err != nil {
		return nil
	}
	s := string(content)

	// The pattern runs over the whole file rather than line by line so arguments on different
	// lines still match; the line number comes from the match's byte offset.
	var calls []GetStringCall
	for _, m := range getStringCallPattern.FindAllStringSubmatchIndex(s, -1) {
		if s[m[4]:m[5]] == component {
			calls = append(calls, GetStringCall{
				Identifier: s[m[2]:m[3]],
				File:       relFile,
				Line:       strings.Count(s[:m[0]], "\n") + 1,
			})
		}
	}
	return calls
}
