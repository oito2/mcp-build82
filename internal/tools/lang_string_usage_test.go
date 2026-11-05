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

package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/oito2/mcp-build82/internal/extractors"
)

// TestCheckLangStringUsage_FlagsUndeclaredOwnString confirms a get_string() call naming a string
// the plugin never declares in its own lang file is caught.
func TestCheckLangStringUsage_FlagsUndeclaredOwnString(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "local_test")
	mustMkdirAll(t, filepath.Join(pluginDir, "lang", "en"))
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"), `<?php
$plugin->component = 'local_test';
$plugin->version = 2024010100;
`)
	mustWriteFile(t, filepath.Join(pluginDir, "lang", "en", "local_test.php"), `<?php
$string['pluginname'] = 'Test';
`)
	mustWriteFile(t, filepath.Join(pluginDir, "lib.php"), `<?php
function local_test_render() {
    return get_string('missingkey', 'local_test');
}
`)

	results := checkLangStringUsage(loadDevPlugins("", []string{pluginDir}))
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 result, got %d: %+v", len(results), results)
	}
	if results[0].Status != statusWarn {
		t.Errorf("expected statusWarn, got %v", results[0].Status)
	}
	if !strings.Contains(results[0].Detail, "missingkey") {
		t.Errorf("detail missing the undeclared string identifier: %q", results[0].Detail)
	}
}

// TestCheckLangStringUsage_DeclaredStringReportsOK confirms a correctly declared-and-used own
// string produces no warning.
func TestCheckLangStringUsage_DeclaredStringReportsOK(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "local_test")
	mustMkdirAll(t, filepath.Join(pluginDir, "lang", "en"))
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"), `<?php
$plugin->component = 'local_test';
$plugin->version = 2024010100;
`)
	mustWriteFile(t, filepath.Join(pluginDir, "lang", "en", "local_test.php"), `<?php
$string['pluginname'] = 'Test';
$string['greeting'] = 'Hello';
`)
	mustWriteFile(t, filepath.Join(pluginDir, "lib.php"), `<?php
function local_test_render() {
    return get_string('greeting', 'local_test');
}
`)

	results := checkLangStringUsage(loadDevPlugins("", []string{pluginDir}))
	if len(results) != 1 || results[0].Status != statusOK {
		t.Fatalf("expected a single OK result, got %+v", results)
	}
}

// TestCheckLangStringUsage_IgnoresCoreAndOtherComponentStrings confirms get_string() calls
// referencing core or a different plugin's component (extremely common and expected) are never
// flagged — only calls naming this plugin's own component are cross-referenced.
func TestCheckLangStringUsage_IgnoresCoreAndOtherComponentStrings(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "local_test")
	mustMkdirAll(t, pluginDir)
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"), `<?php
$plugin->component = 'local_test';
$plugin->version = 2024010100;
`)
	mustWriteFile(t, filepath.Join(pluginDir, "lib.php"), `<?php
function local_test_render() {
    return get_string('save', 'core') . get_string('view', 'moodle');
}
`)

	results := checkLangStringUsage(loadDevPlugins("", []string{pluginDir}))
	if len(results) != 1 || results[0].Status != statusOK {
		t.Fatalf("expected a single OK result (no own-component calls made), got %+v", results)
	}
}

// TestCheckLangStringUsage_NoDevPlugins verifies nil is returned when there are no dev plugins.
func TestCheckLangStringUsage_NoDevPlugins(t *testing.T) {
	if results := checkLangStringUsage(nil); results != nil {
		t.Errorf("expected nil with no dev plugins, got %+v", results)
	}
}

// TestDoctorSourceScans_UseTheListedPhpFiles verifies that the capability and language-string
// checks scan the *.php files listed once by loadDevPlugins instead of walking the plugin again:
// a file created after the listing is not scanned.
func TestDoctorSourceScans_UseTheListedPhpFiles(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "local_test")
	mustMkdirAll(t, filepath.Join(pluginDir, "lang", "en"))
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"), "<?php\n$plugin->component = 'local_test';\n$plugin->version = 2024010100;\n")
	mustWriteFile(t, filepath.Join(pluginDir, "lang", "en", "local_test.php"), "<?php\n$string['pluginname'] = 'Test';\n")
	plugins := loadDevPlugins("", []string{pluginDir})

	mustWriteFile(t, filepath.Join(pluginDir, "late.php"), "<?php\n"+
		"echo get_string('latekey', 'local_test');\n"+
		"require_capability('local/test:latecap', $context);\n")
	for name, results := range map[string][]checkResult{
		"lang strings": checkLangStringUsage(plugins),
		"capabilities": checkCapabilityUsage(plugins),
	} {
		for _, r := range results {
			if strings.Contains(r.Detail, "late") {
				t.Errorf("%s: a file missing from the listing was scanned: %+v", name, r)
			}
		}
	}
	if got := extractors.FindOwnGetStringCalls(pluginDir, "local_test"); len(got) != 1 {
		t.Fatalf("test setup: expected the late file to hold one call, got %+v", got)
	}
}
