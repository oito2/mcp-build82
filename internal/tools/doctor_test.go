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
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/generators"
)

const doctorDeprecatedLibFixture = `<?php
/**
 * Old helper kept for backwards compatibility.
 *
 * @deprecated since Moodle 4.0, use local_test_count_widgets() instead
 * @param int $courseid Course ID
 * @return int
 */
function local_test_legacy_count($courseid) {
    return 0;
}
`

// TestCheckDeprecatedApiUsage_FlagsCallInDevPlugin verifies a dev plugin calling a deprecated
// core function produces a warning naming it.
func TestCheckDeprecatedApiUsage_FlagsCallInDevPlugin(t *testing.T) {
	moodlePath := t.TempDir()
	mustMkdirAll(t, filepath.Join(moodlePath, "lib"))
	mustWriteFile(t, filepath.Join(moodlePath, "lib", "moodlelib.php"), doctorDeprecatedLibFixture)

	pluginDir := filepath.Join(moodlePath, "local", "test")
	mustMkdirAll(t, pluginDir)
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"), `<?php
$plugin->component = 'local_test';
$plugin->version = 2024010100;
`)
	mustWriteFile(t, filepath.Join(pluginDir, "lib.php"), `<?php
function local_test_do_thing($id) {
    return local_test_legacy_count($id);
}
`)

	results := checkDeprecatedApiUsage(moodlePath, loadDevPlugins(moodlePath, []string{pluginDir}))
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 result, got %d: %+v", len(results), results)
	}
	if results[0].Status != statusWarn {
		t.Errorf("expected statusWarn, got %v", results[0].Status)
	}
	if !strings.Contains(results[0].Detail, "local_test") || !strings.Contains(results[0].Detail, "local_test_legacy_count") {
		t.Errorf("detail missing expected component/function name: %q", results[0].Detail)
	}
}

// TestCheckDeprecatedApiUsage_CleanPluginReportsOK verifies a plugin calling no deprecated
// function yields a single passing result.
func TestCheckDeprecatedApiUsage_CleanPluginReportsOK(t *testing.T) {
	moodlePath := t.TempDir()
	mustMkdirAll(t, filepath.Join(moodlePath, "lib"))
	mustWriteFile(t, filepath.Join(moodlePath, "lib", "moodlelib.php"), doctorDeprecatedLibFixture)

	pluginDir := filepath.Join(moodlePath, "local", "clean")
	mustMkdirAll(t, pluginDir)
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"), `<?php
$plugin->component = 'local_clean';
$plugin->version = 2024010100;
`)
	mustWriteFile(t, filepath.Join(pluginDir, "lib.php"), `<?php
function local_clean_do_thing($id) {
    return $id + 1;
}
`)

	results := checkDeprecatedApiUsage(moodlePath, loadDevPlugins(moodlePath, []string{pluginDir}))
	if len(results) != 1 || results[0].Status != statusOK {
		t.Fatalf("expected a single OK result, got %+v", results)
	}
}

// TestCheckDeprecatedApiUsage_UsesCachedApiIndexInsteadOfLiveLibScan verifies the check reads the
// generated MOODLE_API_INDEX.md rather than parsing lib/. lib/moodlelib.php declares no deprecated
// function, but the hand-written index lists one, so only reading the index can produce the
// expected warning.
func TestCheckDeprecatedApiUsage_UsesCachedApiIndexInsteadOfLiveLibScan(t *testing.T) {
	moodlePath := t.TempDir()
	mustMkdirAll(t, filepath.Join(moodlePath, "lib"))
	mustWriteFile(t, filepath.Join(moodlePath, "lib", "moodlelib.php"), `<?php
function local_test_not_deprecated_at_all($x) {
    return $x;
}
`)
	mustMkdirAll(t, filepath.Join(moodlePath, generators.ContextDir))
	mustWriteFile(t, generators.GlobalOutputPath(moodlePath, "MOODLE_API_INDEX.md"),
		"# Moodle API Index\n\n## moodlelib.php\n\n- `cached_only_deprecated_fn()` ~~**@deprecated**~~\n")

	pluginDir := filepath.Join(moodlePath, "local", "test")
	mustMkdirAll(t, pluginDir)
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"), `<?php
$plugin->component = 'local_test';
$plugin->version = 2024010100;
`)
	mustWriteFile(t, filepath.Join(pluginDir, "lib.php"), `<?php
function local_test_do_thing($id) {
    return cached_only_deprecated_fn($id);
}
`)

	results := checkDeprecatedApiUsage(moodlePath, loadDevPlugins(moodlePath, []string{pluginDir}))
	if len(results) != 1 || results[0].Status != statusWarn {
		t.Fatalf("expected a single warning about the cached-index-only deprecated function, got %+v", results)
	}
	if !strings.Contains(results[0].Detail, "cached_only_deprecated_fn") {
		t.Errorf("expected the cached index's function name in the result, got %q", results[0].Detail)
	}
}

// TestCheckDeprecatedApiUsage_NoDevPlugins verifies nil is returned when there are no dev
// plugins.
func TestCheckDeprecatedApiUsage_NoDevPlugins(t *testing.T) {
	moodlePath := t.TempDir()
	if results := checkDeprecatedApiUsage(moodlePath, nil); results != nil {
		t.Errorf("expected nil with no dev plugins, got %+v", results)
	}
}

// callDoctorOverMCP registers only the doctor tool on a fresh server, connects a real client to it
// over the SDK's in-memory transport, and returns the single text block of the tool's response.
func callDoctorOverMCP(t *testing.T, args map[string]any) (string, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	server := mcp.NewServer(&mcp.Implementation{Name: "doctor-test", Version: "0.0.0"}, nil)
	RegisterDoctorTool(server)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "doctor-test-client", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "doctor", Arguments: args})
	if err != nil {
		t.Fatalf("CallTool doctor: %v", err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("expected exactly one content block, got %d", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", res.Content[0])
	}
	return tc.Text, res.IsError
}

// TestDoctor_FormatJSONReturnsStructuredReport verifies the JSON format returns a DoctorOutput
// with every section present.
func TestDoctor_FormatJSONReturnsStructuredReport(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	root := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", root)
	t.Setenv("BUILD82_MOODLE_VERSION", "4.5")

	pluginDir := filepath.Join(root, "local", "demo")
	mustMkdirAll(t, pluginDir)
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"), "<?php\n$plugin->component = 'local_demo';\n$plugin->version = 2024010100;\n")
	mustMkdirAll(t, filepath.Join(pluginDir, generators.ContextDir))
	mustWriteFile(t, filepath.Join(pluginDir, generators.ContextDir, ".indevelopment"), "")

	text, isError := callDoctorOverMCP(t, map[string]any{"format": "json"})
	if isError {
		t.Fatalf("expected a non-error result, got:\n%s", text)
	}

	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("expected a JSON object for format=json, got %v:\n%s", err, text)
	}
	for _, key := range []string{
		"system_dependencies", "configuration", "moodle_installation", "global_index_files",
		"development_plugins", "legacy_files", "cross_plugin_consistency", "deprecated_api_usage",
		"capability_usage", "lang_string_usage", "cache", "verdict",
	} {
		if _, ok := out[key]; !ok {
			t.Errorf("expected section %q in the JSON report, got keys: %v", key, text)
		}
	}

	var report DoctorOutput
	if err := json.Unmarshal([]byte(text), &report); err != nil {
		t.Fatalf("decoding into DoctorOutput: %v", err)
	}
	if len(report.SystemDependencies) != 4 {
		t.Errorf("expected 4 system dependency checks, got %+v", report.SystemDependencies)
	}
	var moodleVersion string
	for _, c := range report.Configuration {
		if c.Label == "Moodle version" {
			moodleVersion = c.Detail
		}
	}
	if moodleVersion != "4.5" {
		t.Errorf("expected the configured Moodle version in the configuration section, got %+v", report.Configuration)
	}
	if len(report.DevelopmentPlugins) != 1 || report.DevelopmentPlugins[0].Component != "local_demo" {
		t.Errorf("expected the local_demo dev plugin to be reported, got %+v", report.DevelopmentPlugins)
	}
	if len(report.DevelopmentPlugins) == 1 && len(report.DevelopmentPlugins[0].Checks) != len(generators.PluginContextFiles) {
		t.Errorf("expected one freshness check per plugin context file, got %+v", report.DevelopmentPlugins[0].Checks)
	}
	// The fixture is not a real Moodle root and has no generated indexes, so the verdict fails.
	if report.Verdict != statusFail {
		t.Errorf("expected verdict %q, got %q", statusFail, report.Verdict)
	}
	if report.Cache == nil {
		t.Error("expected the cache section to be present")
	}
}

// TestDoctor_FormatJSONNotInitializedReportsHint verifies the JSON format of an uninitialized
// installation has a failing verdict and an init hint.
func TestDoctor_FormatJSONNotInitializedReportsHint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("BUILD82_MOODLE_PATH", "")

	text, isError := callDoctorOverMCP(t, map[string]any{"format": "json"})
	if isError {
		t.Fatalf("expected a non-error result, got:\n%s", text)
	}
	var report DoctorOutput
	if err := json.Unmarshal([]byte(text), &report); err != nil {
		t.Fatalf("expected a JSON object for format=json, got %v:\n%s", err, text)
	}
	if report.Verdict != statusFail || !strings.Contains(report.Hint, "init_moodle_context") {
		t.Errorf("expected a failing verdict with an init hint, got %+v", report)
	}
	if len(report.Configuration) != 1 || report.Configuration[0].Detail != "not initialized" {
		t.Errorf("expected a single 'not initialized' configuration check, got %+v", report.Configuration)
	}
}

// TestDoctor_DefaultFormatStaysMarkdown verifies that without a format the report is Markdown.
func TestDoctor_DefaultFormatStaysMarkdown(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("BUILD82_MOODLE_PATH", t.TempDir())

	text, _ := callDoctorOverMCP(t, map[string]any{})
	if !strings.HasPrefix(text, "# build82 Doctor\n") || !strings.Contains(text, "## Lang String Usage") {
		t.Errorf("expected the Markdown report by default, got:\n%s", text)
	}
	if json.Valid([]byte(text)) {
		t.Errorf("expected Markdown, not JSON, by default")
	}
}

// TestDoctor_DetectsEachDevPluginOnce verifies that doctor detects each development plugin a
// single time and shares the result with every check.
func TestDoctor_DetectsEachDevPluginOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	root := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", root)
	t.Setenv("BUILD82_MOODLE_VERSION", "4.5")
	for _, name := range []string{"one", "two"} {
		pluginDir := filepath.Join(root, "local", name)
		mustMkdirAll(t, filepath.Join(pluginDir, generators.ContextDir))
		mustWriteFile(t, filepath.Join(pluginDir, "version.php"), "<?php\n$plugin->component = 'local_"+name+"';\n$plugin->version = 2024010100;\n")
		mustWriteFile(t, filepath.Join(pluginDir, generators.ContextDir, ".indevelopment"), "")
	}

	detections := map[string]int{}
	real := detectPlugin
	detectPlugin = func(dir string) (extractors.PluginInfo, error) {
		detections[dir]++
		return real(dir)
	}
	t.Cleanup(func() { detectPlugin = real })

	callDoctorOverMCP(t, map[string]any{})
	if len(detections) != 2 {
		t.Fatalf("expected both dev plugins detected, got %v", detections)
	}
	for dir, n := range detections {
		if n != 1 {
			t.Errorf("%s was detected %d times, want 1", dir, n)
		}
	}
}

// TestDoctor_UndetectedDevPluginIsNamedByRelativePath verifies that a development plugin whose
// detection fails is named in the report by its directory relative to the Moodle root, never by an
// absolute host path.
func TestDoctor_UndetectedDevPluginIsNamedByRelativePath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	root := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", root)
	t.Setenv("BUILD82_MOODLE_VERSION", "4.5")
	pluginDir := filepath.Join(root, "local", "broken")
	mustMkdirAll(t, filepath.Join(pluginDir, generators.ContextDir))
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"), "<?php\n$plugin->component = 'local_broken';\n$plugin->version = 2024010100;\n")
	mustWriteFile(t, filepath.Join(pluginDir, generators.ContextDir, ".indevelopment"), "")

	real := detectPlugin
	detectPlugin = func(string) (extractors.PluginInfo, error) {
		return extractors.PluginInfo{}, errors.New("unreadable")
	}
	t.Cleanup(func() { detectPlugin = real })

	text, _ := callDoctorOverMCP(t, map[string]any{"format": "json"})
	var out DoctorOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("doctor JSON: %v\n%s", err, text)
	}
	if len(out.DevelopmentPlugins) != 1 || out.DevelopmentPlugins[0].Component != "local/broken" {
		t.Fatalf("development plugins = %+v, want one named local/broken", out.DevelopmentPlugins)
	}
}

// TestLegacyFilesCheck_NamesTheToolThatMigratesEachKind verifies that the legacy-files check names
// update_indexes for global files and generate_plugin_context for plugin files, which are the
// tools whose run moves each kind into .build82/.
func TestLegacyFilesCheck_NamesTheToolThatMigratesEachKind(t *testing.T) {
	if c := legacyFilesCheck(0, 0); c.Status != statusOK {
		t.Errorf("no legacy files: status %s, want ok", c.Status)
	}
	global := legacyFilesCheck(2, 0).Detail
	if !strings.Contains(global, "2 global file(s)") || !strings.Contains(global, "update_indexes") || strings.Contains(global, "generate_plugin_context") {
		t.Errorf("global-only detail = %q", global)
	}
	plugin := legacyFilesCheck(0, 3).Detail
	if !strings.Contains(plugin, "3 plugin file(s)") || !strings.Contains(plugin, "generate_plugin_context") || strings.Contains(plugin, "update_indexes") {
		t.Errorf("plugin-only detail = %q", plugin)
	}
}
