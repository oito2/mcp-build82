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

// TestCheckCapabilityUsage_FlagsUndeclaredOwnCapability confirms a typo'd own-capability check is
// caught: the plugin checks 'local/test:vew' (typo) but only declares 'local/test:view' — the
// "type/name:" form Moodle requires for every plugin type, not only mod and block.
func TestCheckCapabilityUsage_FlagsUndeclaredOwnCapability(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "local_test")
	mustMkdirAll(t, filepath.Join(pluginDir, "db"))
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"), `<?php
$plugin->component = 'local_test';
$plugin->version = 2024010100;
`)
	mustWriteFile(t, filepath.Join(pluginDir, "db", "access.php"), `<?php
$capabilities = [
    'local/test:view' => [
        'captype' => 'read',
        'contextlevel' => CONTEXT_SYSTEM,
    ],
];
`)
	mustWriteFile(t, filepath.Join(pluginDir, "lib.php"), `<?php
function local_test_check($context) {
    return has_capability('local/test:vew', $context);
}
`)

	results := checkCapabilityUsage(loadDevPlugins("", []string{pluginDir}))
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 result, got %d: %+v", len(results), results)
	}
	if results[0].Status != statusWarn {
		t.Errorf("expected statusWarn, got %v", results[0].Status)
	}
	if !strings.Contains(results[0].Detail, "local/test:vew") {
		t.Errorf("detail missing the typo'd capability name: %q", results[0].Detail)
	}
}

// TestCheckCapabilityUsage_DeclaredCapabilityReportsOK confirms a correctly declared-and-checked
// own capability produces no warning.
func TestCheckCapabilityUsage_DeclaredCapabilityReportsOK(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "local_test")
	mustMkdirAll(t, filepath.Join(pluginDir, "db"))
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"), `<?php
$plugin->component = 'local_test';
$plugin->version = 2024010100;
`)
	mustWriteFile(t, filepath.Join(pluginDir, "db", "access.php"), `<?php
$capabilities = [
    'local/test:view' => [
        'captype' => 'read',
        'contextlevel' => CONTEXT_SYSTEM,
    ],
];
`)
	mustWriteFile(t, filepath.Join(pluginDir, "lib.php"), `<?php
function local_test_check($context) {
    return has_capability('local/test:view', $context);
}
`)

	results := checkCapabilityUsage(loadDevPlugins("", []string{pluginDir}))
	if len(results) != 1 || results[0].Status != statusOK {
		t.Fatalf("expected a single OK result, got %+v", results)
	}
}

// TestCheckCapabilityUsage_IgnoresCoreAndOtherPluginCapabilities confirms checking a core or
// different-plugin capability (extremely common and expected) is never flagged — only checks
// naming this plugin's own capability namespace are cross-referenced.
func TestCheckCapabilityUsage_IgnoresCoreAndOtherPluginCapabilities(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "local_test")
	mustMkdirAll(t, pluginDir)
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"), `<?php
$plugin->component = 'local_test';
$plugin->version = 2024010100;
`)
	mustWriteFile(t, filepath.Join(pluginDir, "lib.php"), `<?php
function local_test_check($context) {
    return has_capability('moodle/course:view', $context) && has_capability('mod/forum:startdiscussion', $context);
}
`)

	results := checkCapabilityUsage(loadDevPlugins("", []string{pluginDir}))
	if len(results) != 1 || results[0].Status != statusOK {
		t.Fatalf("expected a single OK result (no own-namespace checks made), got %+v", results)
	}
}

// TestCheckCapabilityUsage_ModPluginUsesSlashPrefix confirms a mod plugin's own capability checks
// are compared against the "mod/{name}:" prefix, not "mod_{name}:".
func TestCheckCapabilityUsage_ModPluginUsesSlashPrefix(t *testing.T) {
	// DetectPlugin infers the type from the parent directory name, so a real "mod/" parent is
	// needed for the type to resolve to "mod".
	pluginDir := filepath.Join(t.TempDir(), "mod", "widget")
	mustMkdirAll(t, filepath.Join(pluginDir, "db"))
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"), `<?php
$plugin->component = 'mod_widget';
$plugin->version = 2024010100;
`)
	mustWriteFile(t, filepath.Join(pluginDir, "db", "access.php"), `<?php
$capabilities = [
    'mod/widget:view' => [
        'captype' => 'read',
        'contextlevel' => CONTEXT_MODULE,
    ],
];
`)
	mustWriteFile(t, filepath.Join(pluginDir, "lib.php"), `<?php
function mod_widget_check($context) {
    return has_capability('mod/widget:vew', $context); // typo
}
`)

	results := checkCapabilityUsage(loadDevPlugins("", []string{pluginDir}))
	if len(results) != 1 || results[0].Status != statusWarn {
		t.Fatalf("expected a single warning about mod/widget:vew, got %+v", results)
	}
	if !strings.Contains(results[0].Detail, "mod/widget:vew") {
		t.Errorf("detail missing the typo'd capability name: %q", results[0].Detail)
	}
}

// TestCheckCapabilityUsage_FrankenstyleFormIsNotOwn confirms that the "component:" form, which
// Moodle rejects as a capability name, is not taken for the plugin's own namespace: the doctor
// compares against "type/name:" for every plugin type.
func TestCheckCapabilityUsage_FrankenstyleFormIsNotOwn(t *testing.T) {
	for _, component := range []string{"tool_bar", "report_baz", "assignsubmission_file"} {
		if got, want := extractors.CapabilityPrefix(component), strings.Replace(component, "_", "/", 1); got != want {
			t.Errorf("CapabilityPrefix(%q) = %q, want %q", component, got, want)
		}
	}
	if got := extractors.CapabilityPrefix("core"); got != "" {
		t.Errorf("CapabilityPrefix(core) = %q, want empty", got)
	}
}

// TestCheckCapabilityUsage_NoDevPlugins verifies nil is returned when there are no dev plugins.
func TestCheckCapabilityUsage_NoDevPlugins(t *testing.T) {
	if results := checkCapabilityUsage(nil); results != nil {
		t.Errorf("expected nil with no dev plugins, got %+v", results)
	}
}
