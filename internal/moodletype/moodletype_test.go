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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// TestResolvePluginPath_RejectsMalformedComponents verifies that component identifiers whose type
// or name is not a plain identifier are never joined into a path.
func TestResolvePluginPath_RejectsMalformedComponents(t *testing.T) {
	moodle := t.TempDir()
	mustMkdirAll(t, filepath.Join(moodle, "mod"))
	for _, id := range []string{"mod_..", "mod_.", "_..", "MOD_x", "mod_x/../.."} {
		if p, ok := ResolvePluginPath(id, moodle); ok && !strings.Contains(id, "/") {
			t.Errorf("ResolvePluginPath(%q) = %q, true; want false", id, p)
		}
	}
	if p, ok := ResolvePluginPath("mod_..", moodle); ok {
		t.Errorf("mod_.. resolved to %s", p)
	}
}

// TestIsWithinMoodle_NewPathUnderSymlinkedDirectory verifies that a path that does not exist yet
// is checked through the symbolic links of its existing ancestors, and that a dangling link
// ancestor is refused.
func TestIsWithinMoodle_NewPathUnderSymlinkedDirectory(t *testing.T) {
	moodle := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(moodle, "local")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if IsWithinMoodle(filepath.Join(moodle, "local", "newplugin"), moodle) {
		t.Error("a new path under a directory linked outside the root must be outside")
	}
	if err := os.Symlink(filepath.Join(outside, "gone"), filepath.Join(moodle, "dangling")); err != nil {
		t.Fatal(err)
	}
	if IsWithinMoodle(filepath.Join(moodle, "dangling", "x"), moodle) {
		t.Error("a path under a dangling symbolic link must be outside")
	}
	if !IsWithinMoodle(filepath.Join(moodle, "blocks", "new", "deeper"), moodle) {
		t.Error("a new path under plain directories must be inside")
	}
}

// TestIsRoot verifies the Moodle-root comparison, including through a symbolic link.
func TestIsRoot(t *testing.T) {
	moodle := t.TempDir()
	if !IsRoot(moodle, moodle) || !IsRoot(filepath.Join(moodle, "local", ".."), moodle) {
		t.Error("the root itself must be the root")
	}
	if IsRoot(filepath.Join(moodle, "local"), moodle) {
		t.Error("a subdirectory is not the root")
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(moodle, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if !IsRoot(link, moodle) {
		t.Error("a link to the root is the root")
	}
}

// TestPluginTypeToDir_MatchesRealComponentsJSON verifies that every core plugin type a real Moodle
// declares in lib/components.json is mapped to the same directory. The installation is read from
// BUILD82_REAL_MOODLE_DIR (default /srv/workspace/localhost/html) and the test is skipped when it is
// absent.
func TestPluginTypeToDir_MatchesRealComponentsJSON(t *testing.T) {
	root := os.Getenv("BUILD82_REAL_MOODLE_DIR")
	if root == "" {
		root = "/srv/workspace/localhost/html"
	}
	for _, install := range []string{"mdle-401", "mdle-500"} {
		data, err := os.ReadFile(filepath.Join(root, install, "lib", "components.json"))
		if err != nil {
			t.Logf("%s: skipped (%v)", install, err)
			continue
		}
		var doc struct {
			PluginTypes map[string]string `json:"plugintypes"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("%s: %v", install, err)
		}
		for typ, dir := range doc.PluginTypes {
			if got := PluginTypeToDir[typ]; got != dir {
				t.Errorf("%s: PluginTypeToDir[%q] = %q, want %q", install, typ, got, dir)
			}
		}
	}
}
