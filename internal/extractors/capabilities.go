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

	"github.com/oito2/mcp-build82/internal/extractors/tsbackend"
	"github.com/oito2/mcp-build82/internal/phparray"
	"github.com/oito2/mcp-build82/internal/phptypes"
)

// Capability and CapabilitiesExtraction are aliases for phptypes' types.
type Capability = phptypes.Capability
type CapabilitiesExtraction = phptypes.CapabilitiesExtraction

// capabilityKeyPattern is single-quote only (no double-quote alternative); Moodle capability
// keys are conventionally single-quoted, and this asymmetry is deliberate.
var capabilityKeyPattern = regexp.MustCompile(`'([a-zA-Z0-9_/]+:[a-zA-Z0-9_]+)'\s*=>\s*(\[|array\s*\()`)

var (
	archetypeKeyPattern   = regexp.MustCompile(`['"](archetypes)['"]\s*=>\s*(\[|array\s*\()`)
	archetypeEntryPattern = regexp.MustCompile(`'([a-zA-Z_]+)'\s*=>\s*([A-Z_0-9]+)`)
)

// capabilityStringPatterns pre-compiles the string/constant pattern pair for each of the fixed
// set of keys extractCapabilityString is ever called with (captype, contextlevel, riskbitmask),
// instead of recompiling both regexes on every call — avoiding 6 regexp compiles per capability
// entry.
type capabilityStringPatternPair struct {
	str, constPattern *regexp.Regexp
}

var capabilityStringPatterns = map[string]capabilityStringPatternPair{
	"captype":      newCapabilityStringPatternPair("captype"),
	"contextlevel": newCapabilityStringPatternPair("contextlevel"),
	"riskbitmask":  newCapabilityStringPatternPair("riskbitmask"),
}

func newCapabilityStringPatternPair(key string) capabilityStringPatternPair {
	quoted := regexp.QuoteMeta(key)
	return capabilityStringPatternPair{
		str:          regexp.MustCompile(`['"]` + quoted + `['"]\s*=>\s*['"]([^'"]+)['"]`),
		constPattern: regexp.MustCompile(`['"]` + quoted + `['"]\s*=>\s*([A-Z_0-9]+)`),
	}
}

// extractCapabilityString is a local variant of phparray.ExtractString: it first tries a
// quoted-string value, and if that fails, falls back to a bare PHP constant
// (CONTEXT_COURSE, CAP_ALLOW, etc — capabilities files commonly use constants for these fields).
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

// ParseAccessPhp parses a db/access.php file. Returns nil if the file can't be read.
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

// ExtractPluginCapabilities parses pluginPath/db/access.php.
func ExtractPluginCapabilities(pluginPath string) *CapabilitiesExtraction {
	return ParseAccessPhp(filepath.Join(pluginPath, "db", "access.php"))
}

// GetCapabilityNames returns the sorted list of capability names. Safe to call with a nil e (the
// plugin has no db/access.php, the common case) — returns nil rather than panicking.
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
