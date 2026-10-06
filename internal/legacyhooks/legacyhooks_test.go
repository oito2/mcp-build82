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
	"strings"
	"testing"
)

// knownLegacySuffixes is a fixed list of well-known legacy callback suffixes that Map must contain.
var knownLegacySuffixes = []string{
	"before_http_headers", "before_footer", "before_standard_html_head", "after_config",
	"after_require_login", "extend_navigation", "pre_signup_requests", "extend_signup_form",
	"validate_extend_signup_form", "course_module_viewed", "extend_course_navigation", "cron",
}

// TestMap_LoadsSuccessfully verifies the embedded map is loaded and non-empty.
func TestMap_LoadsSuccessfully(t *testing.T) {
	if len(Map) == 0 {
		t.Fatal("expected Map to be non-empty — legacy_hooks.json failed to load or is empty")
	}
}

// TestMap_ContainsAllKnownLegacySuffixes verifies every well-known legacy suffix is present in Map.
func TestMap_ContainsAllKnownLegacySuffixes(t *testing.T) {
	for _, suffix := range knownLegacySuffixes {
		fqn, ok := Map[suffix]
		if !ok {
			t.Errorf("expected Map to contain suffix %q", suffix)
			continue
		}
		if strings.TrimSpace(fqn) == "" {
			t.Errorf("expected a non-empty replacement FQN for suffix %q", suffix)
		}
	}
}

// TestMap_ValuesAreFullyQualifiedHookNames verifies every replacement value is a fully qualified hook class name.
func TestMap_ValuesAreFullyQualifiedHookNames(t *testing.T) {
	for suffix, fqn := range Map {
		if !strings.HasPrefix(fqn, `\core\hook\`) {
			t.Errorf("suffix %q: expected a \\core\\hook\\... FQN, got %q", suffix, fqn)
		}
	}
}
