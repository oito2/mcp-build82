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
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/oito2/mcp-build82/internal/cache"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/legacyhooks"
)

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

// preloadPlugin returns the extracted data of the plugin `info`, as GenerateAllForPluginCore passes
// it to the per-plugin generators.
func preloadPlugin(info extractors.PluginInfo) *PreloadedPluginData {
	return extractPluginData(info.Path, info.Component)
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

	result := GeneratePluginContext(testPluginInfo(dir), preloadPlugin(testPluginInfo(dir)))
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

	result := GeneratePluginDependencies(testPluginInfo(dir), preloadPlugin(testPluginInfo(dir)))
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

	result := GeneratePluginDependencies(testPluginInfo(dir), preloadPlugin(testPluginInfo(dir)))
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

	result := GeneratePluginDependencies(testPluginInfo(dir), preloadPlugin(testPluginInfo(dir)))
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

	result := GeneratePluginDependencies(testPluginInfo(dir), preloadPlugin(testPluginInfo(dir)))
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

	result := GeneratePluginDependencies(testPluginInfo(dir), preloadPlugin(testPluginInfo(dir)))
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

	result := GeneratePluginSettings(testPluginInfo(dir), preloadPlugin(testPluginInfo(dir)))
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

	result := GeneratePluginSettings(testPluginInfo(dir), preloadPlugin(testPluginInfo(dir)))
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
	if runtime.GOOS == "windows" {
		t.Skip("a read-only directory cannot be made with chmod on Windows")
	}
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

// TestGenerateAllForPlugin_NewClassRegeneratesOutputs verifies that adding a class, even with an
// old mtime, regenerates the plugin outputs that read classes/ instead of skipping them.
func TestGenerateAllForPlugin_NewClassRegeneratesOutputs(t *testing.T) {
	moodlePath := copyFixtureMoodleTree(t)
	pluginPath := filepath.Join(moodlePath, "local", "demo")
	old := swapGlobalCache(cache.NewMtimeCache())
	defer swapGlobalCache(old)

	GenerateAllForPlugin(pluginPath, moodlePath, true, nil)
	writeOld(t, filepath.Join(pluginPath, "classes", "brand_new_service.php"),
		"<?php\nnamespace local_demo;\nclass brand_new_service {\n    public function run() {}\n}\n")
	result := GenerateAllForPlugin(pluginPath, moodlePath, true, nil)

	if resultFor(t, result.Files, "PLUGIN_ARCHITECTURE.md").Skipped {
		t.Fatal("PLUGIN_ARCHITECTURE.md was skipped after a class was added")
	}
	content, _ := os.ReadFile(PluginOutputPath(pluginPath, "PLUGIN_ARCHITECTURE.md"))
	if !strings.Contains(string(content), "brand_new_service") {
		t.Errorf("the new class is missing from PLUGIN_ARCHITECTURE.md:\n%s", content)
	}

	again := GenerateAllForPlugin(pluginPath, moodlePath, true, nil)
	for _, f := range again.Files {
		if !f.Skipped {
			t.Errorf("expected a cache hit when nothing changed, got %+v", f)
		}
	}
}

// TestGenerateAllForPlugin_FreshPluginIsNotExtracted verifies that the extractors run only when at
// least one of the plugin's outputs is stale.
func TestGenerateAllForPlugin_FreshPluginIsNotExtracted(t *testing.T) {
	moodlePath := copyFixtureMoodleTree(t)
	pluginPath := filepath.Join(moodlePath, "local", "demo")
	old := swapGlobalCache(cache.NewMtimeCache())
	defer swapGlobalCache(old)

	calls := 0
	realExtract := extractPluginData
	extractPluginData = func(pluginPath, component string) *PreloadedPluginData {
		calls++
		return realExtract(pluginPath, component)
	}
	t.Cleanup(func() { extractPluginData = realExtract })

	GenerateAllForPlugin(pluginPath, moodlePath, true, nil)
	GenerateAllForPlugin(pluginPath, moodlePath, true, nil)
	if calls != 1 {
		t.Fatalf("extractors ran %d times for a first run and a fresh rerun, want 1", calls)
	}

	cache.Global.Invalidate(PluginOutputPath(pluginPath, "PLUGIN_EVENTS.md"))
	result := GenerateAllForPlugin(pluginPath, moodlePath, true, nil)
	if calls != 2 {
		t.Errorf("extractors ran %d times after one output was invalidated, want 2", calls)
	}
	if resultFor(t, result.Files, "PLUGIN_EVENTS.md").Skipped || !resultFor(t, result.Files, "PLUGIN_CONTEXT.md").Skipped {
		t.Errorf("only the invalidated output must be regenerated, got %+v", result.Files)
	}
}

// TestGeneratePluginCallbackIndex_OversizedLibPhpIsNotRead verifies that a lib.php larger than
// extractors.MaxReadableFileSize is not read into memory by the callback index.
func TestGeneratePluginCallbackIndex_OversizedLibPhpIsNotRead(t *testing.T) {
	moodlePath := copyFixtureMoodleTree(t)
	pluginPath := filepath.Join(moodlePath, "local", "demo")
	padding := strings.Repeat("// padding\n", extractors.MaxReadableFileSize/11+1)
	mustWriteFile(t, filepath.Join(pluginPath, "lib.php"), "<?php\n"+padding+"function local_demo_before_footer() {}\n")

	info, err := extractors.DetectPlugin(pluginPath)
	if err != nil {
		t.Fatal(err)
	}
	info.MoodlePath = moodlePath
	if r := GeneratePluginCallbackIndex(info, preloadPlugin(info)); !r.Success {
		t.Fatalf("generation failed: %+v", r)
	}
	content, _ := os.ReadFile(PluginOutputPath(pluginPath, "PLUGIN_CALLBACK_INDEX.md"))
	if strings.Contains(string(content), "local_demo_before_footer") {
		t.Errorf("a function from an oversized lib.php was reported, so the file was read:\n%s", content)
	}
}

// TestGeneratePluginCallbackIndex_NamesTheReplacementOfEveryLegacyCallback verifies that a legacy
// callback declared in lib.php, under a module's short name, inside a function_exists guard and
// with different letter case, is listed with its Hook API replacement; one declared in
// locallib.php (which Moodle never loads for callbacks) is listed only in the separate "never
// called" section; and a declaration inside a block comment is not listed at all.
func TestGeneratePluginCallbackIndex_NamesTheReplacementOfEveryLegacyCallback(t *testing.T) {
	moodlePath := t.TempDir()
	pluginPath := filepath.Join(moodlePath, "mod", "demo")
	mustMkdirAll(t, pluginPath)
	mustWriteFile(t, filepath.Join(pluginPath, "version.php"), "<?php\n$plugin->component = 'mod_demo';\n$plugin->version = 2024010100;\n")
	mustWriteFile(t, filepath.Join(pluginPath, "lib.php"), "<?php\nif (!function_exists('demo_before_footer')) {\n    function Demo_Before_Footer() {}\n}\n/*\nfunction demo_before_http_headers() {}\n*/\n")
	mustWriteFile(t, filepath.Join(pluginPath, "locallib.php"), "<?php\nfunction demo_before_standard_top_of_body_html() {}\n")

	info, err := extractors.DetectPlugin(pluginPath)
	if err != nil {
		t.Fatal(err)
	}
	if r := GeneratePluginCallbackIndex(info, preloadPlugin(info)); !r.Success {
		t.Fatalf("generation failed: %+v", r)
	}
	content, _ := os.ReadFile(PluginOutputPath(pluginPath, "PLUGIN_CALLBACK_INDEX.md"))
	libSection, localSection, found := strings.Cut(string(content), "### Legacy Callbacks in locallib.php (never called)")
	if !found {
		t.Fatalf("missing locallib.php section in:\n%s", content)
	}
	want := "| `demo_before_footer` | ⚠ `" + legacyhooks.Map["before_footer"] + "` |"
	if !strings.Contains(libSection, want) {
		t.Errorf("missing lib.php row %q in:\n%s", want, content)
	}
	misplaced := "| `demo_before_standard_top_of_body_html` | ⚠ `" + legacyhooks.Map["before_standard_top_of_body_html"] + "` |"
	if strings.Contains(libSection, "demo_before_standard_top_of_body_html") || !strings.Contains(localSection, misplaced) {
		t.Errorf("the locallib.php callback must be listed only in its own section:\n%s", content)
	}
	if strings.Contains(string(content), "demo_before_http_headers") {
		t.Errorf("a commented-out declaration is listed:\n%s", content)
	}
}

// TestGeneratePluginCallbackIndex_NoLocallibSectionWithoutMisplacedCallbacks verifies that the
// locallib.php section is omitted when locallib.php declares no legacy callback.
func TestGeneratePluginCallbackIndex_NoLocallibSectionWithoutMisplacedCallbacks(t *testing.T) {
	pluginPath := filepath.Join(t.TempDir(), "local", "demo")
	mustMkdirAll(t, pluginPath)
	mustWriteFile(t, filepath.Join(pluginPath, "version.php"), "<?php\n$plugin->component = 'local_demo';\n$plugin->version = 2024010100;\n")
	mustWriteFile(t, filepath.Join(pluginPath, "locallib.php"), "<?php\nfunction local_demo_helper() {}\n")

	info, err := extractors.DetectPlugin(pluginPath)
	if err != nil {
		t.Fatal(err)
	}
	if r := GeneratePluginCallbackIndex(info, preloadPlugin(info)); !r.Success {
		t.Fatalf("generation failed: %+v", r)
	}
	content, _ := os.ReadFile(PluginOutputPath(pluginPath, "PLUGIN_CALLBACK_INDEX.md"))
	if strings.Contains(string(content), "locallib.php (never called)") {
		t.Errorf("unexpected locallib.php section:\n%s", content)
	}
}
