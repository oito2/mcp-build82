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

var (
	phpStringValue    = regexp.MustCompile(`=\s*['"]([^'"]+)['"]`)
	phpNumericValue   = regexp.MustCompile(`=\s*(\d+)`)
	phpConstOrValue   = regexp.MustCompile(`=\s*([A-Z_0-9]+);`)
	pluginNamePattern = regexp.MustCompile(`\$string\['pluginname'\]\s*=\s*['"]([^'"]+)['"]`)
)

// readVersionPhp does a line-based scan of version.php, matching individual $plugin->xxx = ...;
// lines directly. This Contains-based dispatch also matches a commented-out line like
// "// $plugin->component = 'old';".
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

func readDisplayName(pluginPath, component string) string {
	content, err := readFileCapped(filepath.Join(pluginPath, "lang", "en", component+".php"))
	if err != nil {
		return ""
	}
	return firstSubmatch(pluginNamePattern, string(content))
}

// inferTypeFromPath infers a plugin's type from its parent directory name, reverse-mapped
// through moodletype.PluginTypeToDir.
//
// Iterates moodletype.PluginTypeToDir in sorted key order rather than ranging over the map
// directly — several entries share the same trailing directory segment (e.g. "report",
// "gradereport" -> "grade/report", "scormreport" -> "mod/scorm/report" all end in ".../report"),
// so a plugin whose literal parent directory is named "report" could match more than one entry.
// Exact-match candidates are checked before suffix-match candidates, in each case in sorted-key
// order, so the result is deterministic.
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

// IsPlugin reports whether dirPath directly contains a version.php.
func IsPlugin(dirPath string) bool {
	return fileExists(filepath.Join(dirPath, "version.php"))
}

// DetectPlugin detects a plugin's identity from its directory. Returns an error if the directory
// doesn't exist.
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
