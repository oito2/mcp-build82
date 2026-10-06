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

package tsbackend

import (
	"os"
	"path/filepath"
	"testing"
)

// writeVersionPhp writes content to a temporary version.php and returns the directory containing it.
func writeVersionPhp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "version.php"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestReadVersionPhp_MatchesRegexBackendFixture is a parity test against the same fixture the
// regex backend's DetectPlugin test uses.
func TestReadVersionPhp_MatchesRegexBackendFixture(t *testing.T) {
	dir := writeVersionPhp(t, `<?php
defined('MOODLE_INTERNAL') || die();
$plugin->component = 'local_test';
$plugin->version   = 2024010100;
$plugin->requires  = 2023100900;
$plugin->maturity  = MATURITY_STABLE;
$plugin->release   = '1.0.0';
`)
	component, version, requires, maturity := ReadVersionPhp(dir)
	if component != "local_test" || version != "2024010100" || requires != "2023100900" || maturity != "MATURITY_STABLE" {
		t.Errorf("got (%q, %q, %q, %q), want (\"local_test\", \"2024010100\", \"2023100900\", \"MATURITY_STABLE\")",
			component, version, requires, maturity)
	}
}

// TestReadVersionPhp_MissingFieldsComeBackEmpty verifies absent fields are returned as empty strings.
func TestReadVersionPhp_MissingFieldsComeBackEmpty(t *testing.T) {
	dir := writeVersionPhp(t, "<?php\n$version = 2024010100;\n") // no $plugin->* at all
	component, version, requires, maturity := ReadVersionPhp(dir)
	if component != "" || version != "" || requires != "" || maturity != "" {
		t.Errorf("expected all-empty result, got (%q, %q, %q, %q)", component, version, requires, maturity)
	}
}

// TestReadVersionPhp_NonexistentFile verifies a missing version.php yields empty values for every field.
func TestReadVersionPhp_NonexistentFile(t *testing.T) {
	dir := t.TempDir() // no version.php at all
	component, version, requires, maturity := ReadVersionPhp(dir)
	if component != "" || version != "" || requires != "" || maturity != "" {
		t.Errorf("expected all-empty result for a missing file, got (%q, %q, %q, %q)", component, version, requires, maturity)
	}
}

// TestReadVersionPhp_KnownDivergence_CommentedOutComponent verifies that a commented-out
// `$plugin->component` line is treated as a comment and yields nothing. The regex backend matches
// such a line.
func TestReadVersionPhp_KnownDivergence_CommentedOutComponent(t *testing.T) {
	dir := writeVersionPhp(t, "<?php\n// $plugin->component = 'old_name';\n")
	component, _, _, _ := ReadVersionPhp(dir)
	if component != "" {
		t.Errorf("expected the tree-sitter backend to find no component in a commented-out line, got %q", component)
	}
}

// realVersionPhpFixtures returns every version.php under a real Moodle installation, for a
// real-file pass beyond hand-authored fixtures. Skips (not fails) if the path isn't available in
// the current environment.
func realVersionPhpFixtures(t *testing.T, root string, limit int) []string {
	t.Helper()
	if _, err := os.Stat(root); err != nil {
		t.Skipf("real Moodle fixture tree not available at %s: %v", root, err)
	}
	var found []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || len(found) >= limit {
			return nil
		}
		if filepath.Base(path) == "version.php" {
			found = append(found, path)
		}
		return nil
	})
	return found
}

// TestReadVersionPhp_RealFileRegression runs the tree-sitter backend against real version.php
// files from a real Moodle installation (not just the synthetic fixtures above).
func TestReadVersionPhp_RealFileRegression(t *testing.T) {
	paths := realVersionPhpFixtures(t, "/srv/workspace/www/html/mdle/dev-500", 25)
	if len(paths) == 0 {
		t.Skip("no version.php files found")
	}
	for _, path := range paths {
		dir := filepath.Dir(path)
		component, version, _, _ := ReadVersionPhp(dir)
		if component == "" && version == "" {
			t.Errorf("%s: expected at least component or version to be extracted, got nothing", path)
		}
	}
}
