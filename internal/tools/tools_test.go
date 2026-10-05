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
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/cache"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/generators"
)

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func freshCache(t *testing.T) {
	t.Helper()
	old := cache.Global
	cache.Global = cache.NewMtimeCache()
	t.Cleanup(func() { cache.Global = old })
}

// captureStderr redirects os.Stderr for the duration of fn and returns everything written to it
// (best-effort warnings write directly to os.Stderr, with no injectable io.Writer).
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	return string(out)
}

// TestProcessPlugin_NeverAbortsOnBadPlugin is the single most important behavior in plugin_batch:
// one plugin's failure must never stop the batch, nor panic past the caller. A genuinely broken
// plugin here is a path that exists as a *file*, not a directory — every generator's write() call
// (os.MkdirAll into a path component that's already a file) will hard-fail, unlike a merely
// nonexistent directory, which the extractors treat as "no source data" and generate successfully.
func TestProcessPlugin_NeverAbortsOnBadPlugin(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	mustMkdirAll(t, filepath.Join(moodlePath, "local"))
	badPath := filepath.Join(moodlePath, "local", "not_a_directory")
	mustWriteFile(t, badPath, "this is a file, not a plugin directory")

	result := processPlugin(badPath, moodlePath, false, false)
	if result.Failed == 0 {
		t.Errorf("expected failures for a plugin path that's a file, not a directory, got %+v", result)
	}
	// Reaching this line at all (no panic escaping processPlugin) is itself part of the assertion.
}

func TestProcessPlugin_ContinuesAfterOneBadPlugin(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()

	goodPath := filepath.Join(moodlePath, "local", "good")
	mustMkdirAll(t, goodPath)
	mustWriteFile(t, filepath.Join(goodPath, "version.php"), "<?php\n$plugin->component = 'local_good';\n$plugin->version = 2024010100;\n")

	mustMkdirAll(t, filepath.Join(moodlePath, "local"))
	badPath := filepath.Join(moodlePath, "local", "not_a_directory")
	mustWriteFile(t, badPath, "this is a file, not a plugin directory")

	results := []struct {
		path string
	}{{badPath}, {goodPath}}

	var summaries []int
	for _, r := range results {
		res := processPlugin(r.path, moodlePath, false, false)
		summaries = append(summaries, res.Generated)
	}

	// The good plugin, processed *after* the bad one, must still succeed.
	if summaries[1] == 0 {
		t.Errorf("expected the good plugin to succeed even after a bad one preceded it, got %v", summaries)
	}
}

func TestResolveBatchPlugins_ListMode_UnresolvedIsError(t *testing.T) {
	moodlePath := t.TempDir()
	_, _, errResult := resolveBatchPlugins(BatchInput{Mode: BatchModeList, Plugins: []string{"local_nonexistent"}}, moodlePath)
	if errResult == nil || !errResult.IsError {
		t.Fatal("expected an IsError result for an unresolvable plugin identifier")
	}
}

// TestResolveBatchPlugins_ListMode_TooManyPluginsIsError verifies that mode="list" caps len(Plugins)
// at maxListPlugins, so a single request cannot force thousands of
// DetectPlugin/GenerateAllForPlugin calls.
func TestResolveBatchPlugins_ListMode_TooManyPluginsIsError(t *testing.T) {
	moodlePath := t.TempDir()
	plugins := make([]string, maxListPlugins+1)
	for i := range plugins {
		plugins[i] = "local_nonexistent"
	}
	_, _, errResult := resolveBatchPlugins(BatchInput{Mode: BatchModeList, Plugins: plugins}, moodlePath)
	if errResult == nil || !errResult.IsError {
		t.Fatal("expected an IsError result when len(Plugins) exceeds maxListPlugins")
	}
}

func TestResolveBatchPlugins_DevMode_EmptyIsInformationalNotError(t *testing.T) {
	moodlePath := t.TempDir()
	dirs, _, result := resolveBatchPlugins(BatchInput{Mode: BatchModeDev}, moodlePath)
	if dirs != nil {
		t.Errorf("expected nil dirs, got %v", dirs)
	}
	if result == nil || result.IsError {
		t.Fatal("expected a non-error informational result when no dev plugins exist")
	}
}

// TestWithRecover_ConvertsPanicToErrorResult verifies that withRecover converts a panic inside a
// handler (most likely from an extractor/generator hitting malformed plugin source) into a normal
// IsError result instead of letting the panic escape.
func TestWithRecover_ConvertsPanicToErrorResult(t *testing.T) {
	panicky := func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, struct{}, error) {
		panic("boom: simulated extractor failure")
	}
	wrapped := withRecover(panicky)

	result, _, err := wrapped(context.Background(), &mcp.CallToolRequest{}, struct{}{})
	if err != nil {
		t.Fatalf("expected no Go error (panic should be converted to an IsError result), got: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("expected an IsError result, got: %+v", result)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected exactly one content block, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(tc.Text, "boom: simulated extractor failure") {
		t.Errorf("expected the panic message in the result text, got: %+v", result.Content[0])
	}
}

// TestWithRecover_PassesThroughNormalResults confirms the wrapper is transparent when the handler
// doesn't panic — it must return exactly what the handler returned, not swallow or alter it.
func TestWithRecover_PassesThroughNormalResults(t *testing.T) {
	normal := func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, struct{}, error) {
		return textResult(false, "all good"), struct{}{}, nil
	}
	wrapped := withRecover(normal)

	result, _, err := wrapped(context.Background(), &mcp.CallToolRequest{}, struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false to pass through unchanged")
	}
	if tc, ok := result.Content[0].(*mcp.TextContent); !ok || tc.Text != "all good" {
		t.Errorf("expected the handler's own result to pass through unchanged, got: %+v", result.Content[0])
	}
}

// TestClassifyResults_RelativizesAndPreservesErrorMessage verifies that classifyResults
// relativizes every bucket (generated, skipped, failed) and preserves the error message of failed
// results.
func TestClassifyResults_RelativizesAndPreservesErrorMessage(t *testing.T) {
	base := "/srv/moodle"
	results := []generators.GeneratorResult{
		{File: "/srv/moodle/.build82/AI_CONTEXT.md", Success: true},
		{File: "/srv/moodle/.build82/MOODLE_API_INDEX.md", Success: true, Skipped: true},
		{File: "/srv/moodle/.build82/MOODLE_EVENTS_INDEX.md", Success: false, Error: "disk full"},
	}
	generated, skipped, failed := classifyResults(results, base)

	if len(generated) != 1 || generated[0] != ".build82/AI_CONTEXT.md" {
		t.Errorf("expected 1 relativized generated entry, got %v", generated)
	}
	if len(skipped) != 1 || skipped[0] != ".build82/MOODLE_API_INDEX.md" {
		t.Errorf("expected 1 relativized skipped entry, got %v", skipped)
	}
	if len(failed) != 1 || failed[0].File != ".build82/MOODLE_EVENTS_INDEX.md" || failed[0].Error != "disk full" {
		t.Errorf("expected 1 relativized failed entry with its error message preserved, got %+v", failed)
	}
}

func TestFilterFunctionLines_VisibilityMarker(t *testing.T) {
	lines := []string{
		"- `public_fn()` — does a thing",
		"- `deprecated_fn()` ~~**@deprecated**: use x instead~~",
		"not a function line",
		"## a heading",
	}
	got := filterFunctionLines(lines)
	if len(got) != 2 {
		t.Fatalf("expected 2 function lines, got %d: %v", len(got), got)
	}
}

func TestSearchApiVisibilityFilter(t *testing.T) {
	lines := []string{
		"- `public_fn()` — a public function",
		"- `deprecated_fn()` ~~**@deprecated**: old~~",
	}
	var publicOnly, deprecatedOnly, all []string
	for _, l := range lines {
		hasDeprecated := len(l) > 0 && containsDeprecatedMarker(l)
		if !hasDeprecated {
			publicOnly = append(publicOnly, l)
		} else {
			deprecatedOnly = append(deprecatedOnly, l)
		}
		all = append(all, l)
	}
	if len(publicOnly) != 1 || len(deprecatedOnly) != 1 || len(all) != 2 {
		t.Errorf("visibility partition mismatch: public=%d deprecated=%d all=%d", len(publicOnly), len(deprecatedOnly), len(all))
	}
}

func containsDeprecatedMarker(line string) bool {
	return len(line) > 0 && (indexOfDeprecated(line) != -1)
}

func indexOfDeprecated(s string) int {
	const marker = "@deprecated"
	for i := 0; i+len(marker) <= len(s); i++ {
		if s[i:i+len(marker)] == marker {
			return i
		}
	}
	return -1
}

func TestReleasePlugin_ExcludedNamesIncludesContextDirAndLegacyFiles(t *testing.T) {
	if _, ok := excludedNames["PLUGIN_AI_CONTEXT.md"]; !ok {
		t.Error("expected PLUGIN_AI_CONTEXT.md in the exclusion set")
	}
	if _, ok := excludedNames[".indevelopment"]; !ok {
		t.Error("expected .indevelopment in the exclusion set")
	}
	if _, ok := excludedNames[".buildignore"]; !ok {
		t.Error("expected .buildignore in the exclusion set")
	}
	if len(excludedNames) < 13 { // 11 PluginContextFiles-derived + .build82 dir + .buildignore, at minimum
		t.Errorf("expected at least 13 excluded names, got %d", len(excludedNames))
	}
}

// TestCreateZip_ExcludesBuildignoreItself confirms .buildignore is excluded from the archive:
// .buildignore is build82's own config file, not part of the plugin, so it is never shipped
// inside the release ZIP, without needing to list it inside .buildignore itself. Opens the actual produced archive and asserts no entry is
// .buildignore.
func TestCreateZip_ExcludesBuildignoreItself(t *testing.T) {
	pluginPath := t.TempDir()
	mustWriteFile(t, filepath.Join(pluginPath, "lib.php"), "<?php\n")
	mustWriteFile(t, filepath.Join(pluginPath, ".buildignore"), "# comment\n")

	dest := filepath.Join(t.TempDir(), "out.zip")
	if _, err := createZip(pluginPath, dest, "myplugin", excludedNames); err != nil {
		t.Fatalf("createZip: %v", err)
	}

	r, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatalf("open produced zip: %v", err)
	}
	defer r.Close()

	sawLib := false
	for _, f := range r.File {
		if strings.Contains(f.Name, ".buildignore") {
			t.Errorf("expected .buildignore to be excluded from the archive, found entry %q", f.Name)
		}
		if strings.HasSuffix(f.Name, "lib.php") {
			sawLib = true
		}
	}
	if !sawLib {
		t.Error("expected lib.php to still be included in the archive")
	}
}

// writeFailCloser wraps a real *os.File so its Close() still actually releases the file
// descriptor (no fd leak), but its Write() always fails — simulating a destination that goes bad
// partway through (e.g. a full disk or another I/O error), specifically so the failure surfaces
// while zip.Writer.Close() is flushing the central directory rather than during an earlier
// zw.Create/io.Copy call.
type writeFailCloser struct {
	*os.File
}

func (w writeFailCloser) Write([]byte) (int, error) {
	return 0, errors.New("simulated write failure")
}

// TestCreateZip_PropagatesCloseError verifies that createZip returns the error from closing the
// zip writer or destination file, and leaves no partial archive behind. If zip.Writer.Close()
// fails while flushing the ZIP's central directory (e.g. a full disk or another I/O error), the
// error must reach the caller. The plugin fixture is empty on purpose: with no files to walk, the
// only Write() call the destination sees is the one zip.Writer.Close() makes to write the (empty)
// central directory and end-of-central-directory record, so a Write failure here can only be
// observed via the Close() error path, never via the walk's own error path.
func TestCreateZip_PropagatesCloseError(t *testing.T) {
	pluginPath := t.TempDir() // deliberately empty: forces the only Write() to happen inside zw.Close()
	dest := filepath.Join(t.TempDir(), "out.zip")

	orig := newZipDestination
	t.Cleanup(func() { newZipDestination = orig })
	newZipDestination = func(path string) (io.WriteCloser, error) {
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		return writeFailCloser{File: f}, nil
	}

	if _, err := createZip(pluginPath, dest, "myplugin", excludedNames); err == nil {
		t.Fatal("expected createZip to return an error when the ZIP writer's Close() fails, got nil")
	}
}

func TestMergeBuildIgnore_AdditiveNotReplacing(t *testing.T) {
	pluginPath := t.TempDir()
	mustWriteFile(t, filepath.Join(pluginPath, ".buildignore"), "# comment\ndocs\n\ntests\n")

	merged := mergeBuildIgnore(pluginPath, excludedNames)
	if _, ok := merged["docs"]; !ok {
		t.Error("expected 'docs' from .buildignore to be merged in")
	}
	if _, ok := merged["tests"]; !ok {
		t.Error("expected 'tests' from .buildignore to be merged in")
	}
	if _, ok := merged[generators.ContextDir]; !ok {
		t.Error("expected the fixed exclusion set to still be present (additive, not replacing)")
	}
}

func validReleaseInfo() extractors.PluginInfo {
	return extractors.PluginInfo{
		Component: "local_test",
		Version:   "2024010100",
		Requires:  "2023100900",
		Maturity:  "MATURITY_STABLE",
	}
}

// writeValidReleaseFixture creates everything validateForRelease requires when nothing is being
// deliberately left out — a valid lang file and a Privacy API provider — so tests targeting one
// specific missing requirement don't also trip over unrelated ones.
func writeValidReleaseFixture(t *testing.T, pluginPath string) {
	t.Helper()
	mustMkdirAll(t, filepath.Join(pluginPath, "lang", "en"))
	mustWriteFile(t, filepath.Join(pluginPath, "lang", "en", "local_test.php"), `<?php $string['pluginname'] = 'Test';`)
	mustMkdirAll(t, filepath.Join(pluginPath, "classes", "privacy"))
	mustWriteFile(t, filepath.Join(pluginPath, "classes", "privacy", "provider.php"), `<?php
namespace local_test\privacy;
class provider implements \core_privacy\local\metadata\null_provider {
    public static function get_reason(): string {
        return 'privacy:metadata';
    }
}
`)
}

func TestValidateForRelease_ValidPluginHasNoIssues(t *testing.T) {
	pluginPath := t.TempDir()
	writeValidReleaseFixture(t, pluginPath)

	if issues := validateForRelease(pluginPath, "local_test", validReleaseInfo()); len(issues) != 0 {
		t.Errorf("expected no issues, got %+v", issues)
	}
}

func TestValidateForRelease_ComponentMismatch(t *testing.T) {
	pluginPath := t.TempDir()
	info := validReleaseInfo()
	info.Component = "local_other"

	issues := validateForRelease(pluginPath, "local_test", info)
	if !anyContains(issues, "local_other") || !anyContains(issues, "local_test") {
		t.Errorf("expected a component-mismatch issue naming both components, got %+v", issues)
	}
}

func TestValidateForRelease_MissingRequiresAndMaturity(t *testing.T) {
	pluginPath := t.TempDir()
	info := validReleaseInfo()
	info.Requires = ""
	info.Maturity = ""

	issues := validateForRelease(pluginPath, "local_test", info)
	if !anyContains(issues, "requires") || !anyContains(issues, "maturity") {
		t.Errorf("expected requires and maturity issues, got %+v", issues)
	}
}

func TestValidateForRelease_UnrecognizedMaturity(t *testing.T) {
	pluginPath := t.TempDir()
	info := validReleaseInfo()
	info.Maturity = "MATURITY_MADE_UP"

	issues := validateForRelease(pluginPath, "local_test", info)
	if !anyContains(issues, "MATURITY_MADE_UP") {
		t.Errorf("expected an unrecognized-maturity issue, got %+v", issues)
	}
}

func TestValidateForRelease_MissingLangFile(t *testing.T) {
	pluginPath := t.TempDir()
	issues := validateForRelease(pluginPath, "local_test", validReleaseInfo())
	if !anyContains(issues, "lang/en/local_test.php") {
		t.Errorf("expected a missing-language-file issue, got %+v", issues)
	}
}

func TestValidateForRelease_GitDirectoryPresent(t *testing.T) {
	pluginPath := t.TempDir()
	writeValidReleaseFixture(t, pluginPath)
	mustMkdirAll(t, filepath.Join(pluginPath, ".git"))

	issues := validateForRelease(pluginPath, "local_test", validReleaseInfo())
	if !anyContains(issues, ".git") {
		t.Errorf("expected a .git-directory issue, got %+v", issues)
	}
}

// TestValidateForRelease_MissingPrivacyProvider covers the new Privacy API check: every plugin
// must declare classes/privacy/provider.php since Moodle 3.9, even one implementing null_provider
// because it stores no personal data at all.
func TestValidateForRelease_MissingPrivacyProvider(t *testing.T) {
	pluginPath := t.TempDir()
	mustMkdirAll(t, filepath.Join(pluginPath, "lang", "en"))
	mustWriteFile(t, filepath.Join(pluginPath, "lang", "en", "local_test.php"), `<?php $string['pluginname'] = 'Test';`)

	issues := validateForRelease(pluginPath, "local_test", validReleaseInfo())
	if !anyContains(issues, "classes/privacy/provider.php") {
		t.Errorf("expected a missing-privacy-provider issue, got %+v", issues)
	}
}

// TestValidateForRelease_ThirdpartyLibsWithoutDeclaration covers the new thirdpartylibs.xml check:
// required whenever a plugin bundles a thirdparty/ directory.
func TestValidateForRelease_ThirdpartyLibsWithoutDeclaration(t *testing.T) {
	pluginPath := t.TempDir()
	writeValidReleaseFixture(t, pluginPath)
	mustMkdirAll(t, filepath.Join(pluginPath, "thirdparty"))

	issues := validateForRelease(pluginPath, "local_test", validReleaseInfo())
	if !anyContains(issues, "thirdpartylibs.xml") {
		t.Errorf("expected a missing-thirdpartylibs.xml issue, got %+v", issues)
	}
}

// TestValidateForRelease_ThirdpartyLibsDeclared confirms declaring thirdpartylibs.xml alongside a
// thirdparty/ directory satisfies the check.
func TestValidateForRelease_ThirdpartyLibsDeclared(t *testing.T) {
	pluginPath := t.TempDir()
	writeValidReleaseFixture(t, pluginPath)
	mustMkdirAll(t, filepath.Join(pluginPath, "thirdparty"))
	mustWriteFile(t, filepath.Join(pluginPath, "thirdpartylibs.xml"), `<?xml version="1.0"?><libraries></libraries>`)

	if issues := validateForRelease(pluginPath, "local_test", validReleaseInfo()); len(issues) != 0 {
		t.Errorf("expected no issues, got %+v", issues)
	}
}

func anyContains(issues []string, substr string) bool {
	for _, issue := range issues {
		if strings.Contains(issue, substr) {
			return true
		}
	}
	return false
}

// TestHandleGenerateContext_LogsAiIndexFailureToStderr confirms GenerateAiIndex's result is not
// discarded: a failure writing MOODLE_AI_INDEX.md must be logged, not left invisible to both the
// caller's report and any log.
func TestHandleGenerateContext_LogsAiIndexFailureToStderr(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)

	pluginPath := filepath.Join(moodlePath, "local", "demo")
	mustMkdirAll(t, pluginPath)
	mustWriteFile(t, filepath.Join(pluginPath, "version.php"), "<?php\n$plugin->component = 'local_demo';\n$plugin->version = 2024010100;\n")

	// Force MOODLE_AI_INDEX.md's write to fail: pre-create the *global* .build82/ directory
	// read-only. This is a separate directory from the plugin's own pluginPath/.build82/, so
	// generate_plugin_context's own plugin-scoped work still succeeds — isolating the failure to
	// just the AI index update.
	globalContextDir := filepath.Join(moodlePath, generators.ContextDir)
	mustMkdirAll(t, globalContextDir)
	if err := os.Chmod(globalContextDir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(globalContextDir, 0o755) }) // let t.TempDir() clean up afterward

	var res *mcp.CallToolResult
	stderr := captureStderr(t, func() {
		var handlerErr error
		res, _, handlerErr = handleGenerateContext(context.Background(), nil, GenerateContextInput{PluginPath: "local/demo"})
		if handlerErr != nil {
			t.Fatalf("unexpected error: %v", handlerErr)
		}
	})

	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res.Content)
	}
	if !strings.Contains(stderr, "MOODLE_AI_INDEX.md") {
		t.Errorf("expected the AI index write failure logged to stderr, got:\n%s", stderr)
	}
}

// TestRequireConfig_HomeDirUnresolvableSurfacesAsToolError verifies that a failure to resolve the
// user's home directory is returned by requireConfig as a real error, and that tools guarding on
// requireConfig (handleGenerateContext stands in for all of them, since they share the same
// two-step guard) render it as an explicit IsError tool result instead of falling through to
// NotInitialized or a wrong config path.
func TestRequireConfig_HomeDirUnresolvableSurfacesAsToolError(t *testing.T) {
	t.Setenv("BUILD82_MOODLE_PATH", "")
	t.Setenv("BUILD82_MOODLE_VERSION", "")
	t.Setenv("BUILD82_MOODLE_FULLVERSION", "")
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	res, _, err := handleGenerateContext(context.Background(), nil, GenerateContextInput{PluginPath: "local/demo"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected an IsError result when the home directory cannot be resolved, got %+v", res)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if strings.Contains(text, "not initialized") {
		t.Errorf("expected a config-resolution error, not the generic NotInitialized message, got: %s", text)
	}
}

// TestHandleUpdateIndexes_LogsConfigSaveFailureToStderr confirms a failed config.Save when
// resyncing a changed Moodle version is logged to stderr; ~/.build82 then keeps reporting the old
// version.
func TestHandleUpdateIndexes_LogsConfigSaveFailureToStderr(t *testing.T) {
	freshCache(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows equivalent, harmless on other OSes

	moodlePath := t.TempDir()
	mustWriteFile(t, filepath.Join(moodlePath, "version.php"),
		"<?php\n$version = 2024100700.00;\n$release = '4.5';\n$branch = '405';\n")

	// BUILD82_MOODLE_PATH takes priority over the file-based config (config.Load's own documented
	// precedence), so this is enough to make requireConfig() succeed without ever writing to
	// ~/.build82 first — but it means the stored MoodleVersion/MoodleFullVersion are empty,
	// guaranteeing the version-resync branch triggers (real version.php content != "").
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)

	// config.Save writes to ~/.build82 — making HOME read-only forces the save to fail with
	// permission denied (fsutil.WriteAtomic creates its temp file inside the target's own
	// directory, which is HOME itself here).
	if err := os.Chmod(home, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(home, 0o755) }) // let t.TempDir() clean up afterward

	stderr := captureStderr(t, func() {
		res, _, err := handleUpdateIndexes(context.Background(), nil, UpdateIndexesInput{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error result: %+v", res.Content)
		}
	})

	if !strings.Contains(stderr, "failed to persist updated Moodle version") {
		t.Errorf("expected the config.Save failure logged to stderr, got:\n%s", stderr)
	}
}

// TestToolInputs_UseSnakeCaseJsonTags confirms ReleasePluginInput.OutputDir and
// CreatePluginSkeletonInput.DisplayName serialize in snake_case, like every other tool input field
// (moodle_path, plugin_path, mark_as_dev, ...). Marshals each struct and asserts on the actual
// wire-format JSON keys, not just the source-level json tag, since that's what an MCP client
// actually sees.
func TestToolInputs_UseSnakeCaseJsonTags(t *testing.T) {
	releaseJSON, err := json.Marshal(ReleasePluginInput{OutputDir: "x"})
	if err != nil {
		t.Fatalf("marshal ReleasePluginInput: %v", err)
	}
	if !strings.Contains(string(releaseJSON), `"output_dir"`) {
		t.Errorf("expected ReleasePluginInput to serialize OutputDir as \"output_dir\", got: %s", releaseJSON)
	}
	if strings.Contains(string(releaseJSON), `"outputDir"`) {
		t.Errorf("ReleasePluginInput still serializes the old camelCase \"outputDir\" key: %s", releaseJSON)
	}

	scaffoldJSON, err := json.Marshal(CreatePluginSkeletonInput{DisplayName: "x"})
	if err != nil {
		t.Fatalf("marshal CreatePluginSkeletonInput: %v", err)
	}
	if !strings.Contains(string(scaffoldJSON), `"display_name"`) {
		t.Errorf("expected CreatePluginSkeletonInput to serialize DisplayName as \"display_name\", got: %s", scaffoldJSON)
	}
	if strings.Contains(string(scaffoldJSON), `"displayName"`) {
		t.Errorf("CreatePluginSkeletonInput still serializes the old camelCase \"displayName\" key: %s", scaffoldJSON)
	}
}

// TestFuzzySearchInFile_MissingFileReturnsNil confirms fuzzySearchInFile's missing-file behavior:
// a leftover os.Stat call was removed from it (dead code — its result was immediately discarded
// via `_ = info`), leaving os.ReadFile's own error as the sole existence check. Asserts the
// missing-file behavior is unchanged by that simplification.
func TestFuzzySearchInFile_MissingFileReturnsNil(t *testing.T) {
	if got := fuzzySearchInFile(filepath.Join(t.TempDir(), "does-not-exist.md"), "anything"); got != nil {
		t.Errorf("expected nil for a nonexistent file, got %v", got)
	}
}

// TestFuzzySearchInFile_SharesMtimeCacheWithSearchInFile confirms fuzzySearchInFile caches by
// mtime like searchInFile: both use the same cachedLines helper and the same underlying cache
// map. searchInFile first populates the
// cache for path; the file is then overwritten with different content but its mtime pinned back
// to the pre-overwrite value via os.Chtimes — a cache keyed correctly on mtime must still serve
// the stale-but-cached lines. Only reading through the shared cache (not straight from disk on
// every call) can produce that result.
func TestFuzzySearchInFile_SharesMtimeCacheWithSearchInFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.md")
	mustWriteFile(t, path, "widget_helper_function\n")

	if got := searchInFile(path, "widget_helper_function"); len(got) != 1 {
		t.Fatalf("searchInFile: expected 1 exact match to populate the cache, got %v", got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	original := info.ModTime()

	mustWriteFile(t, path, "completely_different_line\n")
	if err := os.Chtimes(path, original, original); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	got := fuzzySearchInFile(path, "widget_helper_functoin") // deliberate typo, exercises the fuzzy path
	if len(got) != 1 || !strings.Contains(got[0], "widget_helper_function") {
		t.Errorf("expected fuzzySearchInFile to serve the cached (pre-overwrite) content via the shared cache, got %v", got)
	}
}

// TestHandleReleasePlugin_RejectsIdentifierEscapingMoodleRoot confirms that, since release_plugin
// accepts relative and absolute paths as well as components, an identifier that
// resolves outside the Moodle root — even to a real plugin directory with a version.php — is
// rejected by the containment check and never packaged.
func TestHandleReleasePlugin_RejectsIdentifierEscapingMoodleRoot(t *testing.T) {
	freshCache(t)
	base := t.TempDir()
	moodlePath := filepath.Join(base, "moodle")
	mustMkdirAll(t, filepath.Join(moodlePath, "local"))
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)

	outside := filepath.Join(base, "evil")
	mustMkdirAll(t, outside)
	mustWriteFile(t, filepath.Join(outside, "version.php"),
		"<?php\n$plugin->component = 'local_evil';\n$plugin->version = 2024010100;\n")

	outDir := t.TempDir()
	for _, component := range []string{"local/../../evil", "_..", outside} {
		res, _, err := handleReleasePlugin(context.Background(), nil, ReleasePluginInput{Component: component, OutputDir: outDir})
		if err != nil {
			t.Fatalf("component %q: unexpected error: %v", component, err)
		}
		if res == nil || !res.IsError {
			t.Fatalf("component %q: expected an IsError result, got: %+v", component, res)
		}
		if text := res.Content[0].(*mcp.TextContent).Text; !strings.Contains(text, "Invalid plugin path") {
			t.Errorf("component %q: expected the containment rejection, got: %s", component, text)
		}
	}
	if entries, _ := os.ReadDir(outDir); len(entries) != 0 {
		t.Errorf("no archive may be written for a rejected identifier, found %d entries", len(entries))
	}
}

// TestHandleReleasePlugin_ZipNameUsesExtractedComponentNotRaw confirms the
// release ZIP's filename is built from info.Component (extracted and validated from the
// plugin's own version.php by resolveAndValidatePlugin), not from the raw in.Component the MCP
// client supplied. Sets up a plugin directory whose version.php declares a *different* component
// than the identifier used to resolve it, so the two can't agree by coincidence: the ZIP name
// must start with "local_other_", not "local_test_".
func TestHandleReleasePlugin_ZipNameUsesExtractedComponentNotRaw(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)

	pluginPath := filepath.Join(moodlePath, "local_test")
	mustMkdirAll(t, pluginPath)
	mustWriteFile(t, filepath.Join(pluginPath, "version.php"),
		"<?php\n$plugin->component = 'local_other';\n$plugin->version = 2024010100;\n")

	outDir := t.TempDir()
	res, _, err := handleReleasePlugin(context.Background(), nil, ReleasePluginInput{
		Component: "local_test", // the identifier used to locate the plugin directory
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res.Content)
	}

	wantZip := filepath.Join(outDir, "local_other_2024010100.zip")
	if _, statErr := os.Stat(wantZip); statErr != nil {
		t.Errorf("expected the ZIP to be named after the extracted component (local_other), not the raw identifier (local_test): %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(outDir, "local_test_2024010100.zip")); statErr == nil {
		t.Error("ZIP must not be named after the raw client-supplied component")
	}
}

// TestCreateZip_ExcludesGitDirectoryEvenWithoutStrict confirms a plugin's
// .git directory (common during development) never ends up inside the release ZIP, regardless of
// strict mode.
func TestCreateZip_ExcludesGitDirectoryEvenWithoutStrict(t *testing.T) {
	pluginPath := t.TempDir()
	mustWriteFile(t, filepath.Join(pluginPath, "lib.php"), "<?php\n")
	mustMkdirAll(t, filepath.Join(pluginPath, ".git", "objects"))
	mustWriteFile(t, filepath.Join(pluginPath, ".git", "config"), "[core]\n")
	mustWriteFile(t, filepath.Join(pluginPath, ".git", "objects", "deadbeef"), "not really a git object")

	dest := filepath.Join(t.TempDir(), "out.zip")
	if _, err := createZip(pluginPath, dest, "myplugin", excludedNames); err != nil {
		t.Fatalf("createZip: %v", err)
	}

	r, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatalf("open produced zip: %v", err)
	}
	defer r.Close()

	sawLib := false
	for _, f := range r.File {
		if strings.Contains(filepath.ToSlash(f.Name), ".git/") {
			t.Errorf("expected no file under .git/ in the archive, found entry %q", f.Name)
		}
		if strings.HasSuffix(f.Name, "lib.php") {
			sawLib = true
		}
	}
	if !sawLib {
		t.Error("expected lib.php to still be included in the archive")
	}
}

// TestBuildExcludedNames_IncludesGitUnconditionally confirms .git is part of the fixed exclusion
// set itself, not just something createZip happens to skip in the fixture above.
func TestBuildExcludedNames_IncludesGitUnconditionally(t *testing.T) {
	if _, ok := excludedNames[".git"]; !ok {
		t.Error("expected .git in the unconditional exclusion set")
	}
}

func TestMergeBuildIgnore_MissingFileIsNotAnError(t *testing.T) {
	pluginPath := t.TempDir()
	merged := mergeBuildIgnore(pluginPath, excludedNames)
	if len(merged) != len(excludedNames) {
		t.Errorf("expected merged set to equal the base set when .buildignore is absent, got %d vs %d", len(merged), len(excludedNames))
	}
}
