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

package tools

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckCapabilityUsage_FlagsUndeclaredOwnCapability confirms a typo'd own-capability check is
// caught: the plugin checks 'local_test:vew' (typo) but only declares 'local_test:view'.
func TestCheckCapabilityUsage_FlagsUndeclaredOwnCapability(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "local_test")
	mustMkdirAll(t, filepath.Join(pluginDir, "db"))
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"), `<?php
$plugin->component = 'local_test';
$plugin->version = 2024010100;
`)
	mustWriteFile(t, filepath.Join(pluginDir, "db", "access.php"), `<?php
$capabilities = [
    'local_test:view' => [
        'captype' => 'read',
        'contextlevel' => CONTEXT_SYSTEM,
    ],
];
`)
	mustWriteFile(t, filepath.Join(pluginDir, "lib.php"), `<?php
function local_test_check($context) {
    return has_capability('local_test:vew', $context);
}
`)

	results := checkCapabilityUsage([]string{pluginDir})
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 result, got %d: %+v", len(results), results)
	}
	if results[0].Status != statusWarn {
		t.Errorf("expected statusWarn, got %v", results[0].Status)
	}
	if !strings.Contains(results[0].Detail, "local_test:vew") {
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
    'local_test:view' => [
        'captype' => 'read',
        'contextlevel' => CONTEXT_SYSTEM,
    ],
];
`)
	mustWriteFile(t, filepath.Join(pluginDir, "lib.php"), `<?php
function local_test_check($context) {
    return has_capability('local_test:view', $context);
}
`)

	results := checkCapabilityUsage([]string{pluginDir})
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

	results := checkCapabilityUsage([]string{pluginDir})
	if len(results) != 1 || results[0].Status != statusOK {
		t.Fatalf("expected a single OK result (no own-namespace checks made), got %+v", results)
	}
}

// TestCheckCapabilityUsage_ModPluginUsesSlashPrefix confirms a mod plugin's own capability checks
// are cross-referenced against the "mod/{name}:" prefix, not "mod_{name}:" (CapabilityPrefix's own
// documented mod/block special case).
func TestCheckCapabilityUsage_ModPluginUsesSlashPrefix(t *testing.T) {
	// extractors.DetectPlugin infers Type from the parent directory name (inferTypeFromPath) —
	// needs a real "mod/" parent directory here for info.Type to resolve to "mod".
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

	results := checkCapabilityUsage([]string{pluginDir})
	if len(results) != 1 || results[0].Status != statusWarn {
		t.Fatalf("expected a single warning about mod/widget:vew, got %+v", results)
	}
	if !strings.Contains(results[0].Detail, "mod/widget:vew") {
		t.Errorf("detail missing the typo'd capability name: %q", results[0].Detail)
	}
}

func TestCheckCapabilityUsage_NoDevPlugins(t *testing.T) {
	if results := checkCapabilityUsage(nil); results != nil {
		t.Errorf("expected nil with no dev plugins, got %+v", results)
	}
}
