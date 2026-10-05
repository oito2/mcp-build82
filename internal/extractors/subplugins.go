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
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/oito2/mcp-build82/internal/phparray"
)

// Subplugin is one subplugin-type registration: a plugin declaring that it hosts subplugins of
// its own, keyed by a type prefix and rooted at a path relative to the Moodle install (e.g.
// "workshopform" -> "mod/workshop/form").
type Subplugin struct {
	Type string
	Path string
}

// subpluginsJSON mirrors db/subplugins.json's shape. Both "plugintypes" (the older key) and
// "subplugintypes" (the newer key) are read and merged, so a plugin declaring either or both
// is handled.
type subpluginsJSON struct {
	PluginTypes    map[string]string `json:"plugintypes"`
	SubpluginTypes map[string]string `json:"subplugintypes"`
}

// legacySubpluginEntryPattern matches one 'prefix' => 'path' entry inside db/subplugins.php's
// legacy $subplugins array, a flat string-to-string shape.
var legacySubpluginEntryPattern = regexp.MustCompile(`'([a-zA-Z0-9_]+)'\s*=>\s*'([^']+)'`)

// ExtractSubplugins reads pluginPath's subplugin-type declaration, preferring db/subplugins.json
// and falling back to the legacy db/subplugins.php array only when the JSON file doesn't exist;
// the JSON file is authoritative when both are present. Returns nil if the plugin declares no
// subplugins at all, the common case.
func ExtractSubplugins(pluginPath string) []Subplugin {
	if types, ok := parseSubpluginsJSON(filepath.Join(pluginPath, "db", "subplugins.json")); ok {
		return sortedSubplugins(types)
	}
	return sortedSubplugins(parseLegacySubpluginsPhp(filepath.Join(pluginPath, "db", "subplugins.php")))
}

func parseSubpluginsJSON(path string) (map[string]string, bool) {
	content, err := readFileCapped(path)
	if err != nil {
		return nil, false
	}
	var doc subpluginsJSON
	if err := json.Unmarshal(content, &doc); err != nil {
		return nil, false
	}
	types := map[string]string{}
	for k, v := range doc.PluginTypes {
		types[k] = v
	}
	for k, v := range doc.SubpluginTypes {
		types[k] = v
	}
	return types, true
}

func parseLegacySubpluginsPhp(path string) map[string]string {
	content, err := readFileCapped(path)
	if err != nil {
		return nil
	}
	body, ok := phparray.ExtractArrayBody(string(content), "subplugins")
	if !ok {
		return nil
	}
	types := map[string]string{}
	for _, m := range legacySubpluginEntryPattern.FindAllStringSubmatch(body, -1) {
		types[m[1]] = m[2]
	}
	return types
}

func sortedSubplugins(types map[string]string) []Subplugin {
	if len(types) == 0 {
		return nil
	}
	out := make([]Subplugin, 0, len(types))
	for typ, path := range types {
		out = append(out, Subplugin{Type: typ, Path: path})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}
