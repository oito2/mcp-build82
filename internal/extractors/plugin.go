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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/oito2/mcp-build82/internal/extractors/tsbackend"
	"github.com/oito2/mcp-build82/internal/moodletype"
)

// PluginInfo describes a plugin's identity, detected from its directory.
type PluginInfo struct {
	Name        string // directory name, e.g. "myplugin"
	Type        string // e.g. "local", "mod"
	Component   string // "local_myplugin"
	Path        string // absolute plugin dir
	Version     string // from version.php, e.g. "2024010100"
	Requires    string // min Moodle version required
	DisplayName string // from lang/en/{component}.php $string['pluginname']
	Maturity    string // MATURITY_STABLE etc — kept as the raw PHP constant string
	MoodlePath  string // set by generators, empty when DetectPlugin called directly
}

// Patterns that read the value of a PHP assignment: a quoted string, digits, or an upper-case
// constant, plus the plugin name string from a lang file.
var (
	phpStringValue    = regexp.MustCompile(`=\s*['"]([^'"]+)['"]`)
	phpNumericValue   = regexp.MustCompile(`=\s*(\d+)`)
	phpConstOrValue   = regexp.MustCompile(`=\s*([A-Z_0-9]+);`)
	pluginNamePattern = regexp.MustCompile(`\$string\['pluginname'\]\s*=\s*['"]([^'"]+)['"]`)
)

// readVersionPhp scans `pluginPath`/version.php line by line and returns the values assigned to
// $plugin->component, version, requires and maturity, with "" for any that are missing or when the
// file cannot be read. Lines are matched by substring, so a commented-out assignment such as
// "// $plugin->component = 'old';" is also picked up.
func readVersionPhp(pluginPath string) (component, version, requires, maturity string) {
	if useTreesitter() {
		return tsbackend.ReadVersionPhp(pluginPath)
	}
	content, err := readFileCapped(filepath.Join(pluginPath, "version.php"))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(content), "\n") {
		switch {
		case strings.Contains(line, "$plugin->component"):
			component = firstSubmatch(phpStringValue, line)
		case strings.Contains(line, "$plugin->version"):
			version = firstSubmatch(phpNumericValue, line)
		case strings.Contains(line, "$plugin->requires"):
			requires = firstSubmatch(phpNumericValue, line)
		case strings.Contains(line, "$plugin->maturity"):
			maturity = firstSubmatch(phpConstOrValue, line)
		}
	}
	return
}

// readDisplayName returns the $string['pluginname'] value from `pluginPath`/lang/en/`component`.php,
// or "" when the file or the string is missing.
func readDisplayName(pluginPath, component string) string {
	content, err := readFileCapped(filepath.Join(pluginPath, "lang", "en", component+".php"))
	if err != nil {
		return ""
	}
	return firstSubmatch(pluginNamePattern, string(content))
}

// inferTypeFromPath infers a plugin's type from the name of its parent directory by reverse
// mapping moodletype.PluginTypeToDir. Several types share a trailing directory segment (e.g.
// "report", "gradereport" -> "grade/report", "scormreport" -> "mod/scorm/report"), so exact
// directory matches are tried before suffix matches, each in sorted type order, to keep the result
// deterministic. When nothing matches, the parent directory name itself is returned.
func inferTypeFromPath(pluginPath string) string {
	parentDir := filepath.Base(filepath.Dir(pluginPath))

	types := make([]string, 0, len(moodletype.PluginTypeToDir))
	for typ := range moodletype.PluginTypeToDir {
		types = append(types, typ)
	}
	sort.Strings(types)

	for _, typ := range types {
		if moodletype.PluginTypeToDir[typ] == parentDir {
			return typ
		}
	}
	for _, typ := range types {
		if strings.HasSuffix(moodletype.PluginTypeToDir[typ], "/"+parentDir) {
			return typ
		}
	}
	return parentDir // fallback: use the raw directory name as the type
}

// IsPlugin reports whether `dirPath` directly contains a version.php.
func IsPlugin(dirPath string) bool {
	return fileExists(filepath.Join(dirPath, "version.php"))
}

// DetectPlugin detects the identity of the plugin in directory `pluginPath`. The type is inferred
// from the parent directory, the component falls back to type_name when version.php does not set
// it, and the display name falls back to the directory name. It returns an error when
// `pluginPath` does not exist or is not a directory.
func DetectPlugin(pluginPath string) (PluginInfo, error) {
	info, err := os.Stat(pluginPath)
	if err != nil || !info.IsDir() {
		return PluginInfo{}, fmt.Errorf("plugin directory not found: %s", pluginPath)
	}

	component, version, requires, maturity := readVersionPhp(pluginPath)
	name := filepath.Base(pluginPath)
	typ := inferTypeFromPath(pluginPath)

	if component == "" {
		component = typ + "_" + name
	}

	displayName := readDisplayName(pluginPath, component)
	if displayName == "" {
		displayName = name
	}

	return PluginInfo{
		Name:        name,
		Type:        typ,
		Component:   component,
		Path:        pluginPath,
		Version:     version,
		Requires:    requires,
		DisplayName: displayName,
		Maturity:    maturity,
	}, nil
}
