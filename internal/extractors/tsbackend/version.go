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
	"path/filepath"

	"github.com/odvcencio/gotreesitter"
)

// ReadVersionPhp reads pluginPath/version.php and extracts the four $plugin->* fields the
// regex-backend readVersionPhp also extracts, with the same signature and semantics: a field that's
// absent, or whose value isn't the expected node kind, comes back as an empty string rather than
// an error, so version.php parsing never fails the whole plugin detection. When a field is
// assigned more than once, the last assignment wins, as when PHP runs the file; comments are not
// part of the tree, so a commented-out assignment is never read.
func ReadVersionPhp(pluginPath string) (component, version, requires, maturity string) {
	tree, src, err := ParseFile(filepath.Join(pluginPath, "version.php"))
	if err != nil {
		return
	}
	defer tree.Release()
	root := tree.RootNode()

	if v := findLastAssignment(root, MatchPluginField(src, "component")); v != nil {
		component, _ = StringValue(v, src)
	}
	if v := findLastAssignment(root, MatchPluginField(src, "version")); v != nil {
		version = numberText(v, src)
	}
	if v := findLastAssignment(root, MatchPluginField(src, "requires")); v != nil {
		requires = numberText(v, src)
	}
	if v := findLastAssignment(root, MatchPluginField(src, "maturity")); v != nil && v.Type(phpLang) == "name" {
		maturity = v.Text(src)
	}
	return
}

// findLastAssignment is FindAssignment returning the value node of the last matching assignment
// in document order instead of the first.
func findLastAssignment(root *gotreesitter.Node, lhsMatch func(lhs *gotreesitter.Node) bool) *gotreesitter.Node {
	var found *gotreesitter.Node
	gotreesitter.Walk(root, func(n *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if n.Type(phpLang) != "assignment_expression" {
			return gotreesitter.WalkContinue
		}
		children := namedChildren(n)
		if len(children) == 2 && lhsMatch(children[0]) {
			found = children[1]
		}
		return gotreesitter.WalkContinue
	})
	return found
}

// numberText returns an `integer` or `float` node's literal text as-is (not round-tripped through
// strconv), matching the regex backend's own string-typed Version/Requires fields exactly,
// decimal part included.
func numberText(node *gotreesitter.Node, src []byte) string {
	if t := node.Type(phpLang); t != "integer" && t != "float" {
		return ""
	}
	return node.Text(src)
}
