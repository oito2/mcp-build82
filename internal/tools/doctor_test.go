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
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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

	results := checkDeprecatedApiUsage(moodlePath, []string{pluginDir})
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

	results := checkDeprecatedApiUsage(moodlePath, []string{pluginDir})
	if len(results) != 1 || results[0].Status != statusOK {
		t.Fatalf("expected a single OK result, got %+v", results)
	}
}

// TestCheckDeprecatedApiUsage_UsesCachedApiIndexInsteadOfLiveLibScan confirms this check reads the
// cached MOODLE_API_INDEX.md rather than re-running extractors.ExtractMoodleApi (a full parse of
// every file in lib/) on every doctor call. The test shows the cached index is actually consulted:
// lib/moodlelib.php has zero @deprecated functions live — a live
// extraction would find nothing — but a hand-written MOODLE_API_INDEX.md lists one deprecated
// function that doesn't exist in the live source at all. Only reading the cached file, not
// re-parsing lib/, can produce the result this test asserts.
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

	results := checkDeprecatedApiUsage(moodlePath, []string{pluginDir})
	if len(results) != 1 || results[0].Status != statusWarn {
		t.Fatalf("expected a single warning about the cached-index-only deprecated function, got %+v", results)
	}
	if !strings.Contains(results[0].Detail, "cached_only_deprecated_fn") {
		t.Errorf("expected the cached index's function name in the result, got %q", results[0].Detail)
	}
}

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
	if len(report.SystemDependencies) != 3 {
		t.Errorf("expected 3 system dependency checks, got %+v", report.SystemDependencies)
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
	// The fixture is not a real Moodle root and has no generated indexes, so the verdict must fail.
	if report.Verdict != statusFail {
		t.Errorf("expected verdict %q, got %q", statusFail, report.Verdict)
	}
	if report.Cache == nil {
		t.Error("expected the cache section to be present")
	}
}

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
