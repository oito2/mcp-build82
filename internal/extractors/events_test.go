// Copyright (C) 2026  oito2
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
	"os"
	"path/filepath"
	"testing"
)

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

func TestParseEventsPhp(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "events.php"), eventsFixtureWellFormed)

	result := ParseEventsPhp(filepath.Join(dir, "db", "events.php"))
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

func TestParseEventsPhp_EmptyArray(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "events.php"), "<?php\n$observers = [];\n")

	result := ParseEventsPhp(filepath.Join(dir, "db", "events.php"))
	if result == nil || len(result.Observers) != 0 {
		t.Fatalf("expected empty observers, got %+v", result)
	}
}

func TestParseEventsPhp_MissingFile(t *testing.T) {
	if ParseEventsPhp("/nonexistent/db/events.php") != nil {
		t.Error("expected nil for missing file")
	}
}

// TestParseEventsPhp_TreesitterBackendParity confirms BUILD82_EXTRACTOR_BACKEND=treesitter
// produces identical output to the regex backend for a normal, well-formed events.php.
func TestParseEventsPhp_TreesitterBackendParity(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "events.php"), eventsFixtureWellFormed)
	path := filepath.Join(dir, "db", "events.php")

	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
	regexResult := ParseEventsPhp(path)

	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
	tsResult := ParseEventsPhp(path)

	if len(regexResult.Observers) != len(tsResult.Observers) {
		t.Fatalf("observer count mismatch: regex=%d treesitter=%d", len(regexResult.Observers), len(tsResult.Observers))
	}
	for i := range regexResult.Observers {
		if regexResult.Observers[i] != tsResult.Observers[i] {
			t.Errorf("observer %d mismatch: regex=%+v treesitter=%+v", i, regexResult.Observers[i], tsResult.Observers[i])
		}
	}
}

// TestParseEventsPhp_TreesitterBackendParity_RealFiles runs both backends against every real
// db/events.php in a real Moodle installation and confirms they agree.
func TestParseEventsPhp_TreesitterBackendParity_RealFiles(t *testing.T) {
	root := "/srv/workspace/www/html/mdle/dev-500"
	if _, err := os.Stat(root); err != nil {
		t.Skipf("real Moodle fixture tree not available: %v", err)
	}

	var checked int
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Base(path) != "events.php" || filepath.Base(filepath.Dir(path)) != "db" {
			return nil
		}
		checked++

		t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
		regexResult := ParseEventsPhp(path)
		t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
		tsResult := ParseEventsPhp(path)

		if len(regexResult.Observers) != len(tsResult.Observers) {
			t.Errorf("%s: observer count mismatch: regex=%d treesitter=%d", path, len(regexResult.Observers), len(tsResult.Observers))
			return nil
		}
		for i := range regexResult.Observers {
			if regexResult.Observers[i] != tsResult.Observers[i] {
				t.Errorf("%s: observer %d mismatch: regex=%+v treesitter=%+v", path, i, regexResult.Observers[i], tsResult.Observers[i])
			}
		}
		return nil
	})
	if checked == 0 {
		t.Skip("no db/events.php files found")
	}
}

// TestGetEventNames_NilInputDoesNotPanic verifies that GetEventNames handles a nil input without
// panicking: ParseEventsPhp returns nil when the plugin has no db/events.php, so
// GetEventNames(ExtractPluginEvents(path)) must be safe to call.
func TestGetEventNames_NilInputDoesNotPanic(t *testing.T) {
	if got := GetEventNames(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestGetEventNames_DedupedAndSorted(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	content := `<?php
$observers = [
    ['eventname' => '\\core\\event\\b_event', 'callback' => 'x::b'],
    ['eventname' => '\\core\\event\\a_event', 'callback' => 'x::a'],
    ['eventname' => '\\core\\event\\a_event', 'callback' => 'x::a2'],
];`
	mustWriteFile(t, filepath.Join(dir, "db", "events.php"), content)

	result := ParseEventsPhp(filepath.Join(dir, "db", "events.php"))
	names := GetEventNames(result)
	want := []string{`\core\event\a_event`, `\core\event\b_event`}
	if len(names) != len(want) || names[0] != want[0] || names[1] != want[1] {
		t.Errorf("got %v, want %v", names, want)
	}
}
