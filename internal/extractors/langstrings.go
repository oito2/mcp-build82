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
	"strings"
)

// langStringKeyPattern matches every $string['key'] = ...; declaration in a lang file — every key,
// not just pluginname.
var langStringKeyPattern = regexp.MustCompile(`\$string\[['"]([a-zA-Z0-9_]+)['"]\]\s*=`)

// ExtractLangStrings returns the set of string identifiers declared in pluginPath's
// lang/en/{component}.php. Returns an empty (never nil) set if the file doesn't exist or declares
// nothing — callers don't need a separate "file exists" branch.
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

// GetStringCall is one get_string() call site found in a plugin's PHP source whose component
// argument is a string literal matching the plugin's own component — a call claiming to read one
// of this plugin's own lang strings, as opposed to a core or different plugin's string (which a
// plugin legitimately references all the time, e.g. get_string('save', 'core')).
type GetStringCall struct {
	Identifier string
	File       string // relative to pluginPath, forward slashes
	Line       int    // 1-based
}

var getStringCallPattern = regexp.MustCompile(`\bget_string\s*\(\s*['"]([a-zA-Z0-9_]+)['"]\s*,\s*['"]([^'"]+)['"]`)

// FindOwnGetStringCalls walks pluginPath recursively for *.php files and returns every get_string()
// call whose component argument is a string literal exactly matching component. Calls with no
// component argument, a different plugin/core's component, or a non-literal (variable) component
// argument are skipped, since none of those name this plugin's own lang file. Like
// FindDeprecatedApiUsage, always uses a regex scan regardless of BUILD82_EXTRACTOR_BACKEND.
func FindOwnGetStringCalls(pluginPath, component string) []GetStringCall {
	var calls []GetStringCall
	walkPhpFiles(pluginPath, func(absPath, relSlash string) {
		calls = append(calls, scanFileForOwnGetStringCalls(absPath, relSlash, component)...)
	})
	return calls
}

func scanFileForOwnGetStringCalls(path, relFile, component string) []GetStringCall {
	content, err := readFileCapped(path)
	if err != nil {
		return nil
	}
	s := string(content)

	// Matched over the whole file content, not line-by-line, so a call like
	// `get_string(\n    'pluginname',\n    'local_test'\n);`, with the identifier/component
	// literals on different lines than `get_string(`, is still matched. getStringCallPattern's `\s*`
	// spans newlines; the line number is derived from the match's byte offset.
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
