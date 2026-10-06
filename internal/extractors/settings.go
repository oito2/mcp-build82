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
	"path/filepath"
	"regexp"
)

// AdminSetting is one admin_setting_* declaration found in a plugin's settings.php.
type AdminSetting struct {
	Name string // e.g. "local_test/apikey" — empty if the name argument isn't a string literal
	Type string // the admin_setting_* class name, e.g. "admin_setting_configtext"
}

// SettingsExtraction is the result of scanning a settings.php file: the file path and its admin
// settings in source order.
type SettingsExtraction struct {
	File     string
	Settings []AdminSetting
}

// adminSettingPattern matches "new admin_setting_xxx(" and captures its first argument when that
// is a string literal; `\s*` lets the call span lines. A non-literal first argument still matches
// the type, with an empty name.
var adminSettingPattern = regexp.MustCompile(`new\s+(admin_setting_\w+)\s*\(\s*(?:['"]([^'"]*)['"])?`)

// ParseSettingsPhp scans the settings.php file at `filePath` for every admin_setting_*
// declaration outside comments and heredocs. It returns nil when the file cannot be read and always uses a regex scan regardless
// of BUILD82_EXTRACTOR_BACKEND.
func ParseSettingsPhp(filePath string) *SettingsExtraction {
	content, err := readFileCapped(filePath)
	if err != nil {
		return nil
	}

	var settings []AdminSetting
	for _, m := range adminSettingPattern.FindAllStringSubmatch(stripCommentsAndHeredocs(string(content)), -1) {
		settings = append(settings, AdminSetting{Type: m[1], Name: m[2]})
	}
	return &SettingsExtraction{File: filePath, Settings: settings}
}

// ExtractPluginSettings parses `pluginPath`/settings.php. It returns nil when that file cannot be
// read.
func ExtractPluginSettings(pluginPath string) *SettingsExtraction {
	return ParseSettingsPhp(filepath.Join(pluginPath, "settings.php"))
}
