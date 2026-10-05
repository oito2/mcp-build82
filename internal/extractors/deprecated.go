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
	"regexp"
	"sort"
	"strings"

	"github.com/oito2/mcp-build82/internal/phparray"
)

// DeprecatedCall is one call site found in a plugin's PHP source matching a core function name
// marked @deprecated in the global API index (see ExtractMoodleApi).
type DeprecatedCall struct {
	Function string
	File     string // relative to pluginPath, forward slashes
	Line     int    // 1-based
}

// FindDeprecatedApiUsage walks pluginPath recursively for *.php files and flags bare calls to any
// name in deprecated. Method calls (`->name(`) and static calls (`::name(`) are skipped, since
// they can only be a same-named plugin method or class constant rather than the core global
// function. This scan does not parse comments or string literals separately, so a deprecated name
// mentioned only in a comment can still be flagged. Always uses the regex scan regardless of
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

	// The alternation pattern is compiled once per distinct deprecated set: names is sorted before
	// being joined, so the assembled pattern string is a stable cache key for phparray.CachedPattern
	// (a sync.Map-backed cache), which compiles it once and reuses it afterwards. Safe for concurrent
	// use.
	pattern := phparray.CachedPattern(`\b(` + strings.Join(names, "|") + `)\s*\(`)

	var calls []DeprecatedCall
	walkPhpFiles(pluginPath, func(absPath, relSlash string) {
		calls = append(calls, scanFileForDeprecatedCalls(absPath, relSlash, pattern)...)
	})
	return calls
}

func scanFileForDeprecatedCalls(path, relFile string, pattern *regexp.Regexp) []DeprecatedCall {
	content, err := readFileCapped(path)
	if err != nil {
		return nil
	}
	s := string(content)

	// Matched over the whole file content, not line-by-line, so a call like
	// `get_context_instance\n    (CONTEXT_COURSE, 1);`, with the function name and its opening
	// paren split across lines, is still matched. The line number is derived from the match's byte
	// offset.
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
