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
	DisplayName string // $string['pluginname'] from the English lang file (moodletype.LangFileName)
	Maturity    string // MATURITY_STABLE etc — kept as the raw PHP constant string
	MoodlePath  string // set by generators, empty when DetectPlugin called directly
}

// Patterns that read the value of a PHP assignment: a quoted string, digits, or an upper-case
// constant, plus the plugin name string from a lang file.
var (
	phpStringValue    = regexp.MustCompile(`=\s*['"]([^'"]+)['"]`)
	phpNumericValue   = regexp.MustCompile(`=\s*(\d+(?:\.\d+)?)`)
	phpConstOrValue   = regexp.MustCompile(`=\s*([A-Z_0-9]+);`)
	pluginNamePattern = regexp.MustCompile(`\$string\['pluginname'\]\s*=\s*['"]([^'"]+)['"]`)
)

// readVersionPhp scans `pluginPath`/version.php line by line and returns the values assigned to
// $plugin->component, version, requires and maturity, with "" for any that are missing or when the
// file cannot be read. Comments and heredoc bodies are blanked out first, so a commented-out
// assignment such as "// $plugin->component = 'old';" is ignored; when a field is assigned more
// than once, the last assignment wins, as when PHP runs the file. Numeric values keep their
// decimal part (2024010100.01).
func readVersionPhp(pluginPath string) (component, version, requires, maturity string) {
	if useTreesitter() {
		return tsbackend.ReadVersionPhp(pluginPath)
	}
	content, err := readFileCapped(filepath.Join(pluginPath, "version.php"))
	if err != nil {
		return
	}
	// Quoted strings are not blanked, so the values are read from the blanked text directly.
	for _, line := range strings.Split(stripCommentsAndHeredocs(string(content)), "\n") {
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

// readDisplayName returns the $string['pluginname'] value from the plugin's English language file
// (`pluginPath`/lang/en/<moodletype.LangFileName(component)>.php),
// or "" when the file or the string is missing.
func readDisplayName(pluginPath, component string) string {
	content, err := readFileCapped(filepath.Join(pluginPath, "lang", "en", moodletype.LangFileName(component)+".php"))
	if err != nil {
		return ""
	}
	return firstSubmatch(pluginNamePattern, string(content))
}

// inferTypeFromPath infers a plugin's type from the directory that contains it, by reverse mapping
// moodletype.PluginTypeToDir: the type whose directory is the longest trailing part of that path
// wins, so ".../question/type/x" is a qtype (not a calendartype, whose directory "calendar/type"
// also ends in "type"), ".../grade/report/x" a gradereport (not a report) and
// ".../mod/scorm/report/x" a scormreport. Ties go to the first type in sorted order, to keep the
// result deterministic. When nothing matches, the parent directory name itself is returned.
func inferTypeFromPath(pluginPath string) string {
	parent := filepath.ToSlash(filepath.Dir(pluginPath))

	types := make([]string, 0, len(moodletype.PluginTypeToDir))
	for typ := range moodletype.PluginTypeToDir {
		types = append(types, typ)
	}
	sort.Strings(types)

	best, bestLen := "", 0
	for _, typ := range types {
		dir := moodletype.PluginTypeToDir[typ]
		if (parent == dir || strings.HasSuffix(parent, "/"+dir)) && len(dir) > bestLen {
			best, bestLen = typ, len(dir)
		}
	}
	if best != "" {
		return best
	}
	return filepath.Base(filepath.Dir(pluginPath)) // fallback: use the raw directory name as the type
}

// IsPlugin reports whether `dirPath` directly contains a version.php.
func IsPlugin(dirPath string) bool {
	return fileExists(filepath.Join(dirPath, "version.php"))
}

// componentPattern matches a frankenstyle component: a lowercase plugin type, an underscore, and a
// name of lowercase letters, digits and underscores (e.g. "local_myplugin", "mod_forum").
var componentPattern = regexp.MustCompile(`^[a-z][a-z0-9]*_[a-z0-9_]+$`)

// ValidComponent reports whether `component` is a frankenstyle component name, and so is safe to
// use as a file or directory name (it cannot contain path separators or "..").
func ValidComponent(component string) bool {
	return componentPattern.MatchString(component)
}

// DetectPlugin detects the identity of the plugin in directory `pluginPath`. The type is inferred
// from the parent directory, the component falls back to type_name when version.php does not set
// it or sets something that is not a frankenstyle component (ValidComponent) — the value ends up in
// file paths and file names, so a crafted one such as "../../x" must never be used — and the
// display name falls back to the directory name. It returns an error when
// `pluginPath` does not exist or is not a directory.
func DetectPlugin(pluginPath string) (PluginInfo, error) {
	info, err := os.Stat(pluginPath)
	if err != nil || !info.IsDir() {
		return PluginInfo{}, fmt.Errorf("plugin directory not found: %s", pluginPath)
	}

	component, version, requires, maturity := readVersionPhp(pluginPath)
	name := filepath.Base(pluginPath)
	typ := inferTypeFromPath(pluginPath)

	if !ValidComponent(component) {
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
