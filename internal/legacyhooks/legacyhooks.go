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

// Package legacyhooks holds the legacy-callback-suffix → Hook API replacement map shared by both
// extraction backends (internal/extractors' regex backend and internal/extractors/tsbackend's
// tree-sitter backend). It lives in its own package so neither backend needs to import the other
// for this data.
package legacyhooks

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/oito2/mcp-build82/internal/phptypes"
)

// data is the embedded legacy_hooks.json content.
//
//go:embed legacy_hooks.json
var data []byte

// Map is the legacy lib.php callback suffix (e.g. "before_footer") to its Hook API replacement FQN
// (e.g. \core\hook\output\before_footer_html_generation). It is decoded from the embedded JSON at package
// initialization, which panics if that JSON is malformed.
var Map = func() map[string]string {
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		panic(fmt.Sprintf("legacyhooks: malformed embedded legacy_hooks.json: %v", err))
	}
	return m
}()

// Guidance returns the migration advice for the legacy callback `legacyFunction`, whose Hook API
// replacement is the hook class `replacement`.
func Guidance(legacyFunction, replacement string) string {
	return fmt.Sprintf(
		"Migrate %s to the Hook API: 1) add a %s entry to db/hooks.php's $callbacks array "+
			"targeting %s; 2) implement the callback in classes/hook_callbacks.php; "+
			"3) remove the legacy %s() function from lib.php.",
		legacyFunction, legacyFunction, replacement, legacyFunction,
	)
}

// Warnings returns a warning, sorted by function name, for every legacy callback <name>_<suffix>
// (for each of `names`, the names a component is known by, and each suffix of Map) for which
// `declared` reports a function declaration. `declared` is asked with the lower-case function
// name, since PHP function names are case-insensitive; callers store declared names lower-cased. Each warning names the Hook API replacement and
// carries its Guidance. Both extraction backends build their warnings with it, and differ only in
// how they find the declared functions.
func Warnings(names []string, declared func(function string) bool) []phptypes.LegacyCallbackWarning {
	var warnings []phptypes.LegacyCallbackWarning
	for suffix, replacement := range Map {
		for _, name := range names {
			legacyFunction := name + "_" + suffix
			if !declared(strings.ToLower(legacyFunction)) {
				continue
			}
			warnings = append(warnings, phptypes.LegacyCallbackWarning{
				LegacyFunction: legacyFunction,
				ReplacedBy:     replacement,
				Guidance:       Guidance(legacyFunction, replacement),
			})
		}
	}
	// Map iteration order is random, so the warnings are sorted for a stable order.
	sort.Slice(warnings, func(i, j int) bool {
		return warnings[i].LegacyFunction < warnings[j].LegacyFunction
	})
	return warnings
}
