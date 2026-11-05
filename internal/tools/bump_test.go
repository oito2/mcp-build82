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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// bumpFixture creates a Moodle root holding the plugin `component` with the given version.php
// content and, when `upgrade` is not empty, db/upgrade.php content; it points the configuration at
// that root, fixes the bump date to 2026-10-09, and returns the plugin directory.
func bumpFixture(t *testing.T, component, versionPhp, upgrade string) string {
	t.Helper()
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)
	saved := bumpNow
	bumpNow = func() time.Time { return time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { bumpNow = saved })

	ptype, name, _ := strings.Cut(component, "_")
	typeDir := ptype
	if ptype == "block" {
		typeDir = "blocks"
	}
	dir := filepath.Join(moodlePath, typeDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "version.php"), []byte(versionPhp), 0o644); err != nil {
		t.Fatal(err)
	}
	if upgrade != "" {
		if err := os.MkdirAll(filepath.Join(dir, "db"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "db", "upgrade.php"), []byte(upgrade), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// simpleVersionPhp returns a version.php for `component` at `version`.
func simpleVersionPhp(component, version string) string {
	return "<?php\ndefined('MOODLE_INTERNAL') || die();\n\n$plugin->component = '" + component + "';\n$plugin->version   = " + version + ";\n$plugin->requires  = 2024042200;\n$plugin->maturity  = MATURITY_ALPHA;\n$plugin->release   = '1.0.0';\n"
}

// runBump calls the bump_plugin_version handler with `in` and returns its structured output, or
// the error it returned.
func runBump(t *testing.T, in BumpPluginVersionInput) (BumpPluginVersionOutput, error) {
	t.Helper()
	res, out, err := handleBumpPluginVersion(context.Background(), nil, in)
	if err == nil && res.IsError {
		t.Fatalf("unexpected error result without error: %s", textOf(res))
	}
	return out, err
}

// TestBumpPluginVersion_RewritesOnlyTheAssignments verifies that version, release and maturity
// are replaced in place, every other byte (alignment, comments mentioning the assignments, other
// properties) is kept, and the structured output reports the change.
func TestBumpPluginVersion_RewritesOnlyTheAssignments(t *testing.T) {
	before := `<?php
// $plugin->version = 1; in a comment is not an assignment.
defined('MOODLE_INTERNAL') || die();

/* $plugin->release = 'old'; */
$plugin->component = 'local_x';
$plugin->version   = 2024010100;   // YYYYMMDDXX
$plugin->requires  = 2024042200;
$plugin->maturity  = MATURITY_ALPHA;
$plugin->release   = "1.0; beta";
`
	want := `<?php
// $plugin->version = 1; in a comment is not an assignment.
defined('MOODLE_INTERNAL') || die();

/* $plugin->release = 'old'; */
$plugin->component = 'local_x';
$plugin->version   = 2025020300;   // YYYYMMDDXX
$plugin->requires  = 2024042200;
$plugin->maturity  = MATURITY_STABLE;
$plugin->release   = 'v1.1 (Dan\'s)';
`
	dir := bumpFixture(t, "local_x", before, "")
	out, err := runBump(t, BumpPluginVersionInput{Plugin: "local_x", Version: "2025020300", Release: "v1.1 (Dan's)", Maturity: "MATURITY_STABLE"})
	if err != nil {
		t.Fatal(err)
	}
	if got := mustReadFile(t, filepath.Join(dir, "version.php")); got != want {
		t.Errorf("version.php:\n%s\nwant:\n%s", got, want)
	}
	if out.OldVersion != "2024010100" || out.NewVersion != "2025020300" || out.Component != "local_x" ||
		out.Path != "local/x" || strings.Join(out.FilesChanged, ",") != "version.php" || out.UpgradeStep != "" {
		t.Errorf("unexpected output %+v", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "db")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("db/ must not be created without upgrade_step: %v", err)
	}
}

// TestBumpPluginVersion_NumericReleaseReplaced verifies that a numeric $plugin->release literal is
// replaced by the new release string.
func TestBumpPluginVersion_NumericReleaseReplaced(t *testing.T) {
	dir := bumpFixture(t, "local_x", "<?php\n$plugin->component = 'local_x';\n$plugin->version = 2024010100;\n$plugin->release = 2025032000;\n", "")
	if _, err := runBump(t, BumpPluginVersionInput{Plugin: "local_x", Release: "2.0"}); err != nil {
		t.Fatal(err)
	}
	if got := mustReadFile(t, filepath.Join(dir, "version.php")); !strings.Contains(got, "$plugin->release = '2.0';") {
		t.Errorf("release not replaced:\n%s", got)
	}
}

// TestBumpPluginVersion_DefaultVersion verifies the default new version: today as YYYYMMDD00 when
// that is greater than the current version, otherwise the current integer part + 1.
func TestBumpPluginVersion_DefaultVersion(t *testing.T) {
	for current, want := range map[string]string{
		"2024010100":    "2026100900",
		"2026100900":    "2026100901",
		"2026100905":    "2026100906",
		"2026100900.01": "2026100901",
		"2026121200":    "2026121201",
	} {
		dir := bumpFixture(t, "local_x", simpleVersionPhp("local_x", current), "")
		out, err := runBump(t, BumpPluginVersionInput{Plugin: "local_x"})
		if err != nil {
			t.Fatalf("%s: %v", current, err)
		}
		if out.NewVersion != want {
			t.Errorf("%s: new version %s, want %s", current, out.NewVersion, want)
		}
		if got := mustReadFile(t, filepath.Join(dir, "version.php")); got != simpleVersionPhp("local_x", want) {
			t.Errorf("%s: version.php:\n%s", current, got)
		}
	}
}

// TestBumpPluginVersion_RefusesWithoutWriting verifies every refusal of version.php edits and
// arguments, and that a refused call leaves the files untouched.
func TestBumpPluginVersion_RefusesWithoutWriting(t *testing.T) {
	head := "<?php\n$plugin->component = 'local_x';\n"
	cases := []struct {
		name, versionPhp string
		in               BumpPluginVersionInput
		want             string
	}{
		{"missing version", head + "$plugin->release = '1';\n", BumpPluginVersionInput{}, "no $plugin->version assignment"},
		{"duplicated version", head + "$plugin->version = 2024010100;\n$plugin->version = 2024010200;\n", BumpPluginVersionInput{}, "assigns $plugin->version 2 times"},
		{"non-literal version", head + "$plugin->version = 2024010100 + 1;\n", BumpPluginVersionInput{}, "not a plain literal"},
		{"constant version", head + "$plugin->version = MY_VERSION;\n", BumpPluginVersionInput{}, "not a plain literal"},
		{"version not greater", simpleVersionPhp("local_x", "2024010100"), BumpPluginVersionInput{Version: "2024010100"}, "not greater than"},
		{"version older", simpleVersionPhp("local_x", "2024010100"), BumpPluginVersionInput{Version: "2023010100"}, "not greater than"},
		{"version not ten digits", simpleVersionPhp("local_x", "2024010100"), BumpPluginVersionInput{Version: "20250101"}, "exactly ten digits"},
		{"default overflows ten digits", simpleVersionPhp("local_x", "9999999999"), BumpPluginVersionInput{}, "does not have ten digits"},
		{"release missing", head + "$plugin->version = 2024010100;\n", BumpPluginVersionInput{Release: "2.0"}, "no $plugin->release assignment"},
		{"release not literal", head + "$plugin->version = 2024010100;\n$plugin->release = '1.' . BUILD;\n", BumpPluginVersionInput{Release: "2.0"}, "not a plain literal"},
		{"release constant", head + "$plugin->version = 2024010100;\n$plugin->release = MY_RELEASE;\n", BumpPluginVersionInput{Release: "2.0"}, "not a plain literal"},
		{"release interpolated", head + "$plugin->version = 2024010100;\n$plugin->release = \"v$x\";\n", BumpPluginVersionInput{Release: "2.0"}, "not a plain literal"},
		{"release with newline", simpleVersionPhp("local_x", "2024010100"), BumpPluginVersionInput{Release: "2.0\n$x = 1;"}, "single line"},
		{"unknown maturity", simpleVersionPhp("local_x", "2024010100"), BumpPluginVersionInput{Maturity: "MATURITY_GOLD"}, "Unrecognized maturity"},
		{"maturity missing", head + "$plugin->version = 2024010100;\n", BumpPluginVersionInput{Maturity: "MATURITY_BETA"}, "no $plugin->maturity assignment"},
		{"maturity not literal", head + "$plugin->version = 2024010100;\n$plugin->maturity = $m;\n", BumpPluginVersionInput{Maturity: "MATURITY_BETA"}, "not a plain literal"},
		{"description closes php", simpleVersionPhp("local_x", "2024010100"), BumpPluginVersionInput{UpgradeStep: true, UpgradeDescription: "a ?> b"}, "upgrade_description"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := bumpFixture(t, "local_x", c.versionPhp, "")
			c.in.Plugin = "local_x"
			_, err := runBump(t, c.in)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("expected an error containing %q, got %v", c.want, err)
			}
			if got := mustReadFile(t, filepath.Join(dir, "version.php")); got != c.versionPhp {
				t.Errorf("version.php changed by a refused call:\n%s", got)
			}
			if _, err := os.Stat(filepath.Join(dir, "db")); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("db/ created by a refused call: %v", err)
			}
		})
	}
}

// TestBumpPluginVersion_RefusesPluginOutsideMoodle verifies that a plugin path outside the Moodle
// root is refused.
func TestBumpPluginVersion_RefusesPluginOutsideMoodle(t *testing.T) {
	bumpFixture(t, "local_x", simpleVersionPhp("local_x", "2024010100"), "")
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "version.php"), []byte(simpleVersionPhp("local_y", "2024010100")), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runBump(t, BumpPluginVersionInput{Plugin: outside}); err == nil || !strings.Contains(err.Error(), "within the Moodle installation") {
		t.Fatalf("expected a containment error, got %v", err)
	}
	if got := mustReadFile(t, filepath.Join(outside, "version.php")); got != simpleVersionPhp("local_y", "2024010100") {
		t.Errorf("version.php outside the Moodle root was changed:\n%s", got)
	}
}

// TestBumpPluginVersion_UpgradeStepAppended verifies the step inserted before the final
// `return true;` of an existing db/upgrade.php, for each savepoint family, and that the doctor
// upgrade-consistency check finds nothing wrong with the result.
func TestBumpPluginVersion_UpgradeStepAppended(t *testing.T) {
	cases := []struct{ component, function, savepoint string }{
		{"local_x", "xmldb_local_x_upgrade", "upgrade_plugin_savepoint(true, 2026100900, 'local', 'x');"},
		{"mod_x", "xmldb_x_upgrade", "upgrade_mod_savepoint(true, 2026100900, 'x');"},
		{"block_x", "xmldb_block_x_upgrade", "upgrade_block_savepoint(true, 2026100900, 'x');"},
	}
	for _, c := range cases {
		t.Run(c.component, func(t *testing.T) {
			before := "<?php\nfunction " + c.function + "($oldversion) {\n    global $DB;\n\n    if ($oldversion < 2024010200) {\n        " +
				strings.Replace(c.savepoint, "2026100900", "2024010200", 1) + "\n    }\n\n    return true;\n}\n"
			want := "<?php\nfunction " + c.function + "($oldversion) {\n    global $DB;\n\n    if ($oldversion < 2024010200) {\n        " +
				strings.Replace(c.savepoint, "2026100900", "2024010200", 1) + "\n    }\n\n" +
				"    if ($oldversion < 2026100900) {\n        // Add the reports table.\n        // TODO: implement this upgrade step.\n\n" +
				"        // " + c.component + " savepoint reached.\n        " + c.savepoint + "\n    }\n\n    return true;\n}\n"
			dir := bumpFixture(t, c.component, simpleVersionPhp(c.component, "2024010200"), before)
			out, err := runBump(t, BumpPluginVersionInput{Plugin: c.component, UpgradeStep: true, UpgradeDescription: "Add the reports table."})
			if err != nil {
				t.Fatal(err)
			}
			if got := mustReadFile(t, filepath.Join(dir, "db", "upgrade.php")); got != want {
				t.Errorf("db/upgrade.php:\n%s\nwant:\n%s", got, want)
			}
			if out.UpgradeFileCreated || strings.Join(out.FilesChanged, ",") != "db/upgrade.php,version.php" || !strings.Contains(out.UpgradeStep, c.savepoint) {
				t.Errorf("unexpected output %+v", out)
			}
			if issues := upgradeIssues(t, dir); len(issues) != 0 {
				t.Errorf("doctor reports issues on the generated step: %+v", issues)
			}
		})
	}
}

// TestBumpPluginVersion_UpgradeFileCreated verifies that a missing db/upgrade.php is created from
// the template with the new step, and that the doctor check accepts it.
func TestBumpPluginVersion_UpgradeFileCreated(t *testing.T) {
	dir := bumpFixture(t, "local_x", simpleVersionPhp("local_x", "2024010100"), "")
	out, err := runBump(t, BumpPluginVersionInput{Plugin: "local_x", UpgradeStep: true})
	if err != nil {
		t.Fatal(err)
	}
	want := `<?php
defined('MOODLE_INTERNAL') || die();

/**
 * Upgrade steps for local_x.
 *
 * @param int $oldversion The version the plugin is upgraded from.
 * @return bool
 */
function xmldb_local_x_upgrade($oldversion) {
    global $DB;
    $dbman = $DB->get_manager();

    if ($oldversion < 2026100900) {
        // TODO: implement this upgrade step.

        // local_x savepoint reached.
        upgrade_plugin_savepoint(true, 2026100900, 'local', 'x');
    }

    return true;
}
`
	if got := mustReadFile(t, filepath.Join(dir, "db", "upgrade.php")); got != want {
		t.Errorf("db/upgrade.php:\n%s\nwant:\n%s", got, want)
	}
	if !out.UpgradeFileCreated {
		t.Errorf("expected upgrade_file_created, got %+v", out)
	}
	if issues := upgradeIssues(t, dir); len(issues) != 0 {
		t.Errorf("doctor reports issues on the created file: %+v", issues)
	}
}

// TestBumpPluginVersion_UpgradeStepKeepsStyle verifies that the step follows the file's CRLF line
// endings and tab indentation.
func TestBumpPluginVersion_UpgradeStepKeepsStyle(t *testing.T) {
	before := "<?php\r\nfunction xmldb_local_x_upgrade($oldversion) {\r\n\tglobal $DB;\r\n\treturn true;\r\n}\r\n"
	want := "<?php\r\nfunction xmldb_local_x_upgrade($oldversion) {\r\n\tglobal $DB;\r\n" +
		"\tif ($oldversion < 2026100900) {\r\n\t\t// TODO: implement this upgrade step.\r\n\r\n" +
		"\t\t// local_x savepoint reached.\r\n\t\tupgrade_plugin_savepoint(true, 2026100900, 'local', 'x');\r\n\t}\r\n\r\n" +
		"\treturn true;\r\n}\r\n"
	dir := bumpFixture(t, "local_x", simpleVersionPhp("local_x", "2024010100"), before)
	if _, err := runBump(t, BumpPluginVersionInput{Plugin: "local_x", UpgradeStep: true}); err != nil {
		t.Fatal(err)
	}
	if got := mustReadFile(t, filepath.Join(dir, "db", "upgrade.php")); got != want {
		t.Errorf("db/upgrade.php:\n%q\nwant:\n%q", got, want)
	}
}

// TestBumpPluginVersion_UpgradeRefusals verifies that db/upgrade.php files the step cannot be
// placed in safely are refused, leaving both files untouched.
func TestBumpPluginVersion_UpgradeRefusals(t *testing.T) {
	cases := []struct{ name, upgrade, want string }{
		{"function missing", "<?php\nfunction xmldb_local_other_upgrade($oldversion) {\n    return true;\n}\n", "does not define function xmldb_local_x_upgrade()"},
		{"function only in a comment", "<?php\n// function xmldb_local_x_upgrade($oldversion) {\nfunction helper() {\n    return true;\n}\n", "does not define function"},
		{"no final return true", "<?php\nfunction xmldb_local_x_upgrade($oldversion) {\n    $result = true;\n    return $result;\n}\n", "does not end with a `return true;`"},
		{"return not on its own line", "<?php\nfunction xmldb_local_x_upgrade($oldversion) {\n    if ($x) return true;\n}\n", "does not end with a `return true;`"},
		{"statement after return", "<?php\nfunction xmldb_local_x_upgrade($oldversion) {\n    return true;\n    log();\n}\n", "does not end with a `return true;`"},
		{"unclosed function", "<?php\nfunction xmldb_local_x_upgrade($oldversion) {\n    return true;\n", "cannot find the end"},
		{"step already present", "<?php\nfunction xmldb_local_x_upgrade($oldversion) {\n    if ($oldversion < 2026100900) {\n        upgrade_plugin_savepoint(true, 2026100900, 'local', 'x');\n    }\n    return true;\n}\n", "already has a step for version 2026100900"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := bumpFixture(t, "local_x", simpleVersionPhp("local_x", "2024010100"), c.upgrade)
			_, err := runBump(t, BumpPluginVersionInput{Plugin: "local_x", Version: "2026100900", UpgradeStep: true})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("expected an error containing %q, got %v", c.want, err)
			}
			if got := mustReadFile(t, filepath.Join(dir, "db", "upgrade.php")); got != c.upgrade {
				t.Errorf("db/upgrade.php changed by a refused call:\n%s", got)
			}
			if got := mustReadFile(t, filepath.Join(dir, "version.php")); got != simpleVersionPhp("local_x", "2024010100") {
				t.Errorf("version.php changed by a refused call:\n%s", got)
			}
		})
	}
}

// TestBumpPluginVersion_BracesInStringsDoNotEndTheFunction verifies that braces inside strings
// and comments of the upgrade function do not end its body early.
func TestBumpPluginVersion_BracesInStringsDoNotEndTheFunction(t *testing.T) {
	before := "<?php\nfunction xmldb_local_x_upgrade($oldversion) {\n    $a = '}';\n    // }\n    $b = \"{$a}\";\n    return true;\n}\n"
	dir := bumpFixture(t, "local_x", simpleVersionPhp("local_x", "2024010100"), before)
	if _, err := runBump(t, BumpPluginVersionInput{Plugin: "local_x", UpgradeStep: true}); err != nil {
		t.Fatal(err)
	}
	got := mustReadFile(t, filepath.Join(dir, "db", "upgrade.php"))
	if !strings.Contains(got, "$b = \"{$a}\";\n    if ($oldversion < 2026100900) {") || !strings.HasSuffix(got, "    }\n\n    return true;\n}\n") {
		t.Errorf("step misplaced:\n%s", got)
	}
}

// TestBumpPluginVersion_RollsBackUpgradeWhenVersionWriteFails verifies that when writing
// version.php fails, db/upgrade.php is restored (or removed with the db directory it created).
func TestBumpPluginVersion_RollsBackUpgradeWhenVersionWriteFails(t *testing.T) {
	failVersion := func(t *testing.T) {
		saved := writeBumpFile
		writeBumpFile = func(path string, data []byte, perm os.FileMode) error {
			if filepath.Base(path) == "version.php" {
				return errors.New("disk full")
			}
			return saved(path, data, perm)
		}
		t.Cleanup(func() { writeBumpFile = saved })
	}
	versionPhp := simpleVersionPhp("local_x", "2024010100")

	t.Run("existing file restored", func(t *testing.T) {
		upgrade := "<?php\nfunction xmldb_local_x_upgrade($oldversion) {\n    return true;\n}\n"
		dir := bumpFixture(t, "local_x", versionPhp, upgrade)
		failVersion(t)
		_, err := runBump(t, BumpPluginVersionInput{Plugin: "local_x", UpgradeStep: true})
		if err == nil || !strings.Contains(err.Error(), "disk full") || !strings.Contains(err.Error(), "was restored") {
			t.Fatalf("expected the write error and a restore note, got %v", err)
		}
		if got := mustReadFile(t, filepath.Join(dir, "db", "upgrade.php")); got != upgrade {
			t.Errorf("db/upgrade.php not restored:\n%s", got)
		}
		if got := mustReadFile(t, filepath.Join(dir, "version.php")); got != versionPhp {
			t.Errorf("version.php changed:\n%s", got)
		}
	})

	t.Run("created file removed", func(t *testing.T) {
		dir := bumpFixture(t, "local_x", versionPhp, "")
		failVersion(t)
		if _, err := runBump(t, BumpPluginVersionInput{Plugin: "local_x", UpgradeStep: true}); err == nil {
			t.Fatal("expected an error")
		}
		if _, err := os.Stat(filepath.Join(dir, "db")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("db/ left behind after a failed call: %v", err)
		}
	})
}

// TestBumpPluginVersion_TextReport verifies the Markdown report lists the change, the files and
// the generated step.
func TestBumpPluginVersion_TextReport(t *testing.T) {
	bumpFixture(t, "local_x", simpleVersionPhp("local_x", "2024010100"), "")
	res, _, err := handleBumpPluginVersion(context.Background(), nil, BumpPluginVersionInput{Plugin: "local_x", UpgradeStep: true, Release: "1.1.0"})
	if err != nil {
		t.Fatal(err)
	}
	text := textOf(res)
	for _, want := range []string{"Bumped local_x from 2024010100 to 2026100900", "Release: 1.1.0", "- db/upgrade.php (created)", "- version.php", "```php\n    if ($oldversion < 2026100900) {"} {
		if !strings.Contains(text, want) {
			t.Errorf("report missing %q:\n%s", want, text)
		}
	}
}
