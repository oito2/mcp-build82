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
// call to a function named in the `deprecated` set. Matches that cannot be the core global
// function are skipped (see isNotGlobalCall): method and static calls, function declarations,
// class instantiations, variable calls and namespaced names. Comments and string
// literals are not parsed separately, so a call written inside one can still be reported. It
// returns nil when `deprecated` is empty and always uses a regex scan regardless of
// BUILD82_EXTRACTOR_BACKEND.
func FindDeprecatedApiUsage(pluginPath string, deprecated map[string]struct{}) []DeprecatedCall {
	if len(deprecated) == 0 {
		return nil
	}
	return FindDeprecatedApiUsageIn(ListPhpFiles(pluginPath), deprecated)
}

// FindDeprecatedApiUsageIn is FindDeprecatedApiUsage over `files`, already listed with
// ListPhpFiles.
func FindDeprecatedApiUsageIn(files []PhpFile, deprecated map[string]struct{}) []DeprecatedCall {
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
	for _, f := range files {
		if s, ok := f.text(); ok {
			calls = append(calls, scanForDeprecatedCalls(s, f.Rel, pattern)...)
		}
	}
	return calls
}

// isNotGlobalCall reports whether the name starting at byte `start` of `s`, followed by `(`, is
// something other than a call to a global function: a method or static call (`->name(`,
// `?->name(`, `::name(`), a function declaration (`function name(`), a class instantiation
// (`new name(`), a variable call (`$name(`), or a name in a namespace (`ns\name(`). A fully
// qualified global name (`\name(`) is a global call.
func isNotGlobalCall(s string, start int) bool {
	if start > 0 {
		switch s[start-1] {
		case '$':
			return true
		case '\\':
			// `ns\name(` names a namespaced function; `\name(` is the global one.
			return start >= 2 && isIdentByte(s[start-2])
		}
	}
	before := strings.TrimRight(s[:start], " \t\r\n")
	if strings.HasSuffix(before, "->") || strings.HasSuffix(before, "::") {
		return true
	}
	for _, keyword := range []string{"function", "new"} {
		if len(before) >= len(keyword) && strings.EqualFold(before[len(before)-len(keyword):], keyword) {
			rest := before[:len(before)-len(keyword)]
			if rest == "" || !isIdentByte(rest[len(rest)-1]) {
				return true
			}
		}
	}
	return false
}

// scanFileForDeprecatedCalls returns the calls matched by `pattern` in the file at `path`,
// recording `relFile` as their file and skipping method and static calls. It returns nil when the
// file cannot be read.
func scanFileForDeprecatedCalls(path, relFile string, pattern *regexp.Regexp) []DeprecatedCall {
	content, err := readFileCapped(path)
	if err != nil {
		return nil
	}
	return scanForDeprecatedCalls(string(content), relFile, pattern)
}

// scanForDeprecatedCalls is scanFileForDeprecatedCalls over the file content `s`.
func scanForDeprecatedCalls(s, relFile string, pattern *regexp.Regexp) []DeprecatedCall {

	// The pattern runs over the whole file rather than line by line so a call whose name and
	// opening parenthesis are on different lines still matches; the line number comes from the
	// match's byte offset.
	var calls []DeprecatedCall
	var lines lineIndex
	for _, loc := range pattern.FindAllStringSubmatchIndex(s, -1) {
		if isNotGlobalCall(s, loc[2]) {
			continue
		}
		if lines == nil {
			lines = newLineIndex(s)
		}
		calls = append(calls, DeprecatedCall{
			Function: s[loc[2]:loc[3]],
			File:     relFile,
			Line:     lines.line(loc[0]),
		})
	}
	return calls
}
