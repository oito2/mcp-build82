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
	"path/filepath"
	"sort"

	"github.com/odvcencio/gotreesitter"

	"github.com/oito2/mcp-build82/internal/legacyhooks"
	"github.com/oito2/mcp-build82/internal/phptypes"
)

// ExtractPluginHooks combines the three hook scans for the plugin at `pluginPath`: db/hooks.php
// callbacks, classes/hook/*.php definitions and legacy lib.php callbacks (matched using
// `component`, the plugin's frankenstyle name). The returned error is always nil.
func ExtractPluginHooks(pluginPath, component string) (phptypes.HooksExtraction, error) {
	return phptypes.HooksExtraction{
		Callbacks:      ParseHookCallbacks(pluginPath),
		Definitions:    ParseHookDefinitions(pluginPath),
		LegacyWarnings: DetectLegacyCallbacks(pluginPath, component),
	}, nil
}

// --- Callbacks (db/hooks.php's $callbacks array) ------------------------------------------------

// ParseHookCallbacks parses a plugin's db/hooks.php file's $callbacks array — same
// array-of-arrays shape as events/tasks. hookname falls back to the legacy "hook" key; requires
// either to be non-empty (after fallback) or callback to be non-empty, matching the regex backend.
func ParseHookCallbacks(pluginPath string) []phptypes.HookCallback {
	tree, src, err := ParseFile(filepath.Join(pluginPath, "db", "hooks.php"))
	if err != nil {
		return nil
	}
	defer tree.Release()
	arr := FindAssignment(tree.RootNode(), MatchVariable(src, "callbacks"))
	if arr == nil {
		return nil
	}

	var callbacks []phptypes.HookCallback
	for _, entry := range ArrayElements(arr) {
		_, inner := KeyValue(entry)
		if inner == nil {
			continue
		}
		cb := parseHookCallback(inner, src)
		if cb.HookName == "" && cb.Callback == "" {
			continue
		}
		callbacks = append(callbacks, cb)
	}
	return callbacks
}

// parseHookCallback reads one $callbacks entry's keyed fields. DefaultEnabled defaults to true,
// and the legacy "hook" key is used as the hook name when "hookname" is absent or empty.
func parseHookCallback(inner *gotreesitter.Node, src []byte) phptypes.HookCallback {
	cb := phptypes.HookCallback{DefaultEnabled: true}
	var hookname, legacyHook string
	for _, element := range ArrayElements(inner) {
		key, value := KeyValue(element)
		k, ok := StringValue(key, src)
		if !ok {
			continue
		}
		switch k {
		case "hookname":
			hookname, _ = StringValue(value, src)
		case "hook":
			legacyHook, _ = StringValue(value, src)
		case "callback":
			cb.Callback, _ = StringValue(value, src)
		case "priority":
			cb.Priority, _ = IntValue(value, src)
		case "defaultenabled":
			if b, ok := BoolValue(value, src); ok {
				cb.DefaultEnabled = b
			}
		}
	}
	if hookname == "" {
		hookname = legacyHook
	}
	cb.HookName = hookname
	return cb
}

// --- Definitions (classes/hook/*.php) ------------------------------------------------------------

// ParseHookDefinitions globs pluginPath/classes/hook/*.php and parses each as a hook definition
// class. A file without a resolvable namespace+class FQN is skipped, matching the regex backend.
func ParseHookDefinitions(pluginPath string) []phptypes.HookDefinition {
	matches, _ := filepath.Glob(filepath.Join(pluginPath, "classes", "hook", "*.php"))
	var defs []phptypes.HookDefinition
	for _, path := range matches {
		tree, src, err := ParseFile(path)
		if err != nil {
			continue
		}
		// Released at the end of each iteration (not deferred to function return) so that scanning
		// a plugin with many classes/hook/*.php files never holds more than one tree's arena alive
		// at a time.
		def, ok := parseHookDefinitionFile(tree.RootNode(), src)
		tree.Release()
		if ok {
			defs = append(defs, def)
		}
	}
	return defs
}

// parseHookDefinitionFile builds a hook definition from a parsed classes/hook file. The boolean
// is false when no namespaced class FQN can be resolved. The description comes from
// get_hook_description() or, failing that, the class docblock summary; tags come from
// get_hook_tags(); the replaced callback comes from a `replaces` key or DEPRECATED_CALLBACK.
func parseHookDefinitionFile(root *gotreesitter.Node, src []byte) (phptypes.HookDefinition, bool) {
	fqn := extractFQN(root, src)
	if fqn == "" {
		return phptypes.HookDefinition{}, false
	}
	classNode := FindDescendant(root, func(n *gotreesitter.Node) bool { return n.Type(phpLang) == "class_declaration" })

	description := methodReturnString(classNode, src, "get_hook_description")
	if description == "" {
		// Anchored to the class's own immediately-preceding docblock via precedingDocComment,
		// not a whole-file search, so a file-header docblock (license/@package) that sits
		// before the class's own docblock is never picked up.
		if doc := precedingDocComment(classNode, src); doc != nil {
			description = doc.Summary
		}
	}

	tags := methodReturnStringArray(classNode, src, "get_hook_tags")

	replaces := findKeyedStringAnywhere(root, "replaces", src)
	if replaces == "" {
		replaces = classConstString(classNode, src, "DEPRECATED_CALLBACK")
	}

	return phptypes.HookDefinition{ClassName: fqn, Description: description, Tags: tags, Replaces: replaces}, true
}

// extractFQN combines the file's namespace declaration and class name into \ns\Class — requires
// both to be present, matching the regex backend's own requirement exactly.
func extractFQN(root *gotreesitter.Node, src []byte) string {
	// Scope to namespace_definition's own namespace_name child — a bare "namespace_name" node
	// also appears inside any unrelated qualified_name (e.g. a `use Foo\Bar;` or `extends X\Y`
	// reference elsewhere in the file), so searching for that type anywhere could match the wrong one.
	nsDef := FindDescendant(root, func(n *gotreesitter.Node) bool { return n.Type(phpLang) == "namespace_definition" })
	classNode := FindDescendant(root, func(n *gotreesitter.Node) bool { return n.Type(phpLang) == "class_declaration" })
	if nsDef == nil || classNode == nil {
		return ""
	}
	nsNameNode := FirstChildOfType(nsDef, "namespace_name")
	nameNode := FirstChildOfType(classNode, "name")
	if nsNameNode == nil || nameNode == nil {
		return ""
	}
	return `\` + nsNameNode.Text(src) + `\` + nameNode.Text(src)
}

// findMethod returns the method_declaration node named methodName inside classNode, or nil when
// classNode is nil or has no such method.
func findMethod(classNode *gotreesitter.Node, src []byte, methodName string) *gotreesitter.Node {
	if classNode == nil {
		return nil
	}
	return FindDescendant(classNode, func(n *gotreesitter.Node) bool {
		if n.Type(phpLang) != "method_declaration" {
			return false
		}
		nameNode := FirstChildOfType(n, "name")
		return nameNode != nil && nameNode.Text(src) == methodName
	})
}

// methodReturnString finds methodName's body and returns the string value of its first
// return statement's expression — mirrors the regex backend's own lazy `return\s+['"](...)['"]`
// match (first return, string only).
func methodReturnString(classNode *gotreesitter.Node, src []byte, methodName string) string {
	expr := firstReturnExpression(classNode, src, methodName)
	if expr == nil {
		return ""
	}
	s, _ := StringValue(expr, src)
	return s
}

// methodReturnStringArray is methodReturnString's array-returning counterpart, for
// get_hook_tags()'s `return ['a', 'b'];`.
func methodReturnStringArray(classNode *gotreesitter.Node, src []byte, methodName string) []string {
	expr := firstReturnExpression(classNode, src, methodName)
	if expr == nil {
		return nil
	}
	var tags []string
	for _, el := range ArrayElements(expr) {
		_, value := KeyValue(el)
		if s, ok := StringValue(value, src); ok {
			tags = append(tags, s)
		}
	}
	return tags
}

// firstReturnExpression returns the expression of the first return statement found in
// methodName's body, or nil when the class, method, body or a returned expression is missing.
func firstReturnExpression(classNode *gotreesitter.Node, src []byte, methodName string) *gotreesitter.Node {
	method := findMethod(classNode, src, methodName)
	if method == nil {
		return nil
	}
	body := FirstChildOfType(method, "compound_statement")
	if body == nil {
		return nil
	}
	ret := FindDescendant(body, func(n *gotreesitter.Node) bool { return n.Type(phpLang) == "return_statement" })
	if ret == nil || ret.NamedChildCount() == 0 {
		return nil
	}
	return ret.NamedChild(0)
}

// classConstString finds a class constant declaration (`const NAME = 'value';`) by name and
// returns its string value.
func classConstString(classNode *gotreesitter.Node, src []byte, constName string) string {
	if classNode == nil {
		return ""
	}
	element := FindDescendant(classNode, func(n *gotreesitter.Node) bool {
		if n.Type(phpLang) != "const_element" {
			return false
		}
		nameNode := FirstChildOfType(n, "name")
		return nameNode != nil && nameNode.Text(src) == constName
	})
	if element == nil || element.NamedChildCount() < 2 {
		return ""
	}
	s, _ := StringValue(element.NamedChild(1), src)
	return s
}

// findKeyedStringAnywhere searches the whole file (not just one array) for the first
// array_element_initializer keyed by key, mirroring the regex backend's own whole-file
// `'replaces'\s*=>\s*'...'` search, which isn't scoped to any particular array literal either.
func findKeyedStringAnywhere(root *gotreesitter.Node, key string, src []byte) string {
	element := FindDescendant(root, func(n *gotreesitter.Node) bool {
		if n.Type(phpLang) != "array_element_initializer" {
			return false
		}
		k, _ := KeyValue(n)
		ks, ok := StringValue(k, src)
		return ok && ks == key
	})
	if element == nil {
		return ""
	}
	_, value := KeyValue(element)
	s, _ := StringValue(value, src)
	return s
}

// --- Legacy callback detection (lib.php) ----------------------------------------------------------

// DetectLegacyCallbacks scans pluginPath/lib.php for top-level functions matching
// {component}_{legacy suffix}, for every suffix in legacyhooks.Map.
func DetectLegacyCallbacks(pluginPath, component string) []phptypes.LegacyCallbackWarning {
	tree, src, err := ParseFile(filepath.Join(pluginPath, "lib.php"))
	if err != nil {
		return nil
	}
	defer tree.Release()
	root := tree.RootNode()

	var warnings []phptypes.LegacyCallbackWarning
	for suffix, replacement := range legacyhooks.Map {
		legacyFunction := component + "_" + suffix
		if !hasTopLevelFunction(root, src, legacyFunction) {
			continue
		}
		warnings = append(warnings, phptypes.LegacyCallbackWarning{
			LegacyFunction: legacyFunction,
			ReplacedBy:     replacement,
			Guidance: fmt.Sprintf(
				"Migrate %s to the Hook API: 1) add a %s entry to db/hooks.php's $callbacks array "+
					"targeting %s; 2) implement the callback in classes/hook_callbacks.php; "+
					"3) remove the legacy %s() function from lib.php.",
				legacyFunction, legacyFunction, replacement, legacyFunction,
			),
		})
	}
	// legacyhooks.Map is a map, so the order warnings are appended in is non-deterministic across
	// runs. Sort explicitly so the LegacyWarnings list is stable.
	sort.Slice(warnings, func(i, j int) bool {
		return warnings[i].LegacyFunction < warnings[j].LegacyFunction
	})
	return warnings
}

// hasTopLevelFunction reports whether the file root declares a top-level function called name.
func hasTopLevelFunction(root *gotreesitter.Node, src []byte, name string) bool {
	for i := 0; i < root.NamedChildCount(); i++ {
		c := root.NamedChild(i)
		if c.Type(phpLang) != "function_definition" {
			continue
		}
		if nameNode := FirstChildOfType(c, "name"); nameNode != nil && nameNode.Text(src) == name {
			return true
		}
	}
	return false
}
