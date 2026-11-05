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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// upgradeFixture writes a plugin `component` at version `version` into a temp Moodle tree, with
// the db/install.xml tables `tables` (table name → field names) and `upgradeBody` as the body of
// its xmldb_<component>_upgrade() function, and returns the plugin directory.
func upgradeFixture(t *testing.T, component, version string, tables map[string][]string, upgradeBody string) string {
	t.Helper()
	ptype, name, _ := strings.Cut(component, "_")
	dir := filepath.Join(t.TempDir(), ptype, name)
	if err := os.MkdirAll(filepath.Join(dir, "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel, content string) {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("version.php", "<?php\n$plugin->component = '"+component+"';\n$plugin->version = "+version+";\n")
	var xml strings.Builder
	xml.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\" ?>\n<XMLDB PATH=\"x\" VERSION=\"1\">\n<TABLES>\n")
	for table, fields := range tables {
		xml.WriteString("<TABLE NAME=\"" + table + "\">\n<FIELDS>\n")
		for _, f := range fields {
			xml.WriteString("<FIELD NAME=\"" + f + "\" TYPE=\"int\" LENGTH=\"10\" NOTNULL=\"true\"/>\n")
		}
		xml.WriteString("</FIELDS>\n</TABLE>\n")
	}
	xml.WriteString("</TABLES>\n</XMLDB>\n")
	write("db/install.xml", xml.String())
	write("db/upgrade.php", "<?php\nfunction xmldb_"+component+"_upgrade($oldversion) {\n    global $DB;\n    $dbman = $DB->get_manager();\n"+upgradeBody+"\n    return true;\n}\n")
	return dir
}

// upgradeIssues runs checkUpgradeConsistency on the plugin at `dir` and returns its non-ok results.
func upgradeIssues(t *testing.T, dir string) []checkResult {
	t.Helper()
	var issues []checkResult
	for _, r := range checkUpgradeConsistency(loadDevPlugins("", []string{dir})) {
		if r.Status != statusOK {
			issues = append(issues, r)
		}
	}
	return issues
}

// expectOneIssue fails unless `issues` holds exactly one result with `status` whose detail
// contains `want`.
func expectOneIssue(t *testing.T, issues []checkResult, status checkStatus, want string) {
	t.Helper()
	if len(issues) != 1 || issues[0].Status != status || !strings.Contains(issues[0].Detail, want) {
		t.Errorf("expected one %s issue containing %q, got %+v", status, want, issues)
	}
}

// TestUpgradeConsistency_Rules verifies each rule on a plugin that breaks only that rule.
func TestUpgradeConsistency_Rules(t *testing.T) {
	items := map[string][]string{"local_x_items": {"id", "name"}}
	cases := []struct {
		name, component, version string
		tables                   map[string][]string
		body                     string
		status                   checkStatus
		want                     string
	}{
		{"step without savepoint", "local_x", "2024010200", items, `
    if ($oldversion < 2024010200) {
        $x = 1;
    }`, statusFail, "has no savepoint call"},
		{"savepoint for another version", "local_x", "2024010200", items, `
    if ($oldversion < 2024010200) {
        upgrade_plugin_savepoint(true, 2024010300, 'local', 'x');
    }`, statusFail, "ends with a savepoint for version 2024010300"},
		{"wrong savepoint for a module", "mod_x", "2024010200", items, `
    if ($oldversion < 2024010200) {
        upgrade_plugin_savepoint(true, 2024010200, 'mod', 'x');
    }`, statusFail, "a mod plugin must call upgrade_mod_savepoint(true, 2024010200, 'x')"},
		{"wrong plugin name", "local_x", "2024010200", items, `
    if ($oldversion < 2024010200) {
        upgrade_plugin_savepoint(true, 2024010200, 'local', 'local_x');
    }`, statusFail, "upgrade_plugin_savepoint(true, 2024010200, 'local', 'x')"},
		{"steps not increasing", "local_x", "2024010300", items, `
    if ($oldversion < 2024010300) {
        upgrade_plugin_savepoint(true, 2024010300, 'local', 'x');
    }
    if ($oldversion < 2024010200) {
        upgrade_plugin_savepoint(true, 2024010200, 'local', 'x');
    }`, statusFail, "steps must increase"},
		{"step newer than version.php", "local_x", "2024010100", items, `
    if ($oldversion < 2024010200) {
        upgrade_plugin_savepoint(true, 2024010200, 'local', 'x');
    }`, statusFail, "newer than $plugin->version 2024010100"},
		{"created table missing from install.xml", "local_x", "2024010200", items, `
    if ($oldversion < 2024010200) {
        $table = new xmldb_table('local_x_new');
        $table->add_field('id', XMLDB_TYPE_INTEGER, '10');
        $dbman->create_table($table);
        upgrade_plugin_savepoint(true, 2024010200, 'local', 'x');
    }`, statusFail, "leaves table local_x_new in place, but db/install.xml does not define it"},
		{"added field missing from install.xml", "local_x", "2024010200", items, `
    if ($oldversion < 2024010200) {
        $table = new xmldb_table('local_x_items');
        $field = new xmldb_field('extra', XMLDB_TYPE_INTEGER, '10');
        $dbman->add_field($table, $field);
        upgrade_plugin_savepoint(true, 2024010200, 'local', 'x');
    }`, statusFail, "leaves field local_x_items.extra in place"},
		{"dropped table still in install.xml", "local_x", "2024010200", items, `
    if ($oldversion < 2024010200) {
        $table = new xmldb_table('local_x_items');
        $dbman->drop_table($table);
        upgrade_plugin_savepoint(true, 2024010200, 'local', 'x');
    }`, statusWarn, "removes table local_x_items, but db/install.xml still defines it"},
		{"dropped field still in install.xml", "local_x", "2024010200", items, `
    if ($oldversion < 2024010200) {
        $table = new xmldb_table('local_x_items');
        $field = new xmldb_field('name');
        $dbman->drop_field($table, $field);
        upgrade_plugin_savepoint(true, 2024010200, 'local', 'x');
    }`, statusWarn, "removes field local_x_items.name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := upgradeFixture(t, tc.component, tc.version, tc.tables, tc.body)
			expectOneIssue(t, upgradeIssues(t, dir), tc.status, tc.want)
		})
	}
}

// TestUpgradeConsistency_ValidPatterns verifies that patterns found in real Moodle plugins raise no
// issue: the block savepoint with its $allowabort argument, a field added then renamed (judged by
// its final name), a field renamed through setName() and a string variable, a variable assigned by
// foreach or renamed with a foreach variable (unknown, so skipped), an add/drop/add sequence in an
// if/else, a field added then dropped in the same step (only the last operation counts), and a name
// a later step handles through a helper function.
func TestUpgradeConsistency_ValidPatterns(t *testing.T) {
	block := upgradeFixture(t, "block_x", "2024010200", map[string][]string{"block_x_t": {"id"}}, `
    if ($oldversion < 2024010200) {
        upgrade_block_savepoint(true, 2024010200, 'x', false);
    }`)
	if issues := upgradeIssues(t, block); len(issues) != 0 {
		t.Errorf("block savepoint with $allowabort: unexpected issues %+v", issues)
	}

	dir := upgradeFixture(t, "local_x", "2024010500", map[string][]string{"local_x_items": {"id", "newname", "style", "kept"}}, `
    if ($oldversion < 2024010200) {
        $table = new xmldb_table('local_x_items');
        $field = new xmldb_field('oldname', XMLDB_TYPE_CHAR, '10');
        $dbman->add_field($table, $field);
        upgrade_plugin_savepoint(true, 2024010200, 'local', 'x');
    }
    if ($oldversion < 2024010300) {
        $table = new xmldb_table('local_x_items');
        $field = new xmldb_field('oldname', XMLDB_TYPE_CHAR, '10');
        $dbman->rename_field($table, $field, 'newname');
        $other = new xmldb_field('brief', XMLDB_TYPE_CHAR, '10');
        $target = 'style';
        $other->setName($target);
        $dbman->add_field($table, $other);
        upgrade_plugin_savepoint(true, 2024010300, 'local', 'x');
    }
    if ($oldversion < 2024010400) {
        $table = new xmldb_table('local_x_items');
        foreach (['a' => 1] as $loopname => $loopfield) {
            $loopfield->setName($loopname);
            $dbman->add_field($table, $loopfield);
        }
        $field = new xmldb_field('kept', XMLDB_TYPE_INTEGER, '4');
        if (!$dbman->field_exists($table, $field)) {
            $dbman->add_field($table, $field);
        } else {
            $dbman->drop_field($table, $field);
            $dbman->add_field($table, $field);
        }
        $gone = new xmldb_field('legacy');
        $dbman->add_field($table, $gone);
        upgrade_plugin_savepoint(true, 2024010400, 'local', 'x');
    }
    if ($oldversion < 2024010500) {
        local_x_drop_column($dbman, 'local_x_items', 'legacy');
        $table = new xmldb_table('local_x_items');
        $scratch = new xmldb_field('scratch', XMLDB_TYPE_INTEGER, '4');
        $dbman->add_field($table, $scratch);
        $dbman->drop_field($table, $scratch);
        $loopfield = new xmldb_field('beforeloop', XMLDB_TYPE_INTEGER, '4');
        foreach ($list as $loopfield) {
            $dbman->add_field($table, $loopfield);
        }
        $renamed = new xmldb_field('beforerename', XMLDB_TYPE_INTEGER, '4');
        $label = 'bogus';
        foreach ($names as $label) {
            $renamed->setName($label);
            $dbman->add_field($table, $renamed);
        }
        upgrade_plugin_savepoint(true, 2024010500, 'local', 'x');
    }`)
	if issues := upgradeIssues(t, dir); len(issues) != 0 {
		t.Errorf("valid patterns: unexpected issues %+v", issues)
	}
}

// TestUpgradeConsistency_DemoFixtureIsClean verifies the shared demo plugin raises no upgrade or
// Privacy API issue.
func TestUpgradeConsistency_DemoFixtureIsClean(t *testing.T) {
	demo := filepath.Join("..", "..", "testdata", "moodle", "local", "demo")
	for _, r := range append(checkUpgradeConsistency(loadDevPlugins("", []string{demo})), checkPrivacyApi(loadDevPlugins("", []string{demo}))...) {
		if r.Status != statusOK {
			t.Errorf("demo fixture: unexpected issue %+v", r)
		}
	}
}

// privacyFixture writes a local_p plugin with an optional classes/privacy/provider.php
// (`provider`, omitted when empty) and a db/install.xml table holding `fields`, and returns its
// directory.
func privacyFixture(t *testing.T, provider string, fields ...string) string {
	t.Helper()
	dir := upgradeFixture(t, "local_p", "2024010100", map[string][]string{"local_p_t": fields}, "")
	if provider != "" {
		if err := os.MkdirAll(filepath.Join(dir, "classes", "privacy"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "classes", "privacy", "provider.php"), []byte(provider), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestPrivacyApi_Rules verifies each Privacy API warning and the clean cases.
func TestPrivacyApi_Rules(t *testing.T) {
	const nullProvider = "<?php\nnamespace local_p\\privacy;\nclass provider implements \\core_privacy\\local\\metadata\\null_provider {}\n"
	const metadataProvider = "<?php\nnamespace local_p\\privacy;\nuse core_privacy\\local\\metadata\\collection;\n" +
		"class provider implements \\core_privacy\\local\\metadata\\provider, \\core_privacy\\local\\request\\plugin\\provider {}\n"
	const commentOnly = "<?php\n// Not a null_provider yet: implement \\core_privacy\\local\\metadata\\provider.\nclass provider {}\n"
	cases := []struct {
		name, provider string
		fields         []string
		want           string
	}{
		{"missing provider", "", []string{"id"}, "no classes/privacy/provider.php"},
		{"neither interface (names only in comments)", commentOnly, []string{"id"}, "implements neither"},
		{"null provider storing user data", nullProvider, []string{"id", "userid"}, "declares a null_provider, but db/install.xml table local_p_t stores user data (field userid)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			results := checkPrivacyApi(loadDevPlugins("", []string{privacyFixture(t, tc.provider, tc.fields...)}))
			if len(results) != 1 || results[0].Status != statusWarn || !strings.Contains(results[0].Detail, tc.want) {
				t.Errorf("expected one warning containing %q, got %+v", tc.want, results)
			}
		})
	}
	for name, dir := range map[string]string{
		"null provider without user data":  privacyFixture(t, nullProvider, "id", "name"),
		"metadata provider with user data": privacyFixture(t, metadataProvider, "id", "userid"),
	} {
		if r := checkPrivacyApi(loadDevPlugins("", []string{dir})); len(r) != 1 || r[0].Status != statusOK {
			t.Errorf("%s: expected ok, got %+v", name, r)
		}
	}
}
