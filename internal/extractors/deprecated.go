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
	"regexp"
	"sort"
	"strings"

	"github.com/oito2/mcp-build82/internal/phparray"
)

// DeprecatedCall is one call site in a plugin's PHP source of a core function that is marked
// @deprecated (see ExtractMoodleApi).
type DeprecatedCall struct {
	Function string
	File     string // relative to pluginPath, forward slashes
	Line     int    // 1-based
}

// FindDeprecatedApiUsage walks `pluginPath` recursively for *.php files and returns every bare
// call to a function named in the `deprecated` set. Method calls (`->name(`) and static calls
// (`::name(`) are skipped because they cannot be the core global function. Comments and string
// literals are not parsed separately, so a call written inside one can still be reported. It
// returns nil when `deprecated` is empty and always uses a regex scan regardless of
// BUILD82_EXTRACTOR_BACKEND.
func FindDeprecatedApiUsage(pluginPath string, deprecated map[string]struct{}) []DeprecatedCall {
	if len(deprecated) == 0 {
		return nil
	}

	names := make([]string, 0, len(deprecated))
	for name := range deprecated {
		names = append(names, regexp.QuoteMeta(name))
	}
	sort.Strings(names) // deterministic pattern string; match order is unaffected

	// names is sorted, so the joined pattern string is a stable cache key and
	// phparray.CachedPattern compiles each distinct set only once.
	pattern := phparray.CachedPattern(`\b(` + strings.Join(names, "|") + `)\s*\(`)

	var calls []DeprecatedCall
	walkPhpFiles(pluginPath, func(absPath, relSlash string) {
		calls = append(calls, scanFileForDeprecatedCalls(absPath, relSlash, pattern)...)
	})
	return calls
}

// scanFileForDeprecatedCalls returns the calls matched by `pattern` in the file at `path`,
// recording `relFile` as their file and skipping method and static calls. It returns nil when the
// file cannot be read.
func scanFileForDeprecatedCalls(path, relFile string, pattern *regexp.Regexp) []DeprecatedCall {
	content, err := readFileCapped(path)
	if err != nil {
		return nil
	}
	s := string(content)

	// The pattern runs over the whole file rather than line by line so a call whose name and
	// opening parenthesis are on different lines still matches; the line number comes from the
	// match's byte offset.
	var calls []DeprecatedCall
	for _, loc := range pattern.FindAllStringSubmatchIndex(s, -1) {
		start := loc[2]
		if start >= 2 {
			if prefix := s[start-2 : start]; prefix == "->" || prefix == "::" {
				continue
			}
		}
		calls = append(calls, DeprecatedCall{
			Function: s[loc[2]:loc[3]],
			File:     relFile,
			Line:     strings.Count(s[:loc[0]], "\n") + 1,
		})
	}
	return calls
}
