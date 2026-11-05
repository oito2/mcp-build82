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

package legacyhooks

import (
	"regexp"
	"testing"
)

// knownLegacyCallbacks maps each lib.php callback suffix that Moodle core declares as replaced by a
// hook (through the hook class's replaces_callbacks attribute or get_deprecated_plugin_callbacks())
// to that hook class.
var knownLegacyCallbacks = map[string]string{
	"add_htmlattributes":               `\core\hook\output\before_html_attributes`,
	"after_config":                     `\core\hook\after_config`,
	"after_file_created":               `\core_files\hook\after_file_created`,
	"before_file_created":              `\core_files\hook\before_file_created`,
	"before_footer":                    `\core\hook\output\before_footer_html_generation`,
	"before_http_headers":              `\core\hook\output\before_http_headers`,
	"before_standard_html_head":        `\core\hook\output\before_standard_head_html_generation`,
	"before_standard_top_of_body_html": `\core\hook\output\before_standard_top_of_body_html_generation`,
	"bulk_user_actions":                `\core_user\hook\extend_bulk_user_actions`,
	"standard_after_main_region_html":  `\core\hook\output\after_standard_main_region_html_generation`,
	"standard_footer_html":             `\core\hook\output\before_standard_footer_html_generation`,
}

// notReplacedCallbacks are lib.php callbacks that core still calls and that no hook replaces, so
// Map must not report them as legacy.
var notReplacedCallbacks = []string{
	"extend_navigation", "extend_settings_navigation", "extend_navigation_course", "pre_signup_requests",
	"extend_signup_form", "validate_extend_signup_form", "after_require_login", "cron", "course_module_viewed",
}

// TestMap_LoadsSuccessfully verifies the embedded map is loaded and non-empty.
func TestMap_LoadsSuccessfully(t *testing.T) {
	if len(Map) == 0 {
		t.Fatal("expected Map to be non-empty — legacy_hooks.json failed to load or is empty")
	}
}

// TestMap_MatchesCoreReplacements verifies that Map holds exactly the callbacks core declares as
// replaced, each with its hook class, and none of the callbacks core still supports.
func TestMap_MatchesCoreReplacements(t *testing.T) {
	if len(Map) != len(knownLegacyCallbacks) {
		t.Errorf("Map has %d entries, want %d: %v", len(Map), len(knownLegacyCallbacks), Map)
	}
	for suffix, want := range knownLegacyCallbacks {
		if got := Map[suffix]; got != want {
			t.Errorf("Map[%q] = %q, want %q", suffix, got, want)
		}
	}
	for _, suffix := range notReplacedCallbacks {
		if fqn, ok := Map[suffix]; ok {
			t.Errorf("%q is not replaced by a hook, but Map maps it to %q", suffix, fqn)
		}
	}
}

// TestMap_ValuesAreFullyQualifiedHookNames verifies every replacement value is a fully qualified
// class name in a core component's hook namespace.
func TestMap_ValuesAreFullyQualifiedHookNames(t *testing.T) {
	for suffix, fqn := range Map {
		if !hookClassPattern.MatchString(fqn) {
			t.Errorf("suffix %q: expected a \\core[_component]\\hook\\... FQN, got %q", suffix, fqn)
		}
	}
}

// hookClassPattern matches a fully qualified hook class of a core component, such as
// \core\hook\output\before_http_headers or \core_files\hook\after_file_created.
var hookClassPattern = regexp.MustCompile(`^\\core(_[a-z]+)?\\hook\\([a-z_]+\\)*[a-z_]+$`)
