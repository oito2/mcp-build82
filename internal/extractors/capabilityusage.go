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
	"strings"
)

// CapabilityCall is one has_capability()/require_capability() call site found in a plugin's PHP
// source that names a capability under the plugin's own capability namespace.
type CapabilityCall struct {
	Capability string
	File       string // relative to pluginPath, forward slashes
	Line       int    // 1-based
}

// capabilityCallPattern matches a has_capability()/require_capability() call whose first argument
// is a string literal, capturing the capability name. `\s*` lets the literal sit on a later line.
var capabilityCallPattern = regexp.MustCompile(`\b(?:has_capability|require_capability)\s*\(\s*['"]([^'"]+)['"]`)

// CapabilityPrefix returns the prefix under which a plugin declares its own capabilities in
// db/access.php, given its `pluginType`, `name` and frankenstyle `component`. For mod and block
// plugins it is the slash form "type/name" (e.g. "mod/forum"); for every other type it is
// `component` (e.g. "local_test").
func CapabilityPrefix(pluginType, name, component string) string {
	switch pluginType {
	case "mod", "block":
		return pluginType + "/" + name
	default:
		return component
	}
}

// FindOwnCapabilityChecks walks `pluginPath` recursively for *.php files and returns every
// has_capability()/require_capability() call whose string-literal capability name starts with
// `ownPrefix` followed by ":". Like FindDeprecatedApiUsage, it always uses a regex scan regardless
// of BUILD82_EXTRACTOR_BACKEND.
func FindOwnCapabilityChecks(pluginPath, ownPrefix string) []CapabilityCall {
	prefix := ownPrefix + ":"

	var calls []CapabilityCall
	walkPhpFiles(pluginPath, func(absPath, relSlash string) {
		calls = append(calls, scanFileForOwnCapabilityChecks(absPath, relSlash, prefix)...)
	})
	return calls
}

// scanFileForOwnCapabilityChecks returns the capability calls in the file at `path` whose name
// starts with `prefix`, recording `relFile` as their file. It returns nil when the file cannot be
// read.
func scanFileForOwnCapabilityChecks(path, relFile, prefix string) []CapabilityCall {
	content, err := readFileCapped(path)
	if err != nil {
		return nil
	}
	s := string(content)

	// The pattern runs over the whole file rather than line by line so a literal on a different
	// line than `has_capability(` still matches; the line number comes from the match's byte offset.
	var calls []CapabilityCall
	for _, m := range capabilityCallPattern.FindAllStringSubmatchIndex(s, -1) {
		capability := s[m[2]:m[3]]
		if strings.HasPrefix(capability, prefix) {
			calls = append(calls, CapabilityCall{
				Capability: capability,
				File:       relFile,
				Line:       strings.Count(s[:m[0]], "\n") + 1,
			})
		}
	}
	return calls
}
