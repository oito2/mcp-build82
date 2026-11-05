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

package extractors

import (
	"path/filepath"
	"strings"
	"testing"
)

// modFooFixture writes an activity module mod_foo whose language file is lang/en/foo.php, as
// Moodle requires for modules, and returns its directory.
func modFooFixture(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "mod", "foo")
	mustMkdirAll(t, filepath.Join(dir, "lang", "en"))
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$plugin->component = 'mod_foo';\n$plugin->version = 2024010100;\n")
	mustWriteFile(t, filepath.Join(dir, "lang", "en", "foo.php"), "<?php\n$string['pluginname'] = 'Foo activity';\n$string['modulename'] = 'Foo';\n")
	mustWriteFile(t, filepath.Join(dir, "lib.php"), "<?php\n"+
		"function foo_before_footer() {\n    return get_string('pluginname', 'mod_foo') . get_string('missingone', 'foo');\n}\n"+
		"function foo_add_instance($data) {\n    return get_string('modulename', 'foo');\n}\n")
	return dir
}

// TestModPlugin_LangFileAndComponentNames verifies that an activity module's display name and
// declared strings come from lang/en/<name>.php, and that get_string() calls naming the module by
// its component or by its short name are both recognized as its own.
func TestModPlugin_LangFileAndComponentNames(t *testing.T) {
	dir := modFooFixture(t)

	info, err := DetectPlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.DisplayName != "Foo activity" {
		t.Errorf("display name = %q, want the pluginname from lang/en/foo.php", info.DisplayName)
	}
	declared := ExtractLangStrings(dir, "mod_foo")
	if _, ok := declared["pluginname"]; !ok || len(declared) != 2 {
		t.Errorf("declared strings = %v, want pluginname and modulename", declared)
	}
	got := map[string]bool{}
	for _, c := range FindOwnGetStringCalls(dir, "mod_foo") {
		got[c.Identifier] = true
	}
	for _, want := range []string{"pluginname", "missingone", "modulename"} {
		if !got[want] {
			t.Errorf("get_string(%q, ...) not recognized as the module's own call; got %v", want, got)
		}
	}
}

// TestModPlugin_LegacyCallbackShortName verifies, on both extraction backends, that a module's
// legacy callback declared with its short name (foo_before_footer) is detected.
func TestModPlugin_LegacyCallbackShortName(t *testing.T) {
	dir := modFooFixture(t)
	for _, backend := range []string{"", "treesitter"} {
		t.Run("backend="+backend, func(t *testing.T) {
			t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
			hooks, err := ExtractPluginHooks(dir, "mod_foo")
			if err != nil {
				t.Fatal(err)
			}
			if len(hooks.LegacyWarnings) != 1 || hooks.LegacyWarnings[0].LegacyFunction != "foo_before_footer" {
				t.Errorf("legacy warnings = %+v, want foo_before_footer", hooks.LegacyWarnings)
			}
		})
	}
}

// TestExtractFunctionsFromPhpFile_DocBlockBeforeAttributes verifies, on both backends, that a
// PHPDoc block is found across a single-line or multi-line PHP 8 attribute, and with a "**/"
// closer, so the deprecated functions of Moodle 4.5+ are classified as deprecated.
func TestExtractFunctionsFromPhpFile_DocBlockBeforeAttributes(t *testing.T) {
	file := filepath.Join(t.TempDir(), "lib.php")
	mustWriteFile(t, file, `<?php
/**
 * Old one.
 * @deprecated since 4.5
 */
#[\core\attribute\deprecated('x', since: '4.5', mdl: 'MDL-1')] // Kept for compatibility.
function local_x_single() {
}

/**
 * Old two.
 * @deprecated since 4.5
 */
#[\core\attribute\deprecated(
    'y',
    since: '4.5',
)]
function local_x_multi() {
}

/**
 * Old three.
 * @deprecated since 4.5
 **/
function local_x_closer() {
}
`)
	for _, backend := range []string{"", "treesitter"} {
		t.Run("backend="+backend, func(t *testing.T) {
			t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
			got := map[string]ApiVisibility{}
			for _, f := range ExtractFunctionsFromPhpFile(file) {
				got[f.Name] = f.Visibility
			}
			for _, name := range []string{"local_x_single", "local_x_multi", "local_x_closer"} {
				if got[name] != VisDeprecated {
					t.Errorf("%s: visibility %v, want deprecated (all: %v)", name, got[name], got)
				}
			}
		})
	}
}

// TestScanClasses_ModifiersBeforeClass verifies, on both backends, that final, readonly and
// abstract readonly classes are found with the right kind.
func TestScanClasses_ModifiersBeforeClass(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "local", "x")
	mustMkdirAll(t, filepath.Join(dir, "classes"))
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$plugin->component = 'local_x';\n")
	for name, decl := range map[string]string{
		"a": "final class a {}", "b": "readonly class b {}", "c": "final readonly class c {}",
		"d": "abstract readonly class d {}", "e": "class e {}",
	} {
		mustWriteFile(t, filepath.Join(dir, "classes", name+".php"), "<?php\nnamespace local_x;\n\n"+decl+"\n")
	}
	want := map[string]ClassKind{"a": "class", "b": "class", "c": "class", "d": "abstract class", "e": "class"}
	for _, backend := range []string{"", "treesitter"} {
		t.Run("backend="+backend, func(t *testing.T) {
			t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
			got := map[string]ClassKind{}
			for _, c := range ExtractPluginClasses(dir).Classes {
				got[c.Name] = c.Kind
			}
			for name, kind := range want {
				if got[name] != kind {
					t.Errorf("class %s: kind %q, want %q (all: %v)", name, got[name], kind, got)
				}
			}
		})
	}
}

// TestClassConstantRegistrations verifies, on both backends, that db/hooks.php, db/events.php,
// db/tasks.php and db/services.php entries written with `X::class` (alone, concatenated with a
// method, or in a [class, method] array) are read instead of skipped.
func TestClassConstantRegistrations(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "local", "x")
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$plugin->component = 'local_x';\n")
	mustWriteFile(t, filepath.Join(dir, "db", "hooks.php"), `<?php
$callbacks = [
    ['hook' => \core\hook\output\before_footer_html_generation::class, 'callback' => [\local_x\hook_callbacks::class, 'before_footer']],
    ['hook' => \core\hook\after_config::class, 'callback' => \local_x\hook_callbacks::class . '::after_config'],
    ['hook' => 'core\hook\navigation\primary_extend', 'callback' => ['local_x\hook_callbacks', 'primary']],
];
`)
	mustWriteFile(t, filepath.Join(dir, "db", "events.php"), `<?php
$observers = [
    ['eventname' => \core\event\course_viewed::class, 'callback' => [\local_x\observer::class, 'viewed']],
];
`)
	mustWriteFile(t, filepath.Join(dir, "db", "tasks.php"), `<?php
$tasks = [
    ['classname' => \local_x\task\cleanup::class, 'blocking' => 0, 'minute' => '*', 'hour' => '*', 'day' => '*', 'month' => '*', 'dayofweek' => '*'],
];
`)
	mustWriteFile(t, filepath.Join(dir, "db", "services.php"), `<?php
$functions = [
    'local_x_get' => ['classname' => \local_x\external\get::class, 'description' => 'Get.', 'type' => 'read'],
];
`)
	wantHooks := map[string]string{
		`\core\hook\output\before_footer_html_generation`: `\local_x\hook_callbacks::before_footer`,
		`\core\hook\after_config`:                         `\local_x\hook_callbacks::after_config`,
		`core\hook\navigation\primary_extend`:             `local_x\hook_callbacks::primary`,
	}
	for _, backend := range []string{"", "treesitter"} {
		t.Run("backend="+backend, func(t *testing.T) {
			t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
			hooks, err := ExtractPluginHooks(dir, "local_x")
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]string{}
			for _, c := range hooks.Callbacks {
				got[c.HookName] = c.Callback
			}
			if len(got) != len(wantHooks) {
				t.Errorf("hook callbacks = %v, want %v", got, wantHooks)
			}
			for hook, cb := range wantHooks {
				if got[hook] != cb {
					t.Errorf("hook %s: callback %q, want %q", hook, got[hook], cb)
				}
			}
			events := ExtractPluginEvents(dir)
			if events == nil || len(events.Observers) != 1 || events.Observers[0].EventName != `\core\event\course_viewed` ||
				events.Observers[0].Callback != `\local_x\observer::viewed` {
				t.Errorf("observers = %+v", events)
			}
			tasks := ExtractPluginTasks(dir)
			if tasks == nil || len(tasks.Tasks) != 1 || tasks.Tasks[0].ClassName != `\local_x\task\cleanup` {
				t.Errorf("tasks = %+v", tasks)
			}
			services := ExtractPluginServices(dir)
			if services == nil || len(services.Functions) != 1 || services.Functions[0].ClassName != `\local_x\external\get` {
				t.Errorf("services = %+v", services)
			}
		})
	}
}

// TestHookDefinitions_Attributes verifies, on both backends, that a hook class declaring its label,
// tags and replaced callbacks through Moodle 4.4+ attributes is read from those attributes.
func TestHookDefinitions_Attributes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "local", "x")
	mustMkdirAll(t, filepath.Join(dir, "classes", "hook"))
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$plugin->component = 'local_x';\n")
	mustWriteFile(t, filepath.Join(dir, "classes", "hook", "before_thing.php"), `<?php
namespace local_x\hook;

use core\attribute;

/**
 * Docblock summary that the label attribute overrides.
 */
#[\core\attribute\tags('output', 'course')]
#[\core\attribute\label('Allows plugins to act before the thing.')]
#[attribute\hook\replaces_callbacks('before_thing', 'old_thing')]
final class before_thing {
}
`)
	for _, backend := range []string{"", "treesitter"} {
		t.Run("backend="+backend, func(t *testing.T) {
			t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
			hooks, err := ExtractPluginHooks(dir, "local_x")
			if err != nil {
				t.Fatal(err)
			}
			if len(hooks.Definitions) != 1 {
				t.Fatalf("definitions = %+v", hooks.Definitions)
			}
			d := hooks.Definitions[0]
			if d.Description != "Allows plugins to act before the thing." || strings.Join(d.Tags, ",") != "output,course" ||
				d.Replaces != "before_thing, old_thing" {
				t.Errorf("definition = %+v", d)
			}
		})
	}
}

// TestBooleans_CaseInsensitive verifies, on both backends, that PHP's case-insensitive TRUE/True
// are read as true in db/services.php and db/tasks.php.
func TestBooleans_CaseInsensitive(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "local", "x")
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "version.php"), "<?php\n$plugin->component = 'local_x';\n")
	mustWriteFile(t, filepath.Join(dir, "db", "services.php"), `<?php
$functions = ['local_x_get' => ['classname' => 'local_x\external\get', 'description' => 'Get.', 'type' => 'read', 'ajax' => TRUE]];
`)
	mustWriteFile(t, filepath.Join(dir, "db", "tasks.php"), `<?php
$tasks = [['classname' => 'local_x\task\cleanup', 'blocking' => True, 'minute' => '*', 'hour' => '*', 'day' => '*', 'month' => '*', 'dayofweek' => '*']];
`)
	for _, backend := range []string{"", "treesitter"} {
		t.Run("backend="+backend, func(t *testing.T) {
			t.Setenv("BUILD82_EXTRACTOR_BACKEND", backend)
			if s := ExtractPluginServices(dir); s == nil || len(s.Functions) != 1 || !s.Functions[0].Ajax {
				t.Errorf("services = %+v", s)
			}
			if tk := ExtractPluginTasks(dir); tk == nil || len(tk.Tasks) != 1 || !tk.Tasks[0].Blocking {
				t.Errorf("tasks = %+v", tk)
			}
		})
	}
}
