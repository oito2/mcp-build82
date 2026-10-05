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
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/oito2/mcp-build82/internal/extractors/tsbackend"
	"github.com/oito2/mcp-build82/internal/legacyhooks"
	"github.com/oito2/mcp-build82/internal/phparray"
	"github.com/oito2/mcp-build82/internal/phptypes"
)

// HookCallback, HookDefinition, LegacyCallbackWarning, and HooksExtraction are aliases for
// phptypes' types.
type HookCallback = phptypes.HookCallback
type HookDefinition = phptypes.HookDefinition
type LegacyCallbackWarning = phptypes.LegacyCallbackWarning
type HooksExtraction = phptypes.HooksExtraction

var (
	hookFqnNamespacePattern = regexp.MustCompile(`(?m)^namespace\s+([a-zA-Z0-9_\\]+)\s*;`)
	hookFqnClassPattern     = regexp.MustCompile(`(?m)^(?:final\s+)?class\s+([a-zA-Z0-9_]+)`)

	// hookDescriptionMethodPattern/hookTagsMethodPattern match only up to and including the
	// method's opening brace; the body itself is then isolated via phparray.FindBalancedEnd
	// (see findMethodBody) before hookReturnStringPattern/hookReturnArrayPattern run, so a
	// `return` literal belonging to a different method further down the file is never matched.
	hookDescriptionMethodPattern = regexp.MustCompile(`get_hook_description\s*\([^)]*\)\s*:\s*string\s*\{`)
	hookTagsMethodPattern        = regexp.MustCompile(`get_hook_tags\s*\([^)]*\)\s*:\s*array\s*\{`)
	hookReturnStringPattern      = regexp.MustCompile(`return\s+['"]([^'"]*)['"]`)
	hookReturnArrayPattern       = regexp.MustCompile(`return\s+\[([^\]]*)\]`)

	hookReplacesArrayPattern   = regexp.MustCompile(`['"]replaces['"]\s*=>\s*['"]([^'"]+)['"]`)
	hookDeprecatedConstPattern = regexp.MustCompile(`const\s+DEPRECATED_CALLBACK\s*=\s*['"]([^'"]+)['"]\s*;`)

	// docSummaryPattern extracts the first descriptive line out of an already-isolated /** ... */
	// docblock snippet (see findClassDocSummary); it is never run against a whole file's content,
	// where it would return the license/@package file header instead of the class's own docblock.
	docSummaryPattern = regexp.MustCompile(`/\*\*\s*\n?\s*\*?\s*([^\n*][^\n]*)`)
)

// findMethodBody locates methodOpenPattern's match (which must end at the method's opening `{`)
// and returns its brace-balanced body (exclusive of the braces themselves), using
// phparray.FindBalancedEnd.
func findMethodBody(content string, methodOpenPattern *regexp.Regexp) (string, bool) {
	loc := methodOpenPattern.FindStringIndex(content)
	if loc == nil {
		return "", false
	}
	braceStart := loc[1] - 1 // index of the opening '{', the last byte the pattern matched
	end := phparray.FindBalancedEnd(content, braceStart, '{', '}')
	if end == -1 {
		return "", false
	}
	return content[braceStart+1 : end], true
}

// extractReturnedString finds methodPattern's brace-balanced body and applies returnPattern to it
// alone, so a `return` literal belonging to some other, later method can never satisfy it.
func extractReturnedString(content string, methodPattern, returnPattern *regexp.Regexp) (string, bool) {
	body, ok := findMethodBody(content, methodPattern)
	if !ok {
		return "", false
	}
	m := returnPattern.FindStringSubmatch(body)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// findClassDocSummary locates the hook definition class's own declaration line (matching
// hookFqnClassPattern) and scans backward for its immediately preceding PHPDoc block, tolerating
// up to 3 blank lines. This skips any license/@package file-header docblock that comes before the
// class's own docblock.
func findClassDocSummary(content string) string {
	lines := strings.Split(content, "\n")

	classLineIdx := -1
	for i, line := range lines {
		if hookFqnClassPattern.MatchString(line) {
			classLineIdx = i
			break
		}
	}
	if classLineIdx == -1 {
		return ""
	}

	blankCount := 0
	closeIndex := -1
	for i := classLineIdx - 1; i >= 0 && i >= classLineIdx-5; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			blankCount++
			if blankCount > 3 {
				break
			}
			continue
		}
		if trimmed == "*/" {
			closeIndex = i
			break
		}
		break
	}
	if closeIndex == -1 {
		return ""
	}

	openIndex := -1
	for i := closeIndex - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "/**") || trimmed == "/*" {
			openIndex = i
			break
		}
		if !strings.HasPrefix(trimmed, "*") && trimmed != "" {
			break
		}
	}
	if openIndex == -1 {
		return ""
	}

	block := strings.Join(lines[openIndex:closeIndex+1], "\n")
	if m := docSummaryPattern.FindStringSubmatch(block); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func extractFQN(content string) string {
	ns := hookFqnNamespacePattern.FindStringSubmatch(content)
	cls := hookFqnClassPattern.FindStringSubmatch(content)
	if ns == nil || cls == nil {
		return ""
	}
	return `\` + ns[1] + `\` + cls[1]
}

func parseHookCallbacks(pluginPath string) []HookCallback {
	content, err := readFileCapped(filepath.Join(pluginPath, "db", "hooks.php"))
	if err != nil {
		return nil
	}
	body, ok := phparray.ExtractArrayBody(string(content), "callbacks")
	if !ok {
		return nil
	}

	var callbacks []HookCallback
	for _, block := range phparray.SplitIntoBlocks(body) {
		hookname := phparray.ExtractString(block, "hookname")
		if hookname == "" {
			hookname = phparray.ExtractString(block, "hook")
		}
		callback := phparray.ExtractString(block, "callback")
		if hookname == "" && callback == "" {
			continue
		}
		callbacks = append(callbacks, HookCallback{
			HookName:       hookname,
			Callback:       callback,
			Priority:       phparray.ExtractInt(block, "priority", 0),
			DefaultEnabled: phparray.ExtractBool(block, "defaultenabled", true),
		})
	}
	return callbacks
}

func parseHookDefinitionFile(content string) (HookDefinition, bool) {
	fqn := extractFQN(content)
	if fqn == "" {
		return HookDefinition{}, false
	}

	description := ""
	if s, ok := extractReturnedString(content, hookDescriptionMethodPattern, hookReturnStringPattern); ok {
		description = s
	} else {
		description = findClassDocSummary(content)
	}

	var tags []string
	if s, ok := extractReturnedString(content, hookTagsMethodPattern, hookReturnArrayPattern); ok {
		tags = splitAndTrimCommaList(s)
	}

	replaces := ""
	if m := hookReplacesArrayPattern.FindStringSubmatch(content); m != nil {
		replaces = m[1]
	} else if m := hookDeprecatedConstPattern.FindStringSubmatch(content); m != nil {
		replaces = m[1]
	}

	return HookDefinition{ClassName: fqn, Description: description, Tags: tags, Replaces: replaces}, true
}

func parseHookDefinitions(pluginPath string) []HookDefinition {
	matches, _ := filepath.Glob(filepath.Join(pluginPath, "classes", "hook", "*.php"))
	var defs []HookDefinition
	for _, path := range matches {
		content, err := readFileCapped(path)
		if err != nil {
			continue
		}
		if def, ok := parseHookDefinitionFile(string(content)); ok {
			defs = append(defs, def)
		}
	}
	return defs
}

// legacyFunctionDeclPattern matches every top-level function declaration in lib.php in one pass —
// used instead of compiling and running one regex per legacyhooks.Map entry, which would cost up
// to 12 regexp compiles per call.
var legacyFunctionDeclPattern = regexp.MustCompile(`(?m)^function\s+(\w+)\s*\(`)

func detectLegacyCallbacks(pluginPath, component string) []LegacyCallbackWarning {
	content, err := readFileCapped(filepath.Join(pluginPath, "lib.php"))
	if err != nil {
		return nil
	}
	s := string(content)

	declared := map[string]struct{}{}
	for _, m := range legacyFunctionDeclPattern.FindAllStringSubmatch(s, -1) {
		declared[m[1]] = struct{}{}
	}

	var warnings []LegacyCallbackWarning
	for suffix, replacement := range legacyhooks.Map {
		legacyFunction := component + "_" + suffix
		if _, ok := declared[legacyFunction]; !ok {
			continue
		}
		warnings = append(warnings, LegacyCallbackWarning{
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
	// runs. Sort the result explicitly so LegacyWarnings has a stable order.
	sort.Slice(warnings, func(i, j int) bool {
		return warnings[i].LegacyFunction < warnings[j].LegacyFunction
	})
	return warnings
}

// PluginUsesHookApi reports whether a plugin actually uses the Hook API. Legacy warnings alone do
// not count — a plugin with only legacy callbacks and zero hook adoption returns false.
func PluginUsesHookApi(e HooksExtraction) bool {
	return len(e.Callbacks) > 0 || len(e.Definitions) > 0
}

// ExtractPluginHooks combines all three sub-scans: registered callbacks, hook definitions, and
// legacy-callback detection.
func ExtractPluginHooks(pluginPath, component string) (HooksExtraction, error) {
	if useTreesitter() {
		return tsbackend.ExtractPluginHooks(pluginPath, component)
	}
	return HooksExtraction{
		Callbacks:      parseHookCallbacks(pluginPath),
		Definitions:    parseHookDefinitions(pluginPath),
		LegacyWarnings: detectLegacyCallbacks(pluginPath, component),
	}, nil
}
