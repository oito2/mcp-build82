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

package tsbackend

import (
	"path/filepath"

	"github.com/odvcencio/gotreesitter"
)

// ReadVersionPhp reads pluginPath/version.php and extracts the four $plugin->* fields the
// regex-backend readVersionPhp also extracts, with the same signature and semantics: a field that's
// absent, or whose value isn't the expected node kind, comes back as an empty string rather than
// an error, so version.php parsing never fails the whole plugin detection.
func ReadVersionPhp(pluginPath string) (component, version, requires, maturity string) {
	tree, src, err := ParseFile(filepath.Join(pluginPath, "version.php"))
	if err != nil {
		return
	}
	defer tree.Release()
	root := tree.RootNode()

	if v := FindAssignment(root, MatchPluginField(src, "component")); v != nil {
		component, _ = StringValue(v, src)
	}
	if v := FindAssignment(root, MatchPluginField(src, "version")); v != nil {
		version = integerText(v, src)
	}
	if v := FindAssignment(root, MatchPluginField(src, "requires")); v != nil {
		requires = integerText(v, src)
	}
	if v := FindAssignment(root, MatchPluginField(src, "maturity")); v != nil && v.Type(phpLang) == "name" {
		maturity = v.Text(src)
	}
	return
}

// integerText returns an `integer` node's literal text as-is (not round-tripped through strconv),
// matching the regex backend's own string-typed Version/Requires fields exactly.
func integerText(node *gotreesitter.Node, src []byte) string {
	if node.Type(phpLang) != "integer" {
		return ""
	}
	return node.Text(src)
}
