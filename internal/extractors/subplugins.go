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

package extractors

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/oito2/mcp-build82/internal/phparray"
)

// Subplugin is one subplugin-type registration: a plugin declaring that it hosts subplugins of its
// own, as a type prefix and a directory relative to the Moodle root (e.g. "workshopform" to
// "mod/workshop/form").
type Subplugin struct {
	Type string
	Path string
}

// subpluginsJSON mirrors the shape of db/subplugins.json. The "plugintypes" and "subplugintypes"
// keys are both read and merged.
type subpluginsJSON struct {
	PluginTypes    map[string]string `json:"plugintypes"`
	SubpluginTypes map[string]string `json:"subplugintypes"`
}

// legacySubpluginEntryPattern matches one 'prefix' => 'path' entry of the $subplugins array in
// db/subplugins.php.
var legacySubpluginEntryPattern = regexp.MustCompile(`'([a-zA-Z0-9_]+)'\s*=>\s*'([^']+)'`)

// ExtractSubplugins returns the subplugin types declared by the plugin at `pluginPath`, sorted by
// type. db/subplugins.json is authoritative when the file exists: if it is unreadable or invalid
// no subplugins are returned and db/subplugins.php is not consulted. db/subplugins.php is used only
// when db/subplugins.json does not exist. It returns nil when the plugin declares no subplugins.
func ExtractSubplugins(pluginPath string) []Subplugin {
	jsonPath := filepath.Join(pluginPath, "db", "subplugins.json")
	if _, err := os.Stat(jsonPath); err == nil {
		types, _ := parseSubpluginsJSON(jsonPath)
		return sortedSubplugins(types)
	}
	return sortedSubplugins(parseLegacySubpluginsPhp(filepath.Join(pluginPath, "db", "subplugins.php")))
}

// parseSubpluginsJSON reads the type-to-path map from the JSON file at `path`. The boolean is false
// when the file cannot be read or decoded.
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

// parseLegacySubpluginsPhp reads the type-to-path map from the $subplugins array of the PHP file at
// `path`. It returns nil when the file cannot be read or has no such array.
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

// sortedSubplugins converts the type-to-path map `types` into a slice sorted by type, or nil when
// the map is empty.
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
