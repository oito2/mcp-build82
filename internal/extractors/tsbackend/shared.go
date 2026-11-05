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
	"strconv"
	"strings"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/oito2/mcp-build82/internal/fsutil"
	"github.com/oito2/mcp-build82/internal/phparray"
)

// phpLang is the shared PHP grammar every parse in this package uses — Node.Type/Text calls all
// need it, so it's kept as a single package-level value rather than re-resolved per call.
var phpLang = grammars.PhpLanguage()

// maxParseFileSize caps how large a file ParseFile will read+parse. Parsing allocates a large
// multiple of the source size (hundreds of bytes per source byte), and up to 10 extractors run
// concurrently per plugin, so without a cap one unusually large/generated/minified .php file
// (whether benign or adversarial) could force a memory spike of gigabytes. 2 MiB stays above the
// largest non-vendor .php file of a Moodle core install (about 1.3 MB).
const maxParseFileSize = 2 << 20

// parseTimeout bounds one parse. A parse that hits it stops early and ParseFile reports an error
// instead of returning a partial tree. Tests lower it to force an early stop.
var parseTimeout = 10 * time.Second

// maxConcatFragments caps how many `.`-joined operands StringValue will flatten and evaluate for
// a single string_value tree before giving up. It exists purely as a work/memory bound for
// pathological inputs (see StringValue) — real Moodle source never comes close to it.
const maxConcatFragments = 10000

// ParseFile reads path and parses it as PHP, returning the resulting tree and the raw source bytes
// every Node.Text/Type call also needs. Only a regular file is read: a symbolic link or a FIFO at
// path is an error (see fsutil.ReadRegular).
func ParseFile(path string) (*gotreesitter.Tree, []byte, error) {
	src, err := fsutil.ReadRegular(path, maxParseFileSize)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	parser := gotreesitter.NewParser(phpLang)
	parser.SetTimeoutMicros(uint64(parseTimeout / time.Microsecond))
	tree, err := parser.Parse(src)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	// A parse stopped by the timeout or the parser's memory budget returns a partial tree, which
	// would silently drop declarations; it is reported as an error instead.
	if tree.ParseStoppedEarly() {
		reason := tree.ParseStopReason()
		tree.Release()
		return nil, nil, fmt.Errorf("parse %s: stopped early (%s)", path, reason)
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
// within the 2 MiB maxParseFileSize cap and can pack well over a million fragments) yields a
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

// stringLiteralValue extracts a single `string`/`encapsed_string` leaf's inner text, unescaped, or
// the class name of a `X::class` constant (as written, with or without a leading backslash), the
// form Moodle documents for hook, event, task and service registrations. This is the
// non-concatenation base case of StringValue, split out so the iterative concatenation walk above
// can call it directly on each flattened operand (so `X::class . '::m'` gives "X::m").
func stringLiteralValue(node *gotreesitter.Node, src []byte) (string, bool) {
	switch node.Type(phpLang) {
	case "string", "encapsed_string":
	case "class_constant_access_expression":
		if node.NamedChildCount() == 2 && strings.EqualFold(node.NamedChild(1).Text(src), "class") {
			return node.NamedChild(0).Text(src), true
		}
		return "", false
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

// CallbackValue returns the callback named by `node`: a `[X::class, 'method']` (or
// `['X', 'method']`) array gives "X::method"; anything else is read with StringValue.
func CallbackValue(node *gotreesitter.Node, src []byte) (string, bool) {
	if elements := ArrayElements(node); len(elements) == 2 {
		_, classNode := KeyValue(elements[0])
		_, methodNode := KeyValue(elements[1])
		class, okClass := StringValue(classNode, src)
		method, okMethod := StringValue(methodNode, src)
		if okClass && okMethod {
			return class + "::" + method, true
		}
		return "", false
	}
	return StringValue(node, src)
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

// BoolValue extracts a boolean-ish value with internal/phparray.ExtractBool's semantics: a
// `boolean` node (true/false in any letter case) or an `integer` node that is exactly 1 or 0 —
// Moodle db/*.php files use both forms for these fields. Any other node, another integer included,
// gives ok=false, so the caller keeps its default as the regex backend does.
func BoolValue(node *gotreesitter.Node, src []byte) (bool, bool) {
	if node == nil {
		return false, false
	}
	switch node.Type(phpLang) {
	case "boolean":
		return strings.EqualFold(node.Text(src), "true"), true
	case "integer":
		switch node.Text(src) {
		case "1":
			return true, true
		case "0":
			return false, true
		}
		return false, false
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
	for _, c := range namedChildren(node) {
		if c.Type(phpLang) == "array_element_initializer" {
			elements = append(elements, c)
		}
	}
	return elements
}

// namedChildren returns the named children of `n`, in order, in a single pass over its children.
// Iterating with NamedChildCount/NamedChild(i) instead rescans the children on every call, which is
// quadratic in the number of children (a file root with thousands of declarations).
func namedChildren(n *gotreesitter.Node) []*gotreesitter.Node {
	if n == nil {
		return nil
	}
	count := n.ChildCount()
	out := make([]*gotreesitter.Node, 0, count)
	for i := 0; i < count; i++ {
		if c := n.Child(i); c != nil && c.IsNamed() {
			out = append(out, c)
		}
	}
	return out
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
	count := node.ChildCount()
	for i := 0; i < count; i++ {
		if c := node.Child(i); c != nil && c.IsNamed() && c.Type(phpLang) == typeName {
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
