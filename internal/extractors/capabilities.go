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

	"github.com/oito2/mcp-build82/internal/extractors/tsbackend"
	"github.com/oito2/mcp-build82/internal/phparray"
	"github.com/oito2/mcp-build82/internal/phptypes"
)

// Capability and CapabilitiesExtraction alias the phptypes types of the same name.
type Capability = phptypes.Capability
type CapabilitiesExtraction = phptypes.CapabilitiesExtraction

// capabilityKeyPattern matches a single-quoted capability key followed by the opening of its array.
// Double-quoted keys are intentionally not matched.
var capabilityKeyPattern = regexp.MustCompile(`'([a-zA-Z0-9_/]+:[a-zA-Z0-9_]+)'\s*=>\s*(\[|array\s*\()`)

// archetypeKeyPattern matches the opening of a capability's "archetypes" array, and
// archetypeEntryPattern matches one `'role' => CAP_*` entry inside it.
var (
	archetypeKeyPattern   = regexp.MustCompile(`['"](archetypes)['"]\s*=>\s*(\[|array\s*\()`)
	archetypeEntryPattern = regexp.MustCompile(`'([a-zA-Z_]+)'\s*=>\s*([A-Z_0-9]+)`)
)

// capabilityStringPatternPair holds the quoted-string and bare-constant regexes for one array key.
type capabilityStringPatternPair struct {
	str, constPattern *regexp.Regexp
}

// capabilityStringPatterns caches the compiled pattern pair for each key extractCapabilityString
// is called with (captype, contextlevel, riskbitmask), avoiding recompilation on every call.
var capabilityStringPatterns = map[string]capabilityStringPatternPair{
	"captype":      newCapabilityStringPatternPair("captype"),
	"contextlevel": newCapabilityStringPatternPair("contextlevel"),
	"riskbitmask":  newCapabilityStringPatternPair("riskbitmask"),
}

// newCapabilityStringPatternPair compiles the quoted-string and bare-constant patterns that
// extract the value of the array key `key`.
func newCapabilityStringPatternPair(key string) capabilityStringPatternPair {
	quoted := regexp.QuoteMeta(key)
	return capabilityStringPatternPair{
		str:          regexp.MustCompile(`['"]` + quoted + `['"]\s*=>\s*['"]([^'"]+)['"]`),
		constPattern: regexp.MustCompile(`['"]` + quoted + `['"]\s*=>\s*([A-Z_0-9]+)`),
	}
}

// extractCapabilityString returns the value of the array key `key` inside the capability entry
// `block`. It first tries a quoted-string value and falls back to a bare PHP constant such as
// CONTEXT_COURSE, which access.php files commonly use. It returns "" when neither is found.
func extractCapabilityString(block, key string) string {
	pair, ok := capabilityStringPatterns[key]
	if !ok {
		pair = newCapabilityStringPatternPair(key)
	}
	if m := pair.str.FindStringSubmatch(block); m != nil {
		return m[1]
	}
	if m := pair.constPattern.FindStringSubmatch(block); m != nil {
		return m[1]
	}
	return ""
}

// extractArchetypes returns the role-to-permission map (e.g. "manager" to CAP_ALLOW) from the
// "archetypes" array of the capability entry `block`; the map is empty when there is none.
func extractArchetypes(block string) map[string]string {
	result := map[string]string{}
	entries := phparray.SplitKeyedEntries(block, archetypeKeyPattern)
	if len(entries) == 0 {
		return result
	}
	for _, m := range archetypeEntryPattern.FindAllStringSubmatch(entries[0].Body, -1) {
		result[m[1]] = m[2]
	}
	return result
}

// ParseAccessPhp parses the db/access.php file at `filePath` into its declared capabilities. The
// captype defaults to "read" when omitted. It returns nil when the file cannot be read, and an
// empty extraction when the file has no $capabilities array.
func ParseAccessPhp(filePath string) *CapabilitiesExtraction {
	if useTreesitter() {
		return tsbackend.ParseAccessPhp(filePath)
	}
	content, err := readFileCapped(filePath)
	if err != nil {
		return nil
	}
	body, ok := phparray.ExtractArrayBody(string(content), "capabilities")
	if !ok {
		return &CapabilitiesExtraction{File: filePath, Capabilities: []Capability{}}
	}

	caps := []Capability{}
	for _, entry := range phparray.SplitKeyedEntries(body, capabilityKeyPattern) {
		caps = append(caps, Capability{
			Name:         entry.Key,
			CapType:      orDefault(extractCapabilityString(entry.Body, "captype"), "read"),
			ContextLevel: extractCapabilityString(entry.Body, "contextlevel"),
			RiskBitmask:  extractCapabilityString(entry.Body, "riskbitmask"),
			Archetypes:   extractArchetypes(entry.Body),
		})
	}
	return &CapabilitiesExtraction{File: filePath, Capabilities: caps}
}

// ExtractPluginCapabilities parses `pluginPath`/db/access.php. It returns nil when that file
// cannot be read.
func ExtractPluginCapabilities(pluginPath string) *CapabilitiesExtraction {
	return ParseAccessPhp(filepath.Join(pluginPath, "db", "access.php"))
}

// GetCapabilityNames returns the capability names of `e` in sorted order. It returns nil when `e`
// is nil, which is the case for a plugin without db/access.php.
func GetCapabilityNames(e *CapabilitiesExtraction) []string {
	if e == nil {
		return nil
	}
	names := make([]string, len(e.Capabilities))
	for i, c := range e.Capabilities {
		names[i] = c.Name
	}
	sort.Strings(names)
	return names
}
