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
	"path/filepath"
	"testing"
)

func TestExtractPluginSettings_SingleLineDeclaration(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "settings.php"), `<?php
if ($hassiteconfig) {
    $settings->add(new admin_setting_configtext('local_test/apikey', get_string('apikey', 'local_test'), '', ''));
}
`)

	got := ExtractPluginSettings(dir)
	if got == nil || len(got.Settings) != 1 {
		t.Fatalf("expected 1 setting, got %+v", got)
	}
	if got.Settings[0] != (AdminSetting{Name: "local_test/apikey", Type: "admin_setting_configtext"}) {
		t.Errorf("unexpected setting: %+v", got.Settings[0])
	}
}

// TestExtractPluginSettings_MultiLineDeclaration confirms the common real-world formatting style
// (constructor arguments each on their own line) is still parsed correctly.
func TestExtractPluginSettings_MultiLineDeclaration(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "settings.php"), `<?php
if ($hassiteconfig) {
    $settings->add(new admin_setting_configcheckbox(
        'local_test/enablefeature',
        get_string('enablefeature', 'local_test'),
        get_string('enablefeature_desc', 'local_test'),
        1
    ));
}
`)

	got := ExtractPluginSettings(dir)
	if got == nil || len(got.Settings) != 1 {
		t.Fatalf("expected 1 setting, got %+v", got)
	}
	if got.Settings[0] != (AdminSetting{Name: "local_test/enablefeature", Type: "admin_setting_configcheckbox"}) {
		t.Errorf("unexpected setting: %+v", got.Settings[0])
	}
}

func TestExtractPluginSettings_MultipleEntries(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "settings.php"), `<?php
if ($hassiteconfig) {
    $settings->add(new admin_setting_heading('local_test/heading', get_string('heading', 'local_test'), ''));
    $settings->add(new admin_setting_configtext('local_test/apikey', get_string('apikey', 'local_test'), '', ''));
    $settings->add(new admin_setting_configcheckbox('local_test/enabled', get_string('enabled', 'local_test'), '', 1));
}
`)

	got := ExtractPluginSettings(dir)
	if got == nil || len(got.Settings) != 3 {
		t.Fatalf("expected 3 settings, got %+v", got)
	}
}

func TestExtractPluginSettings_MissingFileReturnsNil(t *testing.T) {
	if got := ExtractPluginSettings(t.TempDir()); got != nil {
		t.Errorf("expected nil for a plugin with no settings.php, got %+v", got)
	}
}
