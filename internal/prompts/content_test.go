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

package prompts

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file tests the rendered content of successful scaffold_plugin, review_plugin and
// debug_plugin calls, so that a swapped field or dropped section is detected.

// setupPromptPlugin creates a temporary Moodle root containing local/demo with a version.php and a
// generated PLUGIN_AI_CONTEXT.md, points BUILD82_MOODLE_PATH at it, and returns the root and the
// plugin's absolute path.
func setupPromptPlugin(t *testing.T) (root, pluginPath string) {
	t.Helper()
	root = t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", root)

	pluginPath = filepath.Join(root, "local", "demo")
	mustMkdirAll(t, pluginPath)
	mustWriteFile(t, filepath.Join(pluginPath, "version.php"),
		"<?php\n$plugin->component = 'local_demo';\n$plugin->version = 2024010100;\n")
	mustMkdirAll(t, filepath.Join(pluginPath, ".build82"))
	mustWriteFile(t, filepath.Join(pluginPath, ".build82", "PLUGIN_AI_CONTEXT.md"), "PLUGIN AI CONTEXT BODY")
	return root, pluginPath
}

// textContentOf returns the text of a prompt message, failing the test if it is not text content.
func textContentOf(t *testing.T, msg *mcp.PromptMessage) string {
	t.Helper()
	tc, ok := msg.Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", msg.Content)
	}
	return tc.Text
}

// TestHandleReviewPrompt_ContentReflectsPluginAndFocus verifies review_plugin output reflects the plugin metadata, the chosen focus and the plugin context.
func TestHandleReviewPrompt_ContentReflectsPluginAndFocus(t *testing.T) {
	_, pluginPath := setupPromptPlugin(t)

	result, err := handleReviewPrompt(context.Background(), promptReq(map[string]string{
		"plugin": pluginPath, "focus": "security",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Messages) != 3 {
		t.Fatalf("expected 3 messages (few-shot user/assistant + real request), got %d", len(result.Messages))
	}
	text := textContentOf(t, result.Messages[2])
	if !strings.Contains(text, "local_demo") {
		t.Errorf("expected component name in review prompt, got:\n%s", text)
	}
	if !strings.Contains(text, "Focus | security") {
		t.Errorf("expected focus reflected, got:\n%s", text)
	}
	if !strings.Contains(text, "Authentication & Authorization") {
		t.Errorf("expected the security focus criteria section, got:\n%s", text)
	}
	if strings.Contains(text, "## Performance") {
		t.Errorf("expected the performance section to be excluded for focus=security, got:\n%s", text)
	}
	if !strings.Contains(text, "PLUGIN AI CONTEXT BODY") {
		t.Errorf("expected the plugin's generated AI context to be embedded, got:\n%s", text)
	}
}

// TestHandleReviewPrompt_DefaultFocusIncludesAllSections verifies review_plugin without a focus includes every criteria section.
func TestHandleReviewPrompt_DefaultFocusIncludesAllSections(t *testing.T) {
	_, pluginPath := setupPromptPlugin(t)

	result, err := handleReviewPrompt(context.Background(), promptReq(map[string]string{"plugin": pluginPath}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := textContentOf(t, result.Messages[2])
	for _, section := range []string{"## Security", "## Performance", "## Standards", "## Database", "## API Usage"} {
		if !strings.Contains(text, section) {
			t.Errorf("expected default focus 'all' to include %q, got:\n%s", section, text)
		}
	}
}

// TestHandleDebugPrompt_KeywordMatchedHints verifies debug_plugin includes only the hints matching keywords in the error text.
func TestHandleDebugPrompt_KeywordMatchedHints(t *testing.T) {
	_, pluginPath := setupPromptPlugin(t)

	result, err := handleDebugPrompt(context.Background(), promptReq(map[string]string{
		"plugin": pluginPath, "error": "Sorry, but you do not currently have permissions to do that (capability check failed)",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := textContentOf(t, result.Messages[2])
	if !strings.Contains(text, "db/access.php") {
		t.Errorf("expected the capability-specific hint, got:\n%s", text)
	}
	if strings.Contains(text, "scheduled_task.php --execute") {
		t.Errorf("expected the unrelated task-specific hint to be excluded, got:\n%s", text)
	}
}

// TestHandleDebugPrompt_GenericHintsWhenNoKeywordMatches verifies debug_plugin falls back to the generic hints when no keyword matches.
func TestHandleDebugPrompt_GenericHintsWhenNoKeywordMatches(t *testing.T) {
	_, pluginPath := setupPromptPlugin(t)

	result, err := handleDebugPrompt(context.Background(), promptReq(map[string]string{
		"plugin": pluginPath, "error": "something completely unrelated happened",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := textContentOf(t, result.Messages[2])
	if !strings.Contains(text, "Enable DEVELOPER-level debugging") {
		t.Errorf("expected the generic fallback hints, got:\n%s", text)
	}
}

// TestHandleDebugPrompt_LongArgsAreTruncated verifies that oversized "error" and "context"
// arguments are cut at maxPromptArgLen and marked with "...(truncated)".
func TestHandleDebugPrompt_LongArgsAreTruncated(t *testing.T) {
	_, pluginPath := setupPromptPlugin(t)

	longError := strings.Repeat("e", maxPromptArgLen+500)
	longContext := strings.Repeat("c", maxPromptArgLen+500)

	result, err := handleDebugPrompt(context.Background(), promptReq(map[string]string{
		"plugin": pluginPath, "error": longError, "context": longContext,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := textContentOf(t, result.Messages[2])

	if strings.Contains(text, strings.Repeat("e", maxPromptArgLen+1)) {
		t.Error("expected the oversized 'error' argument to be truncated, but the full value appears in the rendered prompt")
	}
	if strings.Contains(text, strings.Repeat("c", maxPromptArgLen+1)) {
		t.Error("expected the oversized 'context' argument to be truncated, but the full value appears in the rendered prompt")
	}
	if strings.Count(text, "...(truncated)") != 2 {
		t.Errorf("expected exactly 2 truncation markers (error + context), got %d in:\n%s", strings.Count(text, "...(truncated)"), text)
	}
}

// TestHandleDebugPrompt_NormalArgsAreUnchanged verifies that arguments under the limit appear
// unchanged and unmarked.
func TestHandleDebugPrompt_NormalArgsAreUnchanged(t *testing.T) {
	_, pluginPath := setupPromptPlugin(t)

	result, err := handleDebugPrompt(context.Background(), promptReq(map[string]string{
		"plugin": pluginPath, "error": "a normal error message", "context": "happens on save",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := textContentOf(t, result.Messages[2])
	if !strings.Contains(text, "a normal error message") {
		t.Errorf("expected the normal error message to appear unchanged, got:\n%s", text)
	}
	if !strings.Contains(text, "happens on save") {
		t.Errorf("expected the normal context to appear unchanged, got:\n%s", text)
	}
	if strings.Contains(text, "...(truncated)") {
		t.Errorf("expected no truncation marker for normal-sized args, got:\n%s", text)
	}
}

// TestHandleReviewPrompt_LongFilesArgIsTruncated verifies that an oversized "files" argument is
// truncated and marked.
func TestHandleReviewPrompt_LongFilesArgIsTruncated(t *testing.T) {
	_, pluginPath := setupPromptPlugin(t)

	longFiles := strings.Repeat("f", maxPromptArgLen+500)
	result, err := handleReviewPrompt(context.Background(), promptReq(map[string]string{
		"plugin": pluginPath, "files": longFiles,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := textContentOf(t, result.Messages[2])
	if strings.Contains(text, strings.Repeat("f", maxPromptArgLen+1)) {
		t.Error("expected the oversized 'files' argument to be truncated, but the full value appears in the rendered prompt")
	}
	if !strings.Contains(text, "...(truncated)") {
		t.Errorf("expected a truncation marker in the rendered prompt, got:\n%s", text)
	}
}

// TestHandleReviewPrompt_NormalFilesArgIsUnchanged verifies that a short "files" list passes
// through unaltered.
func TestHandleReviewPrompt_NormalFilesArgIsUnchanged(t *testing.T) {
	_, pluginPath := setupPromptPlugin(t)

	result, err := handleReviewPrompt(context.Background(), promptReq(map[string]string{
		"plugin": pluginPath, "files": "lib.php, classes/observer.php",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := textContentOf(t, result.Messages[2])
	if !strings.Contains(text, "lib.php, classes/observer.php") {
		t.Errorf("expected the normal 'files' value to appear unchanged, got:\n%s", text)
	}
	if strings.Contains(text, "...(truncated)") {
		t.Errorf("expected no truncation marker for a normal-sized 'files' value, got:\n%s", text)
	}
}

// TestHandleScaffoldPrompt_LongArgsAreTruncated verifies that oversized "description" and
// "features" arguments are truncated; only the description is echoed into the output.
func TestHandleScaffoldPrompt_LongArgsAreTruncated(t *testing.T) {
	longDescription := strings.Repeat("d", maxPromptArgLen+500)
	longFeatures := "database," + strings.Repeat("x", maxPromptArgLen+500)

	result, err := handleScaffoldPrompt(context.Background(), promptReq(map[string]string{
		"type": "local", "name": "demo", "description": longDescription, "features": longFeatures,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := textContentOf(t, result.Messages[2])
	if strings.Contains(text, strings.Repeat("d", maxPromptArgLen+1)) {
		t.Error("expected the oversized 'description' argument to be truncated, but the full value appears in the rendered prompt")
	}
	if strings.Contains(text, strings.Repeat("x", maxPromptArgLen+1)) {
		t.Error("expected the oversized 'features' argument to be truncated, but the full value appears in the rendered prompt")
	}
	if strings.Count(text, "...(truncated)") != 1 {
		t.Errorf("expected exactly 1 truncation marker (description; features isn't echoed verbatim into the output, only parsed), got %d in:\n%s", strings.Count(text, "...(truncated)"), text)
	}
}

// TestHandleScaffoldPrompt_NormalArgsAreUnchanged verifies that short description and features
// values pass through unaltered.
func TestHandleScaffoldPrompt_NormalArgsAreUnchanged(t *testing.T) {
	result, err := handleScaffoldPrompt(context.Background(), promptReq(map[string]string{
		"type": "local", "name": "demo", "description": "A small demo plugin", "features": "database, events",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := textContentOf(t, result.Messages[2])
	if !strings.Contains(text, "A small demo plugin") {
		t.Errorf("expected the normal description to appear unchanged, got:\n%s", text)
	}
	if strings.Contains(text, "...(truncated)") {
		t.Errorf("expected no truncation marker for normal-sized args, got:\n%s", text)
	}
}

// TestHandleScaffoldPrompt_ContentReflectsTypeAndFeatures verifies scaffold_plugin output reflects the plugin type and requested features.
func TestHandleScaffoldPrompt_ContentReflectsTypeAndFeatures(t *testing.T) {
	result, err := handleScaffoldPrompt(context.Background(), promptReq(map[string]string{
		"type": "mod", "name": "widget", "description": "A widget activity", "features": "database, scheduled tasks",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := textContentOf(t, result.Messages[2])
	if !strings.Contains(text, "mod_widget") {
		t.Errorf("expected the component name, got:\n%s", text)
	}
	if !strings.Contains(text, "mod/widget") {
		t.Errorf("expected the directory, got:\n%s", text)
	}
	if !strings.Contains(text, "`{component}_add_instance`") {
		t.Errorf("expected the mod-specific type note, got:\n%s", text)
	}
	if !strings.Contains(text, "db/install.xml") {
		t.Errorf("expected db/install.xml listed for the database feature, got:\n%s", text)
	}
	if !strings.Contains(text, "db/tasks.php") {
		t.Errorf("expected db/tasks.php listed for the scheduled-tasks feature, got:\n%s", text)
	}
}
