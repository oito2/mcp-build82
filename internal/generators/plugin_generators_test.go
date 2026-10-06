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

package generators

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/oito2/mcp-build82/internal/cache"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/legacyhooks"
)

// TestLegacyCallbackSuffixesForIndex_MatchesLegacyhooksMap verifies that the callback suffix list
// is derived from legacyhooks.Map: every key present, in sorted order, nothing extra.
func TestLegacyCallbackSuffixesForIndex_MatchesLegacyhooksMap(t *testing.T) {
	want := make([]string, 0, len(legacyhooks.Map))
	for suffix := range legacyhooks.Map {
		want = append(want, suffix)
	}
	sort.Strings(want)

	if len(legacyCallbackSuffixesForIndex) != len(want) {
		t.Fatalf("got %d suffixes, want %d: %v vs %v", len(legacyCallbackSuffixesForIndex), len(want), legacyCallbackSuffixesForIndex, want)
	}
	for i, suffix := range want {
		if legacyCallbackSuffixesForIndex[i] != suffix {
			t.Errorf("index %d: got %q, want %q (legacyCallbackSuffixesForIndex: %v)", i, legacyCallbackSuffixesForIndex[i], suffix, legacyCallbackSuffixesForIndex)
		}
	}
}

// TestBuildDirectoryTree_ExcludesDotfilesVendorNodeModules verifies the directory tree omits dot-directories, vendor and node_modules.
func TestBuildDirectoryTree_ExcludesDotfilesVendorNodeModules(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, ContextDir))
	mustMkdirAll(t, filepath.Join(dir, "vendor"))
	mustMkdirAll(t, filepath.Join(dir, "node_modules"))
	mustMkdirAll(t, filepath.Join(dir, "classes"))
	mustWriteFile(t, filepath.Join(dir, "lib.php"), "<?php\n")

	var b strings.Builder
	buildDirectoryTree(dir, dir, 0, "", &b)
	tree := b.String()

	if strings.Contains(tree, ContextDir) || strings.Contains(tree, "vendor") || strings.Contains(tree, "node_modules") {
		t.Errorf("expected dotfiles/vendor/node_modules excluded, got:\n%s", tree)
	}
	if !strings.Contains(tree, "lib.php") || !strings.Contains(tree, "classes") {
		t.Errorf("expected real entries present, got:\n%s", tree)
	}
}

// TestFunctionExistsInContent_SpaceBeforeParenAndCaseInsensitive verifies that
// functionExistsInContent matches a declaration with a space before the parenthesis and a
// differently-cased function name (PHP function names are case-insensitive).
func TestFunctionExistsInContent_SpaceBeforeParenAndCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lib.php")
	mustWriteFile(t, path, "<?php\nfunction local_test_cron ()\n{\n}\n")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !functionExistsInContent(content, "local_test_cron") {
		t.Error("expected a match despite the space before the parenthesis")
	}

	mustWriteFile(t, path, "<?php\nfunction Local_Test_Cron() {}\n")
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !functionExistsInContent(content, "local_test_cron") {
		t.Error("expected a case-insensitive match (PHP function names are case-insensitive)")
	}
}

// testPluginInfo returns the PluginInfo detected at `path`, ignoring detection errors.
func testPluginInfo(path string) extractors.PluginInfo {
	info, _ := extractors.DetectPlugin(path)
	return info
}

// TestGeneratePluginContext verifies PLUGIN_CONTEXT.md is generated successfully for a plugin.
func TestGeneratePluginContext(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$plugin->component = 'local_test';\n$plugin->version = 2024010100;\n")
	mustWriteFile(t, filepath.Join(dir, "db", "events.php"),
		"<?php\n$observers = [['eventname' => '\\\\core\\\\event\\\\x', 'callback' => 'y::z']];")

	result := GeneratePluginContext(testPluginInfo(dir), nil)
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	content, _ := os.ReadFile(PluginOutputPath(dir, "PLUGIN_CONTEXT.md"))
	if !strings.Contains(string(content), "local_test") {
		t.Errorf("expected component name in output, got:\n%s", content)
	}
	if !strings.Contains(string(content), "Event observers | 1") {
		t.Errorf("expected event count reflected, got:\n%s", content)
	}
}

// TestGeneratePluginDependencies_HookApiConditionalSections verifies the Hook API note and the legacy warning are rendered independently.
func TestGeneratePluginDependencies_HookApiConditionalSections(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$plugin->component = 'local_test';\n")
	mustWriteFile(t, filepath.Join(dir, "lib.php"), "<?php\nfunction local_test_before_footer() {}\n")

	result := GeneratePluginDependencies(testPluginInfo(dir), nil)
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	content, _ := os.ReadFile(PluginOutputPath(dir, "PLUGIN_DEPENDENCIES.md"))
	s := string(content)

	// No hook adoption -> the "doesn't use hooks" line...
	if !strings.Contains(s, "does not use the Hook API") {
		t.Errorf("expected the no-hook-adoption line, got:\n%s", s)
	}
	// ...AND the legacy migration warning, simultaneously (independent conditionals).
	if !strings.Contains(s, "Legacy Callbacks") || !strings.Contains(s, "local_test_before_footer") {
		t.Errorf("expected the legacy callback warning alongside the no-hook-adoption line, got:\n%s", s)
	}
}

// TestGeneratePluginDependencies_RegisteredCallbacksSection verifies registered hook callbacks appear in the Hook API section.
func TestGeneratePluginDependencies_RegisteredCallbacksSection(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$plugin->component = 'local_test';\n")
	mustWriteFile(t, filepath.Join(dir, "db", "hooks.php"), `<?php
$callbacks = [['hookname' => '\\core\\hook\\output\\before_footer', 'callback' => 'x::y']];`)

	result := GeneratePluginDependencies(testPluginInfo(dir), nil)
	content, _ := os.ReadFile(PluginOutputPath(dir, "PLUGIN_DEPENDENCIES.md"))
	s := string(content)
	if !result.Success || strings.Contains(s, "does not use the Hook API") {
		t.Errorf("expected hook adoption detected (no 'does not use' line), got:\n%s", s)
	}
	if !strings.Contains(s, "Registered Callbacks") {
		t.Errorf("expected the registered-callbacks section, got:\n%s", s)
	}
}

// TestGeneratePluginDependencies_SubpluginsFromJson verifies a db/subplugins.json declaration
// appears in the generated Subplugins section.
func TestGeneratePluginDependencies_SubpluginsFromJson(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$plugin->component = 'mod_workshop';\n")
	mustWriteFile(t, filepath.Join(dir, "db", "subplugins.json"), `{
  "plugintypes": {
    "workshopform": "mod/workshop/form",
    "workshopallocation": "mod/workshop/allocation"
  }
}`)

	result := GeneratePluginDependencies(testPluginInfo(dir), nil)
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	content, _ := os.ReadFile(PluginOutputPath(dir, "PLUGIN_DEPENDENCIES.md"))
	s := string(content)
	if !strings.Contains(s, "workshopform") || !strings.Contains(s, "mod/workshop/form") {
		t.Errorf("expected the subplugins.json declaration reflected, got:\n%s", s)
	}
	if !strings.Contains(s, "workshopallocation") {
		t.Errorf("expected the second subplugin type reflected, got:\n%s", s)
	}
}

// TestGeneratePluginDependencies_SubpluginsFromLegacyPhp verifies the db/subplugins.php array
// format is read as a fallback when subplugins.json does not exist.
func TestGeneratePluginDependencies_SubpluginsFromLegacyPhp(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$plugin->component = 'mod_workshop';\n")
	mustWriteFile(t, filepath.Join(dir, "db", "subplugins.php"), `<?php
$subplugins = array(
    'workshopeval' => 'mod/workshop/eval',
);
`)

	result := GeneratePluginDependencies(testPluginInfo(dir), nil)
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	content, _ := os.ReadFile(PluginOutputPath(dir, "PLUGIN_DEPENDENCIES.md"))
	s := string(content)
	if !strings.Contains(s, "workshopeval") || !strings.Contains(s, "mod/workshop/eval") {
		t.Errorf("expected the legacy subplugins.php declaration reflected, got:\n%s", s)
	}
}

// TestGeneratePluginDependencies_NoSubpluginsShowsPlaceholder verifies a plugin without subplugins
// renders the explicit "does not host" placeholder.
func TestGeneratePluginDependencies_NoSubpluginsShowsPlaceholder(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$plugin->component = 'local_test';\n")

	result := GeneratePluginDependencies(testPluginInfo(dir), nil)
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	content, _ := os.ReadFile(PluginOutputPath(dir, "PLUGIN_DEPENDENCIES.md"))
	if !strings.Contains(string(content), "does not host subplugins") {
		t.Errorf("expected the no-subplugins placeholder, got:\n%s", content)
	}
}

// TestGenerateAllForPlugin_SettingsPhpChangeTriggersRegeneration verifies that a settings.php-only
// change after the first generation is detected by the cache and regenerates PLUGIN_SETTINGS.md.
func TestGenerateAllForPlugin_SettingsPhpChangeTriggersRegeneration(t *testing.T) {
	moodlePath := copyFixtureMoodleTree(t)
	pluginPath := filepath.Join(moodlePath, "local", "demo")

	c := cache.NewMtimeCache()
	old := swapGlobalCache(c)
	defer swapGlobalCache(old)

	GenerateAllForPlugin(pluginPath, moodlePath, true, nil)

	// Regenerate immediately with nothing changed: every file should be a cache hit.
	result := GenerateAllForPlugin(pluginPath, moodlePath, true, nil)
	for _, f := range result.Files {
		if !f.Skipped {
			t.Errorf("expected a cache hit before any change, got %+v", f)
		}
	}

	// Add settings.php with an mtime one hour in the future so it is unambiguously newer than the
	// cache mark, avoiding mtime-resolution flakiness.
	settingsPath := filepath.Join(pluginPath, "settings.php")
	mustWriteFile(t, settingsPath, "<?php\n$settings->add(new admin_setting_configtext('local_demo/x', '', '', ''));\n")
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(settingsPath, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	result = GenerateAllForPlugin(pluginPath, moodlePath, true, nil)
	var settingsResult *GeneratorResult
	for i := range result.Files {
		if strings.HasSuffix(result.Files[i].File, "PLUGIN_SETTINGS.md") {
			settingsResult = &result.Files[i]
		}
	}
	if settingsResult == nil {
		t.Fatal("expected a PLUGIN_SETTINGS.md result")
	}
	if settingsResult.Skipped {
		t.Error("expected PLUGIN_SETTINGS.md to be regenerated after settings.php changed, not skipped as still fresh")
	}
	content, _ := os.ReadFile(PluginOutputPath(pluginPath, "PLUGIN_SETTINGS.md"))
	if !strings.Contains(string(content), "local_demo/x") {
		t.Errorf("expected the regenerated file to reflect the new setting, got:\n%s", content)
	}
}

// TestGeneratePluginSettings_ReflectsDeclaredSettings verifies declared admin settings appear in PLUGIN_SETTINGS.md.
func TestGeneratePluginSettings_ReflectsDeclaredSettings(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$plugin->component = 'local_test';\n")
	mustWriteFile(t, filepath.Join(dir, "settings.php"), `<?php
if ($hassiteconfig) {
    $settings->add(new admin_setting_configtext('local_test/apikey', get_string('apikey', 'local_test'), '', ''));
}
`)

	result := GeneratePluginSettings(testPluginInfo(dir), nil)
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	content, _ := os.ReadFile(PluginOutputPath(dir, "PLUGIN_SETTINGS.md"))
	s := string(content)
	if !strings.Contains(s, "local_test/apikey") || !strings.Contains(s, "admin_setting_configtext") {
		t.Errorf("expected the declared setting reflected, got:\n%s", s)
	}
}

// TestGeneratePluginSettings_NoSettingsShowsPlaceholder verifies a plugin without settings renders the placeholder text.
func TestGeneratePluginSettings_NoSettingsShowsPlaceholder(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$plugin->component = 'local_test';\n")

	result := GeneratePluginSettings(testPluginInfo(dir), nil)
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	content, _ := os.ReadFile(PluginOutputPath(dir, "PLUGIN_SETTINGS.md"))
	if !strings.Contains(string(content), "no admin_setting_") {
		t.Errorf("expected the no-settings placeholder, got:\n%s", content)
	}
}

// TestGeneratePluginStructure_KeyFilesTable verifies the key-files table marks only the files that exist.
func TestGeneratePluginStructure_KeyFilesTable(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$plugin->component = 'local_test';\n")
	mustWriteFile(t, filepath.Join(dir, "db", "install.xml"), "<?xml version=\"1.0\"?><XMLDB></XMLDB>")

	result := GeneratePluginStructure(testPluginInfo(dir))
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	content, _ := os.ReadFile(PluginOutputPath(dir, "PLUGIN_STRUCTURE.md"))
	s := string(content)
	if !strings.Contains(s, "`db/install.xml` | ✔") {
		t.Errorf("expected db/install.xml marked present, got:\n%s", s)
	}
	if !strings.Contains(s, "`settings.php` | \n") && !strings.Contains(s, "`settings.php` | |") {
		// settings.php does not exist, so the row is present but unmarked.
		if !strings.Contains(s, "settings.php") {
			t.Errorf("expected settings.php row present even though absent, got:\n%s", s)
		}
	}
}

// TestGenerateAllForPlugin_Integration verifies GenerateAllForPlugin writes every plugin context file with successful results.
func TestGenerateAllForPlugin_Integration(t *testing.T) {
	moodlePath := copyFixtureMoodleTree(t)
	pluginPath := filepath.Join(moodlePath, "local", "demo")

	c := cache.NewMtimeCache()
	old := swapGlobalCache(c)
	defer swapGlobalCache(old)

	result := GenerateAllForPlugin(pluginPath, moodlePath, true, nil)
	if result.Plugin != "local_demo" {
		t.Errorf("expected component local_demo, got %q", result.Plugin)
	}
	if len(result.Files) != 12 {
		t.Fatalf("expected 12 generator results, got %d: %+v", len(result.Files), result.Files)
	}
	for _, f := range result.Files {
		if !f.Success {
			t.Errorf("generator result was not successful: %+v", f)
		}
	}

	for _, f := range PluginContextFiles {
		if _, err := os.Stat(PluginOutputPath(pluginPath, f)); err != nil {
			t.Errorf("expected %s to exist, got error: %v", f, err)
		}
	}
	for _, f := range PluginContextFiles {
		if _, err := os.Stat(filepath.Join(pluginPath, f)); err == nil {
			t.Errorf("expected %s to NOT exist at the plugin root", f)
		}
	}

	if _, err := os.Stat(PluginOutputPath(pluginPath, ".indevelopment")); err != nil {
		t.Errorf("expected .indevelopment marker to exist since markAsDev=true: %v", err)
	}

	// Cross-check the combined AI context file references real extracted data.
	aiContext, _ := os.ReadFile(PluginOutputPath(pluginPath, "PLUGIN_AI_CONTEXT.md"))
	if !strings.Contains(string(aiContext), "local_demo") {
		t.Errorf("expected component name in PLUGIN_AI_CONTEXT.md, got:\n%s", aiContext)
	}
}

// TestGenerateAllForPlugin_MinimalPlugin verifies every generator copes with the nil extraction
// results returned when a plugin lacks the db/*.php files.
func TestGenerateAllForPlugin_MinimalPlugin(t *testing.T) {
	moodlePath := t.TempDir()
	pluginPath := filepath.Join(moodlePath, "local", "bare")
	mustMkdirAll(t, pluginPath)
	mustWriteFile(t, filepath.Join(pluginPath, "version.php"), "<?php\n$plugin->component = 'local_bare';\n$plugin->version = 2024010100;\n")

	c := cache.NewMtimeCache()
	old := swapGlobalCache(c)
	defer swapGlobalCache(old)

	result := GenerateAllForPlugin(pluginPath, moodlePath, false, nil)
	if result.Plugin != "local_bare" {
		t.Errorf("expected component local_bare, got %q", result.Plugin)
	}
	for _, f := range result.Files {
		if !f.Success {
			t.Errorf("expected every generator to succeed against a plugin missing most db/*.php files, got %+v", f)
		}
	}
}

// TestGenerateAllForPlugin_LogsIndevelopmentMarkerWriteFailureToStderr verifies that a failure to
// write the .indevelopment marker (e.g. an unwritable .build82/ directory) is logged to stderr.
func TestGenerateAllForPlugin_LogsIndevelopmentMarkerWriteFailureToStderr(t *testing.T) {
	moodlePath := copyFixtureMoodleTree(t)
	pluginPath := filepath.Join(moodlePath, "local", "demo")
	contextDir := filepath.Join(pluginPath, ContextDir)
	mustMkdirAll(t, contextDir)
	if err := os.Chmod(contextDir, 0o555); err != nil { // read+execute, no write
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(contextDir, 0o755) }) // let t.TempDir() clean up afterward

	c := cache.NewMtimeCache()
	old := swapGlobalCache(c)
	defer swapGlobalCache(old)

	stderr := captureStderr(t, func() { GenerateAllForPlugin(pluginPath, moodlePath, true, nil) })

	if !strings.Contains(stderr, ".indevelopment") {
		t.Errorf("expected the marker write failure logged to stderr, got:\n%s", stderr)
	}
	if _, err := os.Stat(filepath.Join(contextDir, ".indevelopment")); err == nil {
		t.Error("expected the marker write to have actually failed (file should not exist)")
	}
}

// TestGenerateAllForPlugin_MarkAsDevFalse verifies no .indevelopment marker is written when markAsDev is false.
func TestGenerateAllForPlugin_MarkAsDevFalse(t *testing.T) {
	moodlePath := copyFixtureMoodleTree(t)
	pluginPath := filepath.Join(moodlePath, "local", "demo")

	c := cache.NewMtimeCache()
	old := swapGlobalCache(c)
	defer swapGlobalCache(old)

	GenerateAllForPlugin(pluginPath, moodlePath, false, nil)
	if _, err := os.Stat(PluginOutputPath(pluginPath, ".indevelopment")); err == nil {
		t.Error("expected no .indevelopment marker when markAsDev=false")
	}
}

// TestFunctionExistsInContent_IgnoresIndentedMethods verifies only top-level declarations match.
func TestFunctionExistsInContent_IgnoresIndentedMethods(t *testing.T) {
	src := []byte("<?php\nclass x {\n    public function local_a_cron() {}\n}\nfunction local_b_cron() {}\n")
	if functionExistsInContent(src, "local_a_cron") {
		t.Error("indented method must not match")
	}
	if !functionExistsInContent(src, "local_b_cron") {
		t.Error("top-level function must match")
	}
}
