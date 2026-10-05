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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func TestCreatePluginSkeleton_WritesExpectedFiles(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)
	t.Setenv("BUILD82_MOODLE_FULLVERSION", "2024010100")

	res, _, err := handleCreatePluginSkeleton(context.Background(), nil, CreatePluginSkeletonInput{
		Type: "local", Name: "myplugin", DisplayName: "My Plugin",
		Features: "database,tasks,services,events,capabilities,settings",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res.Content)
	}

	pluginPath := filepath.Join(moodlePath, "local", "myplugin")
	expected := []string{
		"version.php", "lang/en/local_myplugin.php", "lib.php",
		"db/install.xml", "db/tasks.php", "db/services.php", "db/events.php",
		"classes/observer.php", "db/access.php", "settings.php",
	}
	for _, f := range expected {
		if _, err := os.Stat(filepath.Join(pluginPath, f)); err != nil {
			t.Errorf("expected %s to exist: %v", f, err)
		}
	}

	version := mustReadFile(t, filepath.Join(pluginPath, "version.php"))
	for _, want := range []string{"$plugin->component = 'local_myplugin';", "$plugin->requires  = 2024010100;", "$plugin->maturity  = MATURITY_ALPHA;"} {
		if !strings.Contains(version, want) {
			t.Errorf("version.php missing %q, got:\n%s", want, version)
		}
	}

	lang := mustReadFile(t, filepath.Join(pluginPath, "lang", "en", "local_myplugin.php"))
	if !strings.Contains(lang, "$string['pluginname'] = 'My Plugin';") {
		t.Errorf("lang file missing pluginname string, got:\n%s", lang)
	}
}

func TestCreatePluginSkeleton_RefusesIfDirectoryExists(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)
	mustMkdirAll(t, filepath.Join(moodlePath, "local", "existing"))

	res, _, err := handleCreatePluginSkeleton(context.Background(), nil, CreatePluginSkeletonInput{
		Type: "local", Name: "existing",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected an error result when the plugin directory already exists")
	}
}

func TestCreatePluginSkeleton_RefusesInvalidName(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)

	for _, name := range []string{"../../etc", "MyPlugin", "1plugin", "my-plugin", ""} {
		res, _, err := handleCreatePluginSkeleton(context.Background(), nil, CreatePluginSkeletonInput{
			Type: "local", Name: name,
		})
		if err != nil {
			t.Fatalf("unexpected error for name %q: %v", name, err)
		}
		if !res.IsError {
			t.Errorf("expected an error result for invalid name %q", name)
		}
	}
	if _, err := os.Stat(filepath.Join(moodlePath, "local")); err == nil {
		t.Error("no directory should have been created under an invalid name")
	}
}

func TestCreatePluginSkeleton_RefusesUnknownType(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)

	res, _, err := handleCreatePluginSkeleton(context.Background(), nil, CreatePluginSkeletonInput{
		Type: "notarealtype", Name: "myplugin",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected an error result for an unknown plugin type")
	}
}

func TestCreatePluginSkeleton_RefusesInvalidMaturity(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)

	res, _, err := handleCreatePluginSkeleton(context.Background(), nil, CreatePluginSkeletonInput{
		Type: "local", Name: "myplugin", Maturity: "MATURITY_MADE_UP",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected an error result for an invalid maturity constant")
	}
}

// TestCreatePluginSkeleton_EscapesDisplayNameForPhp confirms displayName is properly escaped
// before being embedded in a single-quoted PHP string literal. Without escaping, this exact
// payload breaks out of the string and injects eval($_GET['c']) as a second, fully-executed PHP
// statement into the generated lang file.
func TestCreatePluginSkeleton_EscapesDisplayNameForPhp(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)

	malicious := `x'; eval($_GET['c']); //`
	res, _, err := handleCreatePluginSkeleton(context.Background(), nil, CreatePluginSkeletonInput{
		Type: "local", Name: "evil", DisplayName: malicious,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res.Content)
	}

	lang := mustReadFile(t, filepath.Join(moodlePath, "local", "evil", "lang", "en", "local_evil.php"))
	want := `$string['pluginname'] = 'x\'; eval($_GET[\'c\']); //';`
	if !strings.Contains(lang, want) {
		t.Errorf("displayName was not escaped as expected.\ngot:\n%s\nwant substring:\n%s", lang, want)
	}
	// A successful breakout would produce a second top-level statement after the pluginname
	// assignment (e.g. a bare "eval(...)" call outside any string) — with escaping applied, the
	// whole payload stays inside the one string literal, so there is exactly one statement.
	if n := strings.Count(lang, "\n$string["); n != 1 {
		t.Fatalf("expected exactly one $string[...] assignment (payload must not break out of the string literal), found %d:\n%s", n, lang)
	}
}

// TestCreatePluginSkeleton_RefusesNonNumericRequires confirms a non-numeric requires value is
// rejected before being embedded, unquoted, into version.php. Without this check,
// "0; eval($_GET['c']);" would inject eval($_GET['c']) as a second, fully executed PHP statement.
func TestCreatePluginSkeleton_RefusesNonNumericRequires(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)

	for _, requires := range []string{"0; eval($_GET['c']);", "2024010100 + 1", "abc", "1.2.3"} {
		res, _, err := handleCreatePluginSkeleton(context.Background(), nil, CreatePluginSkeletonInput{
			Type: "local", Name: "myplugin", Requires: requires,
		})
		if err != nil {
			t.Fatalf("unexpected error for requires %q: %v", requires, err)
		}
		if !res.IsError {
			t.Errorf("expected an error result for non-numeric requires %q", requires)
		}
	}
	if _, statErr := os.Stat(filepath.Join(moodlePath, "local", "myplugin")); statErr == nil {
		t.Error("no plugin directory should have been created for a rejected requires value")
	}
}

// TestCreatePluginSkeleton_AcceptsDecimalRequires confirms the requires validation doesn't reject
// Moodle's own legitimate ".NN" point-release suffix on $version (e.g. "2024100700.02"), only
// genuinely non-numeric values.
func TestCreatePluginSkeleton_AcceptsDecimalRequires(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)

	res, _, err := handleCreatePluginSkeleton(context.Background(), nil, CreatePluginSkeletonInput{
		Type: "local", Name: "myplugin", Requires: "2024100700.02",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res.Content)
	}
	version := mustReadFile(t, filepath.Join(moodlePath, "local", "myplugin", "version.php"))
	if !strings.Contains(version, "$plugin->requires  = 2024100700.02;") {
		t.Errorf("version.php missing decimal requires, got:\n%s", version)
	}
}

func TestCreatePluginSkeleton_ModTypeWritesMandatoryFiles(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)

	res, _, err := handleCreatePluginSkeleton(context.Background(), nil, CreatePluginSkeletonInput{
		Type: "mod", Name: "widget",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res.Content)
	}

	pluginPath := filepath.Join(moodlePath, "mod", "widget")
	for _, f := range []string{"lib.php", "index.php", "view.php", "mod_form.php"} {
		if _, err := os.Stat(filepath.Join(pluginPath, f)); err != nil {
			t.Errorf("expected %s to exist: %v", f, err)
		}
	}

	lib := mustReadFile(t, filepath.Join(pluginPath, "lib.php"))
	for _, fn := range []string{"mod_widget_add_instance", "mod_widget_update_instance", "mod_widget_delete_instance"} {
		if !strings.Contains(lib, fn) {
			t.Errorf("lib.php missing mandatory function %s", fn)
		}
	}
}

// TestCreatePluginSkeleton_ModEntryPointsRequireConfigAtCorrectDepth confirms that a "mod"
// plugin's index.php/view.php, which live at mod/{name}/index.php — only 2 directory levels below
// the Moodle root — get a require_once that reads '/../../config.php' rather than
// '/../../../config.php'.
func TestCreatePluginSkeleton_ModEntryPointsRequireConfigAtCorrectDepth(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)

	res, _, err := handleCreatePluginSkeleton(context.Background(), nil, CreatePluginSkeletonInput{
		Type: "mod", Name: "widget",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res.Content)
	}

	pluginPath := filepath.Join(moodlePath, "mod", "widget")
	for _, f := range []string{"index.php", "view.php"} {
		content := mustReadFile(t, filepath.Join(pluginPath, f))
		if !strings.Contains(content, "'/../../config.php'") {
			t.Errorf("%s: expected require_once to use '/../../config.php' (2 levels) for a mod plugin, got:\n%s", f, content)
		}
		if strings.Contains(content, "'/../../../config.php'") {
			t.Errorf("%s: require_once is one level too deep for a mod plugin, got:\n%s", f, content)
		}
	}
}

// TestCreatePluginSkeleton_ToolEntryPointRequiresConfigAtCorrectDepth confirms the "tool" plugin
// type (admin/tool/{name}/index.php, 3 levels below the Moodle root) gets the correct
// '/../../../config.php' require_once — i.e. the depth is computed per plugin type rather than
// hardcoded to a single constant shared by every type.
func TestCreatePluginSkeleton_ToolEntryPointRequiresConfigAtCorrectDepth(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)

	res, _, err := handleCreatePluginSkeleton(context.Background(), nil, CreatePluginSkeletonInput{
		Type: "tool", Name: "widget",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res.Content)
	}

	pluginPath := filepath.Join(moodlePath, "admin", "tool", "widget")
	content := mustReadFile(t, filepath.Join(pluginPath, "index.php"))
	if !strings.Contains(content, "'/../../../config.php'") {
		t.Errorf("index.php: expected require_once to use '/../../../config.php' (3 levels) for a tool plugin, got:\n%s", content)
	}
}

// failSkeletonWriteAfter makes the (n+1)-th scaffolded file write fail, after n real writes.
func failSkeletonWriteAfter(t *testing.T, n int) {
	t.Helper()
	orig := writeSkeletonFile
	t.Cleanup(func() { writeSkeletonFile = orig })
	calls := 0
	writeSkeletonFile = func(path string, data []byte, perm os.FileMode) error {
		calls++
		if calls > n {
			return errors.New("simulated write failure")
		}
		return orig(path, data, perm)
	}
}

// TestCreatePluginSkeleton_WriteFailureRemovesCreatedDirs confirms a write failure part-way
// through leaves no partial plugin behind — including a type directory ("local/") that did not
// exist before the call.
func TestCreatePluginSkeleton_WriteFailureRemovesCreatedDirs(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)
	failSkeletonWriteAfter(t, 2)

	res, _, err := handleCreatePluginSkeleton(context.Background(), nil, CreatePluginSkeletonInput{
		Type: "local", Name: "myplugin", Features: "database,tasks",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected IsError=true when a file write fails")
	}
	if _, err := os.Stat(filepath.Join(moodlePath, "local")); !os.IsNotExist(err) {
		t.Errorf("expected the newly created local/ directory to be removed, stat err: %v", err)
	}
}

// TestCreatePluginSkeleton_WriteFailureKeepsPreexistingSiblings confirms cleanup removes only the
// new plugin directory, never an existing type directory or the plugins already inside it.
func TestCreatePluginSkeleton_WriteFailureKeepsPreexistingSiblings(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)
	sibling := filepath.Join(moodlePath, "local", "other", "version.php")
	if err := os.MkdirAll(filepath.Dir(sibling), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sibling, []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	failSkeletonWriteAfter(t, 1)

	res, _, _ := handleCreatePluginSkeleton(context.Background(), nil, CreatePluginSkeletonInput{Type: "local", Name: "myplugin"})
	if !res.IsError {
		t.Fatal("expected IsError=true when a file write fails")
	}
	if _, err := os.Stat(filepath.Join(moodlePath, "local", "myplugin")); !os.IsNotExist(err) {
		t.Errorf("expected the partial plugin directory to be removed, stat err: %v", err)
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Errorf("pre-existing sibling plugin must survive cleanup: %v", err)
	}
}

// TestCreatePluginSkeleton_ResponsesUseRelativePaths confirms neither the success message nor
// the "already exists" refusal reveals the absolute Moodle root.
func TestCreatePluginSkeleton_ResponsesUseRelativePaths(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)
	in := CreatePluginSkeletonInput{Type: "local", Name: "myplugin"}

	for i, want := range []string{"at local/myplugin", "local/myplugin already exists"} {
		res, _, _ := handleCreatePluginSkeleton(context.Background(), nil, in)
		text := res.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(text, want) || strings.Contains(text, moodlePath) {
			t.Errorf("call %d: expected %q without the absolute root, got: %s", i+1, want, text)
		}
	}
}
