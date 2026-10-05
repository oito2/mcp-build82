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
	"strings"
)

// CapabilityCall is one has_capability()/require_capability() call site found in a plugin's PHP
// source, naming a capability whose prefix matches the plugin's own capability namespace — not a
// core or a different plugin's capability, which a plugin legitimately checks all the time.
type CapabilityCall struct {
	Capability string
	File       string // relative to pluginPath, forward slashes
	Line       int    // 1-based
}

var capabilityCallPattern = regexp.MustCompile(`\b(?:has_capability|require_capability)\s*\(\s*['"]([^'"]+)['"]`)

// CapabilityPrefix returns the capability-name prefix a plugin's own capabilities are declared
// under in db/access.php. For every plugin type except mod/block, this is the plugin's own
// frankenstyle component (e.g. "local_test", "tool_test"); mod/block capabilities use a
// legacy slash form instead ("mod/forum:...", "block/html:...", not "mod_forum:"/"block_html:").
func CapabilityPrefix(pluginType, name, component string) string {
	switch pluginType {
	case "mod", "block":
		return pluginType + "/" + name
	default:
		return component
	}
}

// FindOwnCapabilityChecks walks pluginPath recursively for *.php files and returns every
// has_capability()/require_capability() call whose string-literal capability name starts with
// ownPrefix+":" — checks that claim to name one of this plugin's own capabilities. Like
// FindDeprecatedApiUsage, this always uses a regex scan regardless of BUILD82_EXTRACTOR_BACKEND.
func FindOwnCapabilityChecks(pluginPath, ownPrefix string) []CapabilityCall {
	prefix := ownPrefix + ":"

	var calls []CapabilityCall
	walkPhpFiles(pluginPath, func(absPath, relSlash string) {
		calls = append(calls, scanFileForOwnCapabilityChecks(absPath, relSlash, prefix)...)
	})
	return calls
}

func scanFileForOwnCapabilityChecks(path, relFile, prefix string) []CapabilityCall {
	content, err := readFileCapped(path)
	if err != nil {
		return nil
	}
	s := string(content)

	// Matched over the whole file content, not line-by-line, so a call like
	// `has_capability(\n    'local/test:view',\n    $context\n);` is matched even when the capability
	// string literal is on a different line than `has_capability(`. capabilityCallPattern's `\s*`
	// spans newlines; the line number is derived from the match's byte offset.
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
