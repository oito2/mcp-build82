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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleasePlugin_CraftedComponentStaysInOutputDir verifies that a version.php component crafted
// to traverse directories cannot place the ZIP outside output_dir: the plugin is packaged under its
// type_name and nothing appears next to output_dir.
func TestReleasePlugin_CraftedComponentStaysInOutputDir(t *testing.T) {
	moodlePath, _ := setupResolveFixture(t)
	evil := filepath.Join(moodlePath, "local", "evil")
	mustMkdirAll(t, evil)
	mustWriteFile(t, filepath.Join(evil, "version.php"),
		"<?php\n$plugin->component = '../../escaped/pwn';\n$plugin->version = 2024010100;\n")

	base := t.TempDir()
	outDir := filepath.Join(base, "a", "b")
	mustMkdirAll(t, outDir)
	res, _, err := handleReleasePlugin(context.Background(), nil, ReleasePluginInput{Component: "local/evil", OutputDir: outDir})
	if err != nil || res.IsError {
		t.Fatalf("unexpected failure: %v %s", err, textOf(res))
	}
	if _, err := os.Stat(filepath.Join(outDir, "local_evil_2024010100.zip")); err != nil {
		t.Errorf("expected the ZIP under its type_name in output_dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "escaped")); !os.IsNotExist(err) {
		t.Errorf("something was written outside output_dir: %v", err)
	}
}

// TestPluginTools_RejectMoodleRootAndNonPlugins verifies that no tool treats the Moodle root, or a
// directory without version.php, as a plugin, and that nothing is written into the root's context
// directory or marked as under development there.
func TestPluginTools_RejectMoodleRootAndNonPlugins(t *testing.T) {
	moodlePath, _ := setupResolveFixture(t)
	mustWriteFile(t, filepath.Join(moodlePath, "version.php"), "<?php\n$version = 2024100700;\n$release = '4.5';\n")
	mustMkdirAll(t, filepath.Join(moodlePath, "lib"))

	for _, id := range []string{".", "", moodlePath, "local/.."} {
		if _, errResult := resolveAndValidatePlugin(id, moodlePath); errResult == nil {
			t.Errorf("resolveAndValidatePlugin(%q) accepted the Moodle root", id)
		}
		if _, ok := resolvePluginPathWithinMoodle(id, moodlePath); ok {
			t.Errorf("resolvePluginPathWithinMoodle(%q) accepted the Moodle root", id)
		}
	}
	for _, id := range []string{"lib/", filepath.Join(moodlePath, "lib")} {
		if _, ok := resolvePluginPathWithinMoodle(id, moodlePath); ok {
			t.Errorf("%q, a directory without version.php, was accepted as a plugin", id)
		}
	}

	if _, _, err := handleGenerateContext(context.Background(), nil, GenerateContextInput{PluginPath: "."}); err == nil {
		t.Error("generate_plugin_context accepted the Moodle root")
	}
	if _, _, err := handleBatch(context.Background(), nil, BatchInput{Mode: BatchModeList, Plugins: []string{".", "lib/"}}); err == nil ||
		!strings.Contains(err.Error(), "Could not resolve the following plugin identifiers: ., lib/") {
		t.Errorf("plugin_batch list accepted the root or lib: %v", err)
	}
	if _, out, err := handleGetPluginInfo(context.Background(), nil, GetPluginInfoInput{Plugin: "."}); err == nil && out.Path == "." {
		t.Errorf("get_plugin_info reported the Moodle root as a plugin: %+v", out)
	}

	for _, p := range []string{filepath.Join(moodlePath, ".build82", ".indevelopment"), filepath.Join(moodlePath, "lib", ".build82")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s was created: %v", p, err)
		}
	}
}

// TestCreatePluginSkeleton_RefusesTypeDirectoryLinkedOutside verifies that a plugin type directory
// that is a symbolic link pointing outside the Moodle root is not written through.
func TestCreatePluginSkeleton_RefusesTypeDirectoryLinkedOutside(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(moodlePath, "local")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	res, _, _ := handleCreatePluginSkeleton(context.Background(), nil, CreatePluginSkeletonInput{Type: "local", Name: "newplugin"})
	if res == nil || !res.IsError || !strings.Contains(textOf(res), "escapes the Moodle root") {
		t.Errorf("expected a containment refusal, got %v", res)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("%d entries written through the symlink", len(entries))
	}
}
