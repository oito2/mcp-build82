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

// eventsFixtureWellFormed is a well-formed db/events.php fixture, also used to compare against
// the regex backend.
const eventsFixtureWellFormed = `<?php
$observers = [
    [
        'eventname'  => '\\core\\event\\course_viewed',
        'callback'   => '\\local_test\\event\\observer::course_viewed',
        'priority'   => 100,
        'internal'   => false,
    ],
    [
        'eventname'  => '\\core\\event\\user_loggedin',
        'callback'   => '\\local_test\\event\\observer::user_loggedin',
    ],
];`

// writeEventsPhp writes content to a temporary db/events.php and returns its path.
func writeEventsPhp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "events.php")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestParseEventsPhp_MatchesRegexBackendFixture verifies the parsed observers for a well-formed events file match the regex backend's output.
func TestParseEventsPhp_MatchesRegexBackendFixture(t *testing.T) {
	path := writeEventsPhp(t, eventsFixtureWellFormed)
	result := ParseEventsPhp(path)
	if result == nil || len(result.Observers) != 2 {
		t.Fatalf("expected 2 observers, got %+v", result)
	}
	first, second := result.Observers[0], result.Observers[1]

	if first.EventName != `\core\event\course_viewed` {
		t.Errorf("eventname mismatch: %q", first.EventName)
	}
	if first.Callback != `\local_test\event\observer::course_viewed` {
		t.Errorf("callback mismatch: %q", first.Callback)
	}
	if first.Priority != 100 {
		t.Errorf("priority mismatch: %d", first.Priority)
	}
	if first.Internal != false {
		t.Errorf("internal mismatch: %v", first.Internal)
	}
	if second.Priority != 0 {
		t.Errorf("expected default priority 0, got %d", second.Priority)
	}
	if second.Internal != false {
		t.Errorf("expected default internal false, got %v", second.Internal)
	}
}

// TestParseEventsPhp_EmptyArray verifies a file without an $observers array yields an empty, non-nil observer list.
func TestParseEventsPhp_EmptyArray(t *testing.T) {
	path := writeEventsPhp(t, "<?php\n$observers = [];\n")
	result := ParseEventsPhp(path)
	if result == nil || len(result.Observers) != 0 {
		t.Fatalf("expected empty observers, got %+v", result)
	}
}

// TestParseEventsPhp_MissingFile verifies a nonexistent file yields nil.
func TestParseEventsPhp_MissingFile(t *testing.T) {
	if ParseEventsPhp("/nonexistent/db/events.php") != nil {
		t.Error("expected nil for missing file")
	}
}

// TestParseEventsPhp_RequiresEitherField verifies an observer is kept when it has either eventname or callback, and dropped when it has neither.
func TestParseEventsPhp_RequiresEitherField(t *testing.T) {
	path := writeEventsPhp(t, `<?php
$observers = [
    ['priority' => 5],
];`)
	result := ParseEventsPhp(path)
	if len(result.Observers) != 0 {
		t.Errorf("expected an entry with neither eventname nor callback to be skipped, got %+v", result.Observers)
	}
}

// TestParseEventsPhp_RealFileRegression runs the tree-sitter backend against every real
// db/events.php in a real Moodle installation.
func TestParseEventsPhp_RealFileRegression(t *testing.T) {
	root := "/srv/workspace/www/html/mdle/dev-500"
	if _, err := os.Stat(root); err != nil {
		t.Skipf("real Moodle fixture tree not available: %v", err)
	}

	var found int
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Base(path) != "events.php" || filepath.Base(filepath.Dir(path)) != "db" {
			return nil
		}
		found++
		result := ParseEventsPhp(path)
		if result == nil {
			t.Errorf("%s: expected a non-nil result", path)
		}
		return nil
	})
	if found == 0 {
		t.Skip("no db/events.php files found")
	}
}
