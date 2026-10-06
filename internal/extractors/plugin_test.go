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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pluginFixtureVersionPhp is a version.php declaring component, version, requires, maturity and
// release.
const pluginFixtureVersionPhp = `<?php
defined('MOODLE_INTERNAL') || die();
$plugin->component = 'local_test';
$plugin->version   = 2024010100;
$plugin->requires  = 2023100900;
$plugin->maturity  = MATURITY_STABLE;
$plugin->release   = '1.0.0';
`

// pluginFixtureLangFile is an English lang file that defines the plugin name.
const pluginFixtureLangFile = `<?php
$string['pluginname'] = 'Test Plugin';
`

// TestDetectPlugin verifies the component, version, requirement, maturity and display name read from
// version.php and the lang file.
func TestDetectPlugin(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "lang", "en"))
	mustWriteFile(t, filepath.Join(dir, "version.php"), pluginFixtureVersionPhp)
	mustWriteFile(t, filepath.Join(dir, "lang", "en", "local_test.php"), pluginFixtureLangFile)

	info, err := DetectPlugin(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Component != "local_test" || info.Version != "2024010100" || info.Requires != "2023100900" ||
		info.Maturity != "MATURITY_STABLE" || info.DisplayName != "Test Plugin" {
		t.Errorf("plugin info mismatch: %+v", info)
	}
}

// TestDetectPlugin_NotFound verifies that a missing directory yields a "not found" error.
func TestDetectPlugin_NotFound(t *testing.T) {
	_, err := DetectPlugin("/nonexistent/plugin")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Errorf("expected a 'not found' error, got %v", err)
	}
}

// TestDetectPlugin_ComponentFallsBackToInferredTypeAndDirName verifies that the component falls back to type_name and the display name to
// the directory name when version.php and the lang file do not provide them.
func TestDetectPlugin_ComponentFallsBackToInferredTypeAndDirName(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "local", "noname")
	mustMkdirAll(t, pluginDir)
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"), "<?php\n$version = 2024010100;\n")

	info, err := DetectPlugin(pluginDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Component != "local_noname" {
		t.Errorf("expected fallback component 'local_noname', got %q", info.Component)
	}
	if info.DisplayName != "noname" {
		t.Errorf("expected DisplayName to fall back to the raw dir name, got %q", info.DisplayName)
	}
}

// TestDetectPlugin_AmbiguousParentDirPrefersExactMatch verifies that a plugin whose parent
// directory is "report" resolves to the exact "report" type rather than a suffix match such as
// "gradereport", on every run.
func TestDetectPlugin_AmbiguousParentDirPrefersExactMatch(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "report", "noname")
	mustMkdirAll(t, pluginDir)
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"), "<?php\n$version = 2024010100;\n")

	for i := 0; i < 20; i++ {
		info, err := DetectPlugin(pluginDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Type != "report" {
			t.Fatalf("run %d: expected Type 'report' (exact match), got %q", i, info.Type)
		}
	}
}

// TestDetectPlugin_CommentedOutComponentStillMatches verifies that the regex backend also reads a commented-out component line.
func TestDetectPlugin_CommentedOutComponentStillMatches(t *testing.T) {
	// Only the regex backend reads commented-out lines, so it is selected explicitly.
	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n// $plugin->component = 'old_name';\n")

	info, err := DetectPlugin(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Component != "old_name" {
		t.Errorf("expected the commented-out component to still match (inherited imprecision), got %q", info.Component)
	}
}

// TestDetectPlugin_TreesitterBackendParity verifies that both backends return the same plugin
// info for a well-formed plugin. A commented-out component line is not covered because the
// backends differ on it.
func TestDetectPlugin_TreesitterBackendParity(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "lang", "en"))
	mustWriteFile(t, filepath.Join(dir, "version.php"), pluginFixtureVersionPhp)
	mustWriteFile(t, filepath.Join(dir, "lang", "en", "local_test.php"), pluginFixtureLangFile)

	regexInfo, err := DetectPlugin(dir)
	if err != nil {
		t.Fatalf("regex backend: unexpected error: %v", err)
	}

	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
	tsInfo, err := DetectPlugin(dir)
	if err != nil {
		t.Fatalf("treesitter backend: unexpected error: %v", err)
	}

	if regexInfo.Component != tsInfo.Component || regexInfo.Version != tsInfo.Version ||
		regexInfo.Requires != tsInfo.Requires || regexInfo.Maturity != tsInfo.Maturity {
		t.Errorf("backends disagree: regex=%+v treesitter=%+v", regexInfo, tsInfo)
	}
}

// TestDetectPlugin_TreesitterBackendParity_RealPlugins verifies that both backends agree on real
// plugins from the available Moodle installations.
func TestDetectPlugin_TreesitterBackendParity_RealPlugins(t *testing.T) {
	root := "/srv/workspace/www/html/mdle/dev-500/mod"
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Skipf("real Moodle fixture tree not available: %v", err)
	}

	checked := 0
	for _, e := range entries {
		if checked >= 5 {
			break
		}
		pluginDir := filepath.Join(root, e.Name())
		if !IsPlugin(pluginDir) {
			continue
		}
		checked++

		t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
		regexInfo, err := DetectPlugin(pluginDir)
		if err != nil {
			t.Fatalf("%s: regex backend error: %v", pluginDir, err)
		}
		t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
		tsInfo, err := DetectPlugin(pluginDir)
		if err != nil {
			t.Fatalf("%s: treesitter backend error: %v", pluginDir, err)
		}
		if regexInfo.Component != tsInfo.Component || regexInfo.Version != tsInfo.Version ||
			regexInfo.Requires != tsInfo.Requires || regexInfo.Maturity != tsInfo.Maturity {
			t.Errorf("%s: backends disagree: regex=%+v treesitter=%+v", pluginDir, regexInfo, tsInfo)
		}
	}
	if checked == 0 {
		t.Skip("no real plugins found to sample")
	}
}

// TestIsPlugin verifies that IsPlugin reports a directory containing version.php.
func TestIsPlugin(t *testing.T) {
	dir := t.TempDir()
	if IsPlugin(dir) {
		t.Error("expected false before version.php exists")
	}
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n")
	if !IsPlugin(dir) {
		t.Error("expected true once version.php exists")
	}
}
