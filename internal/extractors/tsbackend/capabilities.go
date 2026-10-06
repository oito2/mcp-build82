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

package tsbackend

import (
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/oito2/mcp-build82/internal/phptypes"
)

// ParseAccessPhp parses a db/access.php file's $capabilities map — keyed by capability name, each
// value an array with a nested archetypes sub-array. Unlike the regex backend, capability keys are
// matched regardless of quote style (single or double). Every matched entry is included; there's
// no "requires field X" gating rule here, unlike events/tasks.
func ParseAccessPhp(filePath string) *phptypes.CapabilitiesExtraction {
	tree, src, err := ParseFile(filePath)
	if err != nil {
		return nil
	}
	defer tree.Release()

	capsArray := FindAssignment(tree.RootNode(), MatchVariable(src, "capabilities"))
	if capsArray == nil {
		return &phptypes.CapabilitiesExtraction{File: filePath, Capabilities: []phptypes.Capability{}}
	}

	caps := []phptypes.Capability{}
	for _, entry := range ArrayElements(capsArray) {
		key, inner := KeyValue(entry)
		name, ok := StringValue(key, src)
		if !ok || inner == nil {
			continue
		}
		caps = append(caps, parseCapability(name, inner, src))
	}
	return &phptypes.CapabilitiesExtraction{File: filePath, Capabilities: caps}
}

// parseCapability reads one capability definition's keyed fields. captype defaults "read",
// matching the regex backend exactly.
func parseCapability(name string, inner *gotreesitter.Node, src []byte) phptypes.Capability {
	capability := phptypes.Capability{Name: name, CapType: "read", Archetypes: map[string]string{}}

	for _, element := range ArrayElements(inner) {
		key, value := KeyValue(element)
		k, ok := StringValue(key, src)
		if !ok {
			continue
		}
		switch k {
		case "captype":
			if s, ok := StringValue(value, src); ok {
				capability.CapType = orDefault(s, "read")
			}
		case "contextlevel":
			capability.ContextLevel = capabilityStringOrConstant(value, src)
		case "riskbitmask":
			capability.RiskBitmask = capabilityStringOrConstant(value, src)
		case "archetypes":
			capability.Archetypes = parseArchetypes(value, src)
		}
	}
	return capability
}

// parseArchetypes reads an archetypes sub-array into a role-to-permission map (for example
// "manager" => CAP_ALLOW). Entries whose key is not a string are skipped; a nil node yields an
// empty map.
func parseArchetypes(node *gotreesitter.Node, src []byte) map[string]string {
	result := map[string]string{}
	for _, element := range ArrayElements(node) {
		key, value := KeyValue(element)
		k, ok := StringValue(key, src)
		if !ok {
			continue
		}
		result[k] = capabilityStringOrConstant(value, src)
	}
	return result
}

// capabilityStringOrConstant mirrors internal/extractors's extractCapabilityString: try a quoted
// string first, else fall back to the value's raw source text — a bare constant like
// CONTEXT_COURSE, or a compound bitwise-OR expression like RISK_SPAM | RISK_PERSONAL. Tree-sitter
// returns the *full* expression text; the regex backend's constant fallback only captures the
// first ALL_CAPS token of a compound expression.
func capabilityStringOrConstant(node *gotreesitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	if s, ok := StringValue(node, src); ok {
		return s
	}
	return strings.TrimSpace(node.Text(src))
}
