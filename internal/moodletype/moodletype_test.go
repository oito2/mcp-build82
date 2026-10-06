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

package moodletype

import (
	"os"
	"path/filepath"
	"testing"
)

// mustMkdirAll creates path and any missing parents, failing the test on error.
func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

// TestResolvePluginPath_Component verifies a "type_name" component resolves through the plugin type directory map.
func TestResolvePluginPath_Component(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "local", "myplugin")
	mustMkdirAll(t, pluginDir)

	resolved, ok := ResolvePluginPath("local_myplugin", dir)
	if !ok || resolved != pluginDir {
		t.Errorf("expected %q, got %q (ok=%v)", pluginDir, resolved, ok)
	}
}

// TestResolvePluginPath_Relative verifies a Moodle-root-relative path resolves to an existing directory.
func TestResolvePluginPath_Relative(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "mod", "quiz")
	mustMkdirAll(t, pluginDir)

	resolved, ok := ResolvePluginPath("mod/quiz", dir)
	if !ok || resolved != pluginDir {
		t.Errorf("expected %q, got %q (ok=%v)", pluginDir, resolved, ok)
	}
}

// TestResolvePluginPath_Absolute verifies an existing absolute path is returned as given.
func TestResolvePluginPath_Absolute(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "local", "myplugin")
	mustMkdirAll(t, pluginDir)

	resolved, ok := ResolvePluginPath(pluginDir, dir)
	if !ok || resolved != pluginDir {
		t.Errorf("expected %q, got %q (ok=%v)", pluginDir, resolved, ok)
	}
}

// TestResolvePluginPath_AbsoluteNotFound verifies a nonexistent absolute path yields ok=false.
func TestResolvePluginPath_AbsoluteNotFound(t *testing.T) {
	dir := t.TempDir()
	_, ok := ResolvePluginPath(filepath.Join(dir, "nope"), dir)
	if ok {
		t.Error("expected ok=false for a nonexistent absolute path")
	}
}

// TestResolvePluginPath_NotFound verifies a component whose directory does not exist yields ok=false.
func TestResolvePluginPath_NotFound(t *testing.T) {
	_, ok := ResolvePluginPath("local_nonexistent", "/tmp")
	if ok {
		t.Error("expected ok=false for a nonexistent plugin path")
	}
}

// TestResolvePluginPath_NoSlashNoUnderscore verifies an identifier with neither a slash nor an underscore yields ok=false.
func TestResolvePluginPath_NoSlashNoUnderscore(t *testing.T) {
	_, ok := ResolvePluginPath("justaword", "/tmp")
	if ok {
		t.Error("expected ok=false for an identifier with neither '/' nor '_'")
	}
}

// TestResolvePluginPath_FirstUnderscoreOnly covers the critical "split on the first underscore
// only" behavior: the plugin *type* segment never contains an underscore, but the *name* segment
// might, so a naive strings.Split(identifier, "_") would incorrectly fragment the name.
func TestResolvePluginPath_FirstUnderscoreOnly(t *testing.T) {
	cases := []struct {
		identifier   string
		expectedType string
		expectedName string
	}{
		{"assignsubmission_file", "assignsubmission", "file"},
		{"workshopallocation_random", "workshopallocation", "random"},
		{"local_my_plugin", "local", "my_plugin"},
	}

	for _, c := range cases {
		t.Run(c.identifier, func(t *testing.T) {
			dir := t.TempDir()
			typeDir, ok := PluginTypeToDir[c.expectedType]
			if !ok {
				typeDir = c.expectedType
			}
			pluginDir := filepath.Join(dir, typeDir, c.expectedName)
			mustMkdirAll(t, pluginDir)

			resolved, ok := ResolvePluginPath(c.identifier, dir)
			if !ok || resolved != pluginDir {
				t.Errorf("ResolvePluginPath(%q) = %q, %v; want %q, true", c.identifier, resolved, ok, pluginDir)
			}
		})
	}
}

// TestResolvePluginPath_UnknownTypeFallsBackToLiteral covers a plugin type not present in
// PluginTypeToDir: the directory falls back to the literal type string itself, rather than erroring.
func TestResolvePluginPath_UnknownTypeFallsBackToLiteral(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "notarealtype", "someplugin")
	mustMkdirAll(t, pluginDir)

	resolved, ok := ResolvePluginPath("notarealtype_someplugin", dir)
	if !ok || resolved != pluginDir {
		t.Errorf("expected fallback to literal type dir %q, got %q (ok=%v)", pluginDir, resolved, ok)
	}
}

// TestIsWithinMoodle verifies containment for equal, nested, sibling and parent paths.
func TestIsWithinMoodle(t *testing.T) {
	dir := t.TempDir()
	moodlePath := filepath.Join(dir, "moodle")
	mustMkdirAll(t, moodlePath)

	inside := filepath.Join(moodlePath, "local", "myplugin")
	if !IsWithinMoodle(inside, moodlePath) {
		t.Errorf("expected %q to be within %q", inside, moodlePath)
	}

	// Trailing-separator safety: a sibling directory sharing the same prefix string
	// (moodle vs moodle2) must never false-positive as "within".
	sibling := filepath.Join(dir, "moodle2", "local", "myplugin")
	if IsWithinMoodle(sibling, moodlePath) {
		t.Errorf("expected %q NOT to be within %q (prefix collision)", sibling, moodlePath)
	}

	outside := filepath.Join(dir, "elsewhere")
	if IsWithinMoodle(outside, moodlePath) {
		t.Errorf("expected %q NOT to be within %q", outside, moodlePath)
	}

	if !IsWithinMoodle(moodlePath, moodlePath) {
		t.Error("expected the moodle root itself to be considered within the moodle root")
	}
}

// TestIsWithinMoodle_RawTraversalBypass verifies that an un-normalized path whose raw string starts
// with moodlePath but whose ".." segments lexically escape it is reported as outside moodlePath.
func TestIsWithinMoodle_RawTraversalBypass(t *testing.T) {
	dir := t.TempDir()
	moodlePath := filepath.Join(dir, "moodle")
	mustMkdirAll(t, moodlePath)

	// Textually starts with moodlePath's prefix, but lexically resolves to dir's parent.
	traversal := filepath.Join(moodlePath, "local", "x", "..", "..", "..")
	if IsWithinMoodle(traversal, moodlePath) {
		t.Errorf("expected the traversal path %q to NOT be considered within %q (bypass)", traversal, moodlePath)
	}
}

// TestIsWithinMoodle_SymlinkInsideRootEscapesIsRejected verifies that a symlink located inside the
// Moodle root but pointing outside it (e.g. local/evil -> /etc) is reported as outside, even though
// its path string starts with moodlePath.
func TestIsWithinMoodle_SymlinkInsideRootEscapesIsRejected(t *testing.T) {
	dir := t.TempDir()
	moodlePath := filepath.Join(dir, "moodle")
	mustMkdirAll(t, moodlePath)

	outside := filepath.Join(dir, "outside-secret")
	mustMkdirAll(t, outside)

	evilLink := filepath.Join(moodlePath, "evil")
	if err := os.Symlink(outside, evilLink); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if IsWithinMoodle(evilLink, moodlePath) {
		t.Errorf("expected the symlink %q (pointing outside the Moodle root) to NOT be considered within %q", evilLink, moodlePath)
	}
}

// TestIsWithinMoodle_NonExistentTargetStillUsesLexicalCheck verifies that a target path that
// doesn't exist yet can't be resolved via EvalSymlinks and falls back to the plain lexical
// comparison instead of being reported as "outside".
func TestIsWithinMoodle_NonExistentTargetStillUsesLexicalCheck(t *testing.T) {
	dir := t.TempDir()
	moodlePath := filepath.Join(dir, "moodle")
	mustMkdirAll(t, moodlePath)

	nonExistent := filepath.Join(moodlePath, "local", "not_created_yet")
	if !IsWithinMoodle(nonExistent, moodlePath) {
		t.Errorf("expected the non-existent path %q to still be considered within %q", nonExistent, moodlePath)
	}
}

// TestIsWithinMoodle_SymlinkedMoodleRootStillWorks verifies that when the Moodle root itself is
// reached through a symlink, a real plugin underneath is still recognized as within it.
func TestIsWithinMoodle_SymlinkedMoodleRootStillWorks(t *testing.T) {
	dir := t.TempDir()
	realMoodle := filepath.Join(dir, "real-moodle")
	pluginDir := filepath.Join(realMoodle, "local", "demo")
	mustMkdirAll(t, pluginDir)

	moodlePath := filepath.Join(dir, "moodle-symlink")
	if err := os.Symlink(realMoodle, moodlePath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	target := filepath.Join(moodlePath, "local", "demo")
	if !IsWithinMoodle(target, moodlePath) {
		t.Errorf("expected %q to be within the symlinked Moodle root %q", target, moodlePath)
	}
}

// TestResolvePluginPath_AbsoluteIdentifierIsCleaned verifies that the absolute-path branch of
// ResolvePluginPath normalizes ".." segments before returning.
func TestResolvePluginPath_AbsoluteIdentifierIsCleaned(t *testing.T) {
	dir := t.TempDir()
	moodlePath := filepath.Join(dir, "moodle")
	mustMkdirAll(t, moodlePath)
	mustMkdirAll(t, filepath.Join(moodlePath, "local", "x"))

	identifier := filepath.Join(moodlePath, "local", "x", "..", "..", "..") // resolves to dir itself
	resolved, ok := ResolvePluginPath(identifier, moodlePath)
	if !ok {
		t.Fatalf("expected ok=true (dir exists), got resolved=%q", resolved)
	}
	if IsWithinMoodle(resolved, moodlePath) {
		t.Errorf("resolved path %q must not be considered within %q", resolved, moodlePath)
	}
}
