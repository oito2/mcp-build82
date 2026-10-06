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

// tasksFixtureWellFormed is a well-formed db/tasks.php fixture.
const tasksFixtureWellFormed = `<?php
$tasks = [
    [
        'classname'  => '\\local_test\\task\\cleanup_task',
        'blocking'   => 0,
        'minute'     => '0',
        'hour'       => '*/2',
        'day'        => '*',
        'month'      => '*',
        'dayofweek'  => '*',
    ],
];`

// writeTasksPhp writes content to a temporary db/tasks.php and returns its path.
func writeTasksPhp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.php")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestParseTasksPhp_MatchesRegexBackendFixture verifies the parsed tasks for a well-formed tasks file match the regex backend's output.
func TestParseTasksPhp_MatchesRegexBackendFixture(t *testing.T) {
	path := writeTasksPhp(t, tasksFixtureWellFormed)
	result := ParseTasksPhp(path)
	if result == nil || len(result.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %+v", result)
	}
	task := result.Tasks[0]
	if task.ClassName != `\local_test\task\cleanup_task` {
		t.Errorf("classname mismatch: %q", task.ClassName)
	}
	if task.Blocking != false {
		t.Errorf("blocking mismatch: %v (blocking=>0 must be false)", task.Blocking)
	}
	if task.Minute != "0" || task.Hour != "*/2" {
		t.Errorf("cron field mismatch: %+v", task)
	}
}

// TestParseTasksPhp_MissingClassnameSkipsEntry verifies an entry without classname is dropped.
func TestParseTasksPhp_MissingClassnameSkipsEntry(t *testing.T) {
	path := writeTasksPhp(t, "<?php\n$tasks = [\n    ['minute' => '5'],\n];")
	result := ParseTasksPhp(path)
	if result == nil || len(result.Tasks) != 0 {
		t.Fatalf("expected entries without classname to be skipped, got %+v", result)
	}
}

// TestParseTasksPhp_CronFieldsDefaultToAsterisk verifies omitted cron fields default to "*".
func TestParseTasksPhp_CronFieldsDefaultToAsterisk(t *testing.T) {
	path := writeTasksPhp(t, "<?php\n$tasks = [\n    ['classname' => '\\\\local_test\\\\task\\\\bare_task'],\n];")
	result := ParseTasksPhp(path)
	if result == nil || len(result.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %+v", result)
	}
	task := result.Tasks[0]
	if task.Minute != "*" || task.Hour != "*" || task.Day != "*" || task.Month != "*" || task.DayOfWeek != "*" {
		t.Errorf("expected all cron fields to default to \"*\", got %+v", task)
	}
}

// TestParseTasksPhp_EmptyArray verifies a file without a $tasks array yields an empty, non-nil task list.
func TestParseTasksPhp_EmptyArray(t *testing.T) {
	path := writeTasksPhp(t, "<?php\n$tasks = [];\n")
	result := ParseTasksPhp(path)
	if result == nil || len(result.Tasks) != 0 {
		t.Fatalf("expected empty tasks, got %+v", result)
	}
}

// TestParseTasksPhp_MissingFile verifies a nonexistent file yields nil.
func TestParseTasksPhp_MissingFile(t *testing.T) {
	if ParseTasksPhp("/nonexistent/db/tasks.php") != nil {
		t.Error("expected nil for missing file")
	}
}

// realMoodleRoots are the entries under /srv/workspace/www/html/mdle/ that are full
// Moodle installations (have a root-level version.php).
var realMoodleRoots = []string{"dev-401", "dev-402", "dev-500", "dev-uvv"}

// TestParseTasksPhp_RealFileRegression runs the tree-sitter backend against every real
// db/tasks.php across all 4 real Moodle installations.
func TestParseTasksPhp_RealFileRegression(t *testing.T) {
	var found int
	for _, name := range realMoodleRoots {
		root := filepath.Join("/srv/workspace/www/html/mdle", name)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Base(path) != "tasks.php" || filepath.Base(filepath.Dir(path)) != "db" {
				return nil
			}
			found++
			if result := ParseTasksPhp(path); result == nil {
				t.Errorf("%s: expected a non-nil result", path)
			}
			return nil
		})
	}
	if found == 0 {
		t.Skip("no db/tasks.php files found across real Moodle installations")
	}
}
