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
	"slices"
	"strings"
	"testing"

	"github.com/oito2/mcp-build82/internal/moodletype"
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

// TestDetectPlugin_CommentedOutComponentIsIgnored verifies that the regex backend, like the
// tree-sitter one, does not read a commented-out component line.
func TestDetectPlugin_CommentedOutComponentIsIgnored(t *testing.T) {
	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n// $plugin->component = 'old_name';\n")

	info, err := DetectPlugin(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Component == "old_name" {
		t.Error("the commented-out component was read")
	}
}

// TestDetectPlugin_TreesitterBackendParity verifies that both backends return the same plugin
// info for a well-formed plugin.
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
	root := filepath.Join(realMoodle("mdle-500"), "mod")
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

// TestInferTypeFromPath_LongestTypeDirectoryWins verifies that the type comes from the longest
// type directory the plugin path ends with, so types sharing a trailing segment are told apart.
func TestInferTypeFromPath_LongestTypeDirectoryWins(t *testing.T) {
	for path, want := range map[string]string{
		"/m/question/type/ordering":   "qtype",
		"/m/calendar/type/gregorian":  "calendartype",
		"/m/grade/report/grader":      "gradereport",
		"/m/report/log":               "report",
		"/m/mod/scorm/report/basic":   "scormreport",
		"/m/mod/forum":                "mod",
		"/m/admin/tool/usertours":     "tool",
		"/m/local/demo":               "local",
		"/m/somewhere/unknown/plugin": "unknown",
	} {
		if got := inferTypeFromPath(filepath.FromSlash(path)); got != want {
			t.Errorf("inferTypeFromPath(%q) = %q, want %q", path, got, want)
		}
	}
}

// TestDetectPlugin_TypeMatchesComponentOnRealInstall verifies, on a real Moodle installation, that
// the type inferred from each plugin's path is the prefix of the component its version.php
// declares. It is skipped when the installation is absent.
func TestDetectPlugin_TypeMatchesComponentOnRealInstall(t *testing.T) {
	root := realMoodle("mdle-500")
	if _, err := os.Stat(filepath.Join(root, "version.php")); err != nil {
		t.Skipf("real Moodle install not available: %v", err)
	}
	checked := 0
	for typ, dir := range moodletype.PluginTypeToDir {
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil {
			continue
		}
		for _, e := range entries {
			pluginDir := filepath.Join(root, filepath.FromSlash(dir), e.Name())
			if !e.IsDir() || !IsPlugin(pluginDir) {
				continue
			}
			info, err := DetectPlugin(pluginDir)
			if err != nil || info.Component == "" {
				continue
			}
			checked++
			if prefix, _, _ := strings.Cut(info.Component, "_"); info.Type != prefix {
				t.Errorf("%s: type %q, but the component %q says %q (directory type %q)", pluginDir, info.Type, info.Component, prefix, typ)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no plugin checked")
	}
}

// TestValidComponent verifies the frankenstyle component check.
func TestValidComponent(t *testing.T) {
	for c, want := range map[string]bool{
		"local_myplugin": true, "mod_forum": true, "assignsubmission_file": true, "qtype_ddwtos": true,
		"tool_mfa_factor": true, "local_x2": true,
		"": false, "local": false, "_x": false, "Local_x": false, "local_": false, "1x_y": false,
		"../../x": false, "local_../../x": false, "local/x": false, `local\x`: false, "local_x y": false,
	} {
		if got := ValidComponent(c); got != want {
			t.Errorf("ValidComponent(%q) = %v, want %v", c, got, want)
		}
	}
}

// TestDetectPlugin_InvalidComponentFallsBack verifies that a version.php component that is not a
// frankenstyle name (here one that would traverse out of the plugin) is replaced by type_name, so
// it never reaches a file path such as lang/en/<component>.php.
func TestDetectPlugin_InvalidComponentFallsBack(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "local", "evil")
	mustMkdirAll(t, filepath.Join(dir, "lang", "en"))
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$plugin->component = '../../../secret';\n$plugin->version = 2024010100;\n")
	mustWriteFile(t, filepath.Join(root, "secret.php"), "<?php\n$string['pluginname'] = 'LEAKED';\n")

	info, err := DetectPlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Component != "local_evil" || info.DisplayName == "LEAKED" {
		t.Errorf("expected the type_name fallback and no leaked display name, got %+v", info)
	}
}

// TestReadVersionPhp_DecimalVersionsAndCommentedAssignments verifies, on both backends, that a
// decimal $plugin->version keeps its decimal part, that commented-out assignments are ignored, and
// that the last real assignment wins.
func TestReadVersionPhp_DecimalVersionsAndCommentedAssignments(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "version.php"), []byte(`<?php
$plugin->component = 'local_old';
$plugin->component = 'local_demo';
// $plugin->component = 'local_commented';
$plugin->version = 2024010100.01;
/* $plugin->version = 2099010100; */
$plugin->requires = 2022112800; // $plugin->requires = 1;
$plugin->maturity = MATURITY_STABLE;
# $plugin->maturity = MATURITY_ALPHA;
`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, backend := range []string{"", "treesitter"} {
		t.Run("backend="+backend, func(t *testing.T) {
			t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
			component, version, requires, maturity := readVersionPhp(dir)
			got := []string{component, version, requires, maturity}
			want := []string{"local_demo", "2024010100.01", "2022112800", "MATURITY_STABLE"}
			if !slices.Equal(got, want) {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}
