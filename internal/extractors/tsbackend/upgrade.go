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
	"sort"
	"strconv"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/oito2/mcp-build82/internal/phptypes"
)

// ParseUpgradePhp parses a db/upgrade.php file's xmldb_{component}_upgrade() function body for
// version-gated `if ($oldversion < NNNNNNNNNN) { ... }` steps — the first extractor reading
// control flow rather than an array literal or class structure. Returns nil if the file can't be
// read.
func ParseUpgradePhp(filePath string) *phptypes.UpgradeExtraction {
	tree, src, err := ParseFile(filePath)
	if err != nil {
		return nil
	}
	defer tree.Release()
	root := tree.RootNode()

	fn := FindDescendant(root, func(n *gotreesitter.Node) bool {
		if n.Type(phpLang) != "function_definition" {
			return false
		}
		nameNode := FirstChildOfType(n, "name")
		if nameNode == nil {
			return false
		}
		name := nameNode.Text(src)
		return strings.HasPrefix(name, "xmldb_") && strings.HasSuffix(name, "_upgrade")
	})
	if fn == nil {
		return &phptypes.UpgradeExtraction{File: filePath, Steps: []phptypes.UpgradeStep{}}
	}
	body := FirstChildOfType(fn, "compound_statement")
	if body == nil {
		return &phptypes.UpgradeExtraction{File: filePath, Steps: []phptypes.UpgradeStep{}}
	}

	steps := []phptypes.UpgradeStep{}
	for _, ifStmt := range findOldversionIfs(body, src) {
		version, _ := oldversionThreshold(ifStmt, src) // already filtered by findOldversionIfs
		block := FirstChildOfType(ifStmt, "compound_statement")
		steps = append(steps, phptypes.UpgradeStep{
			Version:     version,
			Description: upgradeDescription(ifStmt, block, src),
		})
	}

	sort.Slice(steps, func(i, j int) bool {
		vi, _ := strconv.Atoi(steps[i].Version)
		vj, _ := strconv.Atoi(steps[j].Version)
		return vi < vj
	})
	return &phptypes.UpgradeExtraction{File: filePath, Steps: steps}
}

// findOldversionIfs walks body's whole subtree (not just its direct statements — the regex
// backend's own flat text search doesn't care about nesting depth either) and collects every
// if_statement whose condition matches $oldversion < <10-digit integer>.
func findOldversionIfs(body *gotreesitter.Node, src []byte) []*gotreesitter.Node {
	var found []*gotreesitter.Node
	gotreesitter.Walk(body, func(n *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if n.Type(phpLang) == "if_statement" {
			if _, ok := oldversionThreshold(n, src); ok {
				found = append(found, n)
			}
		}
		return gotreesitter.WalkContinue
	})
	return found
}

// oldversionThreshold extracts the version number from an if_statement's condition, requiring the
// exact shape `$oldversion < NNNNNNNNNN` (10 digits) — same restriction as the regex backend's own
// `\d{10}`.
func oldversionThreshold(ifStmt *gotreesitter.Node, src []byte) (string, bool) {
	paren := FirstChildOfType(ifStmt, "parenthesized_expression")
	if paren == nil || paren.NamedChildCount() == 0 {
		return "", false
	}
	cond := paren.NamedChild(0)
	if cond.Type(phpLang) != "binary_expression" || cond.NamedChildCount() != 2 || !hasOperatorToken(cond, src, "<") {
		return "", false
	}
	left, right := cond.NamedChild(0), cond.NamedChild(1)
	if left.Type(phpLang) != "variable_name" || left.Text(src) != "$oldversion" {
		return "", false
	}
	if right.Type(phpLang) != "integer" || len(right.Text(src)) != 10 {
		return "", false
	}
	return right.Text(src), true
}

// hasOperatorToken reports whether node has an unnamed (anonymous) direct child whose text is
// exactly op — used to distinguish "<" from other binary_expression operators sharing the same
// node type (e.g. "<=", "&&", ".").
func hasOperatorToken(node *gotreesitter.Node, src []byte, op string) bool {
	for i := 0; i < node.ChildCount(); i++ {
		c := node.Child(i)
		if !c.IsNamed() && c.Text(src) == op {
			return true
		}
	}
	return false
}

// upgradeDescription mirrors the regex backend's own 3-tier extractDescription fallback exactly:
// (1) a comment on the same source line as the if statement itself; (2) the first `new
// xmldb_table('name')` reference anywhere in the step block; (3) the first bare `//` comment
// anywhere in the step block; else empty.
func upgradeDescription(ifStmt, block *gotreesitter.Node, src []byte) string {
	if block == nil {
		return ""
	}
	ifRow := ifStmt.StartPoint().Row

	if block.NamedChildCount() > 0 {
		if first := block.NamedChild(0); first.StartPoint().Row == ifRow {
			if text, ok := lineCommentText(first, src); ok {
				return truncate120(text)
			}
		}
	}

	if tableName, ok := firstXmldbTableRef(block, src); ok {
		return "xmldb_table: " + tableName
	}

	if c := FindDescendant(block, func(n *gotreesitter.Node) bool {
		_, ok := lineCommentText(n, src)
		return ok
	}); c != nil {
		text, _ := lineCommentText(c, src)
		return truncate120(text)
	}

	return ""
}

// lineCommentText returns a `//`-style comment's text with the marker and surrounding whitespace
// stripped. Block comments (/* ... */) don't count, matching the regex backend's own
// `//`-anchored patterns exactly.
func lineCommentText(node *gotreesitter.Node, src []byte) (string, bool) {
	if node.Type(phpLang) != "comment" {
		return "", false
	}
	text := node.Text(src)
	if !strings.HasPrefix(text, "//") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(text, "//")), true
}

func truncate120(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		return s[:120]
	}
	return s
}

// firstXmldbTableRef finds the first `new xmldb_table('name')` object_creation_expression
// anywhere in block whose argument is a string literal, and returns that table name. A call like
// `new xmldb_table($oldname)` (a variable argument) is not a match and the search continues to the
// next occurrence, rather than stopping at the first object_creation_expression regardless of its
// argument shape.
func firstXmldbTableRef(block *gotreesitter.Node, src []byte) (string, bool) {
	var result string
	var found bool
	gotreesitter.Walk(block, func(n *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if found {
			return gotreesitter.WalkStop
		}
		if n.Type(phpLang) != "object_creation_expression" {
			return gotreesitter.WalkContinue
		}
		nameNode := FirstChildOfType(n, "name")
		if nameNode == nil || nameNode.Text(src) != "xmldb_table" {
			return gotreesitter.WalkContinue
		}
		args := FirstChildOfType(n, "arguments")
		if args == nil || args.NamedChildCount() == 0 {
			return gotreesitter.WalkContinue
		}
		firstArg := args.NamedChild(0) // an "argument" node
		if firstArg.NamedChildCount() == 0 {
			return gotreesitter.WalkContinue
		}
		if s, ok := StringValue(firstArg.NamedChild(0), src); ok {
			result, found = s, true
			return gotreesitter.WalkStop
		}
		return gotreesitter.WalkContinue
	})
	return result, found
}
