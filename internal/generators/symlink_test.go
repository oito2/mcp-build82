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
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestWalkMoodleFiles_SkipsSymlinkedFiles verifies that walkMoodleFiles skips symlinked files.
// Callers read matched files' content, so following a symlink could pull in content from outside
// the scanned tree.
func TestWalkMoodleFiles_SkipsSymlinkedFiles(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "real.php"), "<?php\n")

	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.php")
	mustWriteFile(t, secret, "<?php\n// secret\n")
	if err := os.Symlink(secret, filepath.Join(root, "leaked.php")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	matches := globMoodleSuffix(root, ".php")
	var names []string
	for _, m := range matches {
		names = append(names, filepath.Base(m))
	}
	if !containsName(names, "real.php") {
		t.Errorf("expected real.php present, got %v", names)
	}
	if containsName(names, "leaked.php") {
		t.Errorf("expected the symlinked file to be skipped, got %v", names)
	}
}

// containsName reports whether `needle` is one of the strings in `haystack`.
func containsName(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// symlinkOrSkip creates the symbolic link `link` -> `target`, skipping the test where the platform
// does not allow it.
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

// TestGenerateAllForPlugin_RefusesSymlinkedContextDir verifies that a plugin whose .build82 is a
// symbolic link gets nothing written through it: every output fails, the dev marker is not
// written, and the link target stays empty.
func TestGenerateAllForPlugin_RefusesSymlinkedContextDir(t *testing.T) {
	root := copyFixtureMoodleTree(t)
	plugin := filepath.Join(root, "local", "demo")
	_ = os.RemoveAll(filepath.Join(plugin, ContextDir))
	outside := t.TempDir()
	symlinkOrSkip(t, outside, filepath.Join(plugin, ContextDir))

	result := GenerateAllForPlugin(plugin, root, true, nil)
	if len(result.Files) == 0 {
		t.Fatal("expected one result per output")
	}
	for _, r := range result.Files {
		if r.Success || !strings.Contains(r.Error, "symbolic link") {
			t.Errorf("%s: expected a refusal, got %+v", filepath.Base(r.File), r)
		}
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("%d file(s) written through the symlinked %s", len(entries), ContextDir)
	}
}

// TestGenerateAllForPlugin_ReplacesSymlinkedOutputFile verifies that an output file planted as a
// symbolic link is replaced by the generated file instead of having its target overwritten.
func TestGenerateAllForPlugin_ReplacesSymlinkedOutputFile(t *testing.T) {
	root := copyFixtureMoodleTree(t)
	plugin := filepath.Join(root, "local", "demo")
	mustMkdirAll(t, filepath.Join(plugin, ContextDir))
	target := filepath.Join(t.TempDir(), "bashrc")
	mustWriteFile(t, target, "precious\n")
	output := PluginOutputPath(plugin, "PLUGIN_STRUCTURE.md")
	_ = os.Remove(output)
	symlinkOrSkip(t, target, output)

	GenerateAllForPlugin(plugin, root, false, nil)

	if got := mustReadFile(t, target); got != "precious\n" {
		t.Errorf("the symlink target was overwritten:\n%s", got)
	}
	if fi, err := os.Lstat(output); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("expected %s to be a regular file now, got %v (err %v)", output, fi, err)
	}
}

// TestMigrateLegacyFiles_RefusesSymlinkedDestination verifies that legacy files are not moved
// into a .build82 that is a symbolic link.
func TestMigrateLegacyFiles_RefusesSymlinkedDestination(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "PLUGIN_CONTEXT.md"), "legacy\n")
	outside := t.TempDir()
	symlinkOrSkip(t, outside, filepath.Join(root, ContextDir))

	results := MigrateLegacyFiles(root, []string{"PLUGIN_CONTEXT.md"})
	if len(results) != 1 || !strings.Contains(results[0].Error, "symbolic link") {
		t.Errorf("expected a refusal, got %+v", results)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("%d file(s) moved through the symlinked %s", len(entries), ContextDir)
	}
	if _, err := os.Stat(filepath.Join(root, "PLUGIN_CONTEXT.md")); err != nil {
		t.Errorf("the legacy file must stay in place: %v", err)
	}
}

// TestGeneratePluginDependencies_SubpluginPathCannotInjectMarkdown verifies that a subplugin path
// from db/subplugins.json with line breaks, a pipe and backticks stays inside its table cell.
func TestGeneratePluginDependencies_SubpluginPathCannotInjectMarkdown(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$plugin->component = 'mod_workshop';\n")
	mustWriteFile(t, filepath.Join(dir, "db", "subplugins.json"),
		`{"plugintypes": {"workshopform": "x | y\n\n## Injected heading\n`+"```"+`"}}`)

	if r := GeneratePluginDependencies(testPluginInfo(dir), preloadPlugin(testPluginInfo(dir))); !r.Success {
		t.Fatalf("expected success, got %+v", r)
	}
	s := mustReadFile(t, PluginOutputPath(dir, "PLUGIN_DEPENDENCIES.md"))
	if strings.Contains(s, "\n## Injected heading") || strings.Contains(s, "```") {
		t.Errorf("the subplugin path broke out of its cell:\n%s", s)
	}
	if !strings.Contains(s, `| x \| y## Injected heading |`) {
		t.Errorf("expected the escaped path in its cell:\n%s", s)
	}
}

// TestGeneratePluginStructure_FileNameCannotBreakTheTree verifies that a file name with a line
// break and backticks cannot close the directory tree's code fence.
func TestGeneratePluginStructure_FileNameCannotBreakTheTree(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$plugin->component = 'local_test';\n")
	name := "x\n```\n# Injected"
	if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
		t.Skipf("file names with line breaks unavailable: %v", err)
	}

	if r := GeneratePluginStructure(testPluginInfo(dir)); !r.Success {
		t.Fatalf("expected success, got %+v", r)
	}
	s := mustReadFile(t, PluginOutputPath(dir, "PLUGIN_STRUCTURE.md"))
	if strings.Contains(s, "\n# Injected") || strings.Count(s, "```") != 2 {
		t.Errorf("the file name broke the tree:\n%s", s)
	}
}

// mustReadFile returns the content of the file `path`, failing the test on error.
func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestGeneratePluginGuide_VersionTemplate verifies that the version.php template sets the plugin's
// own date-based version and requires the installation's build number, never the Moodle release.
func TestGeneratePluginGuide_VersionTemplate(t *testing.T) {
	root := copyFixtureMoodleTree(t)
	if r := GeneratePluginGuide(root, "4.3"); !r.Success {
		t.Fatalf("expected success, got %+v", r)
	}
	s := mustReadFile(t, GlobalOutputPath(root, "MOODLE_PLUGIN_GUIDE.md"))
	if !strings.Contains(s, "$plugin->version   = "+time.Now().Format("20060102")+"00;") {
		t.Errorf("expected today's YYYYMMDD00 as the plugin version:\n%s", s)
	}
	if !strings.Contains(s, "$plugin->requires  = 2023110900") {
		t.Errorf("expected the fixture's build number as requires:\n%s", s)
	}
	if strings.Contains(s, "= 4.3;") {
		t.Errorf("the Moodle release must not be used as a version number:\n%s", s)
	}
}

// TestGenerateAiIndex_PluginLinksResolve verifies that every relative link in MOODLE_AI_INDEX.md
// points at an existing file when resolved from the index's own directory.
func TestGenerateAiIndex_PluginLinksResolve(t *testing.T) {
	root := copyFixtureMoodleTree(t)
	plugin := filepath.Join(root, "local", "demo")
	GenerateAllForPlugin(plugin, root, true, nil)
	if r := GenerateAiIndex(root, "4.3"); !r.Success {
		t.Fatalf("expected success, got %+v", r)
	}
	output := GlobalOutputPath(root, "MOODLE_AI_INDEX.md")
	s := mustReadFile(t, output)
	links := regexp.MustCompile(`\]\(([^)]+)\)`).FindAllStringSubmatch(s, -1)
	foundPlugin := false
	for _, m := range links {
		target := filepath.Join(filepath.Dir(output), filepath.FromSlash(m[1]))
		if strings.Contains(m[1], "local/demo") {
			foundPlugin = true
		}
		if _, err := os.Stat(target); err != nil {
			t.Errorf("link %q does not resolve: %v", m[1], err)
		}
	}
	if !foundPlugin {
		t.Errorf("expected a link to the plugin context:\n%s", s)
	}
}
