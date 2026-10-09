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
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/oito2/mcp-build82/internal/fsutil"
	"github.com/oito2/mcp-build82/internal/phparray"
)

// phpLang is the shared PHP grammar every parse in this package uses — Node.Type/Text calls all
// need it, so it's kept as a single package-level value rather than re-resolved per call.
var phpLang = grammars.PhpLanguage()

// maxParseFileSize caps how large a file ParseFile will read+parse. Parsing allocates a large
// multiple of the source size, so without a cap a plugin directory containing one unusually
// large/generated/minified .php file (whether benign or adversarial) could force a
// disproportionate memory spike per call. 8 MiB is generous headroom over any real Moodle
// core/plugin .php file.
const maxParseFileSize = 8 << 20

// maxConcatFragments caps how many `.`-joined operands StringValue will flatten and evaluate for
// a single string_value tree before giving up. It exists purely as a work/memory bound for
// pathological inputs (see StringValue) — real Moodle source never comes close to it.
const maxConcatFragments = 10000

// ParseFile reads path and parses it as PHP, returning the resulting tree and the raw source bytes
// every Node.Text/Type call also needs. Only a regular file is read: a symbolic link or a FIFO at
// path is an error (see fsutil.ReadRegular).
func ParseFile(path string) (*gotreesitter.Tree, []byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if info.Size() > maxParseFileSize {
		return nil, nil, fmt.Errorf("parse %s: file too large (%d bytes, max %d)", path, info.Size(), maxParseFileSize)
	}
	src, err := fsutil.ReadRegular(path, maxParseFileSize)
	if err != nil {
		return nil, nil, err
	}
	parser := gotreesitter.NewParser(phpLang)
	tree, err := parser.Parse(src)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return tree, src, nil
}

// FindAssignment walks every assignment_expression under root (in document order) and returns the
// RHS value node of the first one whose LHS matches lhsMatch. Returns nil if none match.
func FindAssignment(root *gotreesitter.Node, lhsMatch func(lhs *gotreesitter.Node) bool) *gotreesitter.Node {
	var found *gotreesitter.Node
	gotreesitter.Walk(root, func(n *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if found != nil {
			return gotreesitter.WalkStop
		}
		if n.Type(phpLang) != "assignment_expression" || n.NamedChildCount() != 2 {
			return gotreesitter.WalkContinue
		}
		if lhsMatch(n.NamedChild(0)) {
			found = n.NamedChild(1)
			return gotreesitter.WalkStop
		}
		return gotreesitter.WalkContinue
	})
	return found
}

// StringValue extracts a string literal's inner text, unescaped. The PHP grammar uses two node
// types depending on quote style — `string` for single-quoted, `encapsed_string` for
// double-quoted — both with the same named-child shape. Their named children alternate between
// string_content (literal text) and escape_sequence (e.g. \' or \\) around any embedded escape —
// concatenating their raw text in order reconstructs the same inner text the regex backend's
// capture group would see (which also matches either quote style identically via `['"]`), so the
// same phparray.UnescapeString rule applies unchanged. Interpolation is not evaluated.
//
// Also handles `.`-concatenated string literals (e.g. a description split across lines with `.`),
// returning the joined text. The regex backend only captures the first fragment of a concatenation.
//
// Concatenation chains are flattened and evaluated ITERATIVELY with an explicit stack rather than
// by having StringValue call itself on both operands of every `.`: a PHP file consisting of a
// single string built from thousands of tiny `.`-joined fragments (e.g. 'a'.'a'.'a'..., which fits
// within the 8 MiB maxParseFileSize cap and can pack well over a million fragments) yields a
// binary_expression tree whose depth is proportional to the fragment count, and recursing on it
// would exhaust the goroutine stack (a fatal "stack overflow", not a recoverable panic). Using an
// explicit heap-allocated stack keeps Go call-stack depth O(1) regardless of chain length.
// maxConcatFragments additionally bounds the total work (and the explicit stack's own memory) so
// even a well-formed-but-enormous chain can't force unbounded processing time.
func StringValue(node *gotreesitter.Node, src []byte) (string, bool) {
	if node == nil {
		return "", false
	}
	if !isConcatExpr(node, src) {
		return stringLiteralValue(node, src)
	}

	// pending holds nodes not yet resolved into text, in the order they must be popped to produce
	// left-to-right output: each concat node is replaced by pushing its right child then its left
	// child, so the left child (pushed last) is popped — and its own children expanded — before the
	// right child is ever reached. This reproduces the same left-to-right evaluation order as a plain
	// recursive descent, for chains nested on either side (or both).
	pending := []*gotreesitter.Node{node}
	var out strings.Builder
	fragments := 0
	for len(pending) > 0 {
		n := pending[len(pending)-1]
		pending = pending[:len(pending)-1]

		if isConcatExpr(n, src) {
			pending = append(pending, n.NamedChild(1), n.NamedChild(0))
			continue
		}

		fragments++
		if fragments > maxConcatFragments {
			return "", false
		}
		val, ok := stringLiteralValue(n, src)
		if !ok {
			return "", false
		}
		out.WriteString(val)
	}
	return out.String(), true
}

// isConcatExpr reports whether node is a `.`-operator binary_expression with exactly two operands
// — the shape StringValue flattens as a string concatenation, as opposed to any other
// binary_expression (arithmetic, logical, comparison) or leaf node.
func isConcatExpr(node *gotreesitter.Node, src []byte) bool {
	return node.Type(phpLang) == "binary_expression" && node.NamedChildCount() == 2 && concatOperator(node, src)
}

// stringLiteralValue extracts a single `string`/`encapsed_string` leaf's inner text, unescaped.
// This is the non-concatenation base case of StringValue, split out so the iterative concatenation
// walk above can call it directly on each flattened operand.
func stringLiteralValue(node *gotreesitter.Node, src []byte) (string, bool) {
	switch node.Type(phpLang) {
	case "string", "encapsed_string":
	default:
		return "", false
	}
	// Walk ALL children — named and anonymous — not just NamedChild: PHP's "curly"/complex
	// interpolation syntax (`{$expr}` inside a double-quoted string) parses with the `{` and `}`
	// delimiters as anonymous children, which NamedChild alone would drop, producing
	// "...$obj->prop..." instead of "...{$obj->prop}...". `{...}` is preserved literally (no
	// interpolation is evaluated). The only children to skip are the node's own opening and closing
	// quote delimiters (`'` or `"`), which are always exactly the first and last child.
	raw := ""
	n := node.ChildCount()
	for i := 1; i < n-1; i++ {
		raw += node.Child(i).Text(src)
	}
	return phparray.UnescapeString(raw), true
}

// concatOperator reports whether a binary_expression's operator token is PHP's string
// concatenation operator `.` (as opposed to arithmetic, logical, or comparison operators, which
// share the same binary_expression node type).
func concatOperator(node *gotreesitter.Node, src []byte) bool {
	for i := 0; i < node.ChildCount(); i++ {
		c := node.Child(i)
		if !c.IsNamed() && c.Text(src) == "." {
			return true
		}
	}
	return false
}

// IntValue extracts an `integer` node's value.
func IntValue(node *gotreesitter.Node, src []byte) (int, bool) {
	if node == nil || node.Type(phpLang) != "integer" {
		return 0, false
	}
	n, err := strconv.Atoi(node.Text(src))
	if err != nil {
		return 0, false
	}
	return n, true
}

// BoolValue extracts a boolean-ish value, matching internal/phparray.ExtractBool's own
// case-insensitive true/false/1/0 semantics exactly: a `boolean` node ("true"/"false") or an
// `integer` node ("1"/"0") — Moodle db/*.php files use both forms interchangeably for these
// fields.
func BoolValue(node *gotreesitter.Node, src []byte) (bool, bool) {
	if node == nil {
		return false, false
	}
	switch node.Type(phpLang) {
	case "boolean":
		return node.Text(src) == "true", true
	case "integer":
		return node.Text(src) == "1", true
	default:
		return false, false
	}
}

// ArrayElements returns every array_element_initializer child of an array_creation_expression
// node. Returns nil if node isn't an array_creation_expression.
func ArrayElements(node *gotreesitter.Node) []*gotreesitter.Node {
	if node == nil || node.Type(phpLang) != "array_creation_expression" {
		return nil
	}
	var elements []*gotreesitter.Node
	for i := 0; i < node.NamedChildCount(); i++ {
		if c := node.NamedChild(i); c.Type(phpLang) == "array_element_initializer" {
			elements = append(elements, c)
		}
	}
	return elements
}

// MatchVariable returns an LHS matcher for a plain $name variable (as opposed to
// MatchPluginField's $plugin->field member access) — variable_name's Text() renders exactly as
// "$name", so a direct string comparison is enough.
func MatchVariable(src []byte, name string) func(*gotreesitter.Node) bool {
	want := "$" + name
	return func(lhs *gotreesitter.Node) bool {
		return lhs.Type(phpLang) == "variable_name" && lhs.Text(src) == want
	}
}

// MatchPluginField returns an LHS matcher for $plugin->{field} — member_access_expression's Text()
// renders exactly as "$plugin->field", so a direct string comparison is enough; no need to walk
// into the expression's own object/property children.
func MatchPluginField(src []byte, field string) func(*gotreesitter.Node) bool {
	want := "$plugin->" + field
	return func(lhs *gotreesitter.Node) bool {
		return lhs.Type(phpLang) == "member_access_expression" && lhs.Text(src) == want
	}
}

// FirstChildOfType returns node's first direct named child of the given type, or nil if none
// matches. Unlike FindDescendant, this never looks past direct children — useful when a
// construct's shape varies (e.g. method_declaration's optional visibility/static modifiers and
// return type mean the name/body children aren't at a fixed index).
func FirstChildOfType(node *gotreesitter.Node, typeName string) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	for i := 0; i < node.NamedChildCount(); i++ {
		if c := node.NamedChild(i); c.Type(phpLang) == typeName {
			return c
		}
	}
	return nil
}

// FindDescendant walks root's whole subtree (depth-first, document order) and returns the first
// node for which match returns true, or nil if none matches.
func FindDescendant(root *gotreesitter.Node, match func(*gotreesitter.Node) bool) *gotreesitter.Node {
	var found *gotreesitter.Node
	gotreesitter.Walk(root, func(n *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if found != nil {
			return gotreesitter.WalkStop
		}
		if match(n) {
			found = n
			return gotreesitter.WalkStop
		}
		return gotreesitter.WalkContinue
	})
	return found
}

// orDefault mirrors internal/extractors.orDefault — a field that parses to an empty string still
// falls back to def (e.g. a cron field defaulting to "*"). It is shared by the tasks, services and
// capabilities extractors.
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// KeyValue returns an array_element_initializer's key and value nodes (key is nil for a
// positional/non-keyed element, e.g. ['a', 'b'] rather than ['k' => 'v']).
func KeyValue(element *gotreesitter.Node) (key, value *gotreesitter.Node) {
	switch element.NamedChildCount() {
	case 2:
		return element.NamedChild(0), element.NamedChild(1)
	case 1:
		return nil, element.NamedChild(0)
	default:
		return nil, nil
	}
}
