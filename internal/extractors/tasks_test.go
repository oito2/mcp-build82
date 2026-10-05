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

// realMoodleRoots are the entries under /srv/workspace/www/html/mdle/ that are full
// Moodle installations (root-level version.php present).
var realMoodleRoots = []string{"dev-401", "dev-402", "dev-500", "dev-uvv"}

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

func TestParseTasksPhp(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "tasks.php"), tasksFixtureWellFormed)

	result := ParseTasksPhp(filepath.Join(dir, "db", "tasks.php"))
	if result == nil || len(result.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %+v", result)
	}
	task := result.Tasks[0]
	if task.ClassName != `\local_test\task\cleanup_task` {
		t.Errorf("classname mismatch: %q", task.ClassName)
	}
	if task.Blocking != false {
		t.Errorf("blocking mismatch: %v", task.Blocking)
	}
	if task.Minute != "0" || task.Hour != "*/2" {
		t.Errorf("cron field mismatch: %+v", task)
	}
	if got := FormatCronSchedule(task); got != "0 */2 * * *" {
		t.Errorf("FormatCronSchedule mismatch: got %q", got)
	}
}

func TestParseTasksPhp_MissingClassnameSkipsEntry(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "tasks.php"), "<?php\n$tasks = [\n    ['minute' => '5'],\n];")

	result := ParseTasksPhp(filepath.Join(dir, "db", "tasks.php"))
	if result == nil || len(result.Tasks) != 0 {
		t.Fatalf("expected entries without classname to be skipped, got %+v", result)
	}
}

func TestParseTasksPhp_CronFieldsDefaultToAsterisk(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "tasks.php"), "<?php\n$tasks = [\n    ['classname' => '\\\\local_test\\\\task\\\\bare_task'],\n];")

	result := ParseTasksPhp(filepath.Join(dir, "db", "tasks.php"))
	if result == nil || len(result.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %+v", result)
	}
	task := result.Tasks[0]
	if task.Minute != "*" || task.Hour != "*" || task.Day != "*" || task.Month != "*" || task.DayOfWeek != "*" {
		t.Errorf("expected all cron fields to default to \"*\", got %+v", task)
	}
}

// TestParseTasksPhp_TreesitterBackendParity confirms BUILD82_EXTRACTOR_BACKEND=treesitter
// produces identical output to the regex backend for a normal, well-formed tasks.php.
func TestParseTasksPhp_TreesitterBackendParity(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "tasks.php"), tasksFixtureWellFormed)
	path := filepath.Join(dir, "db", "tasks.php")

	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
	regexResult := ParseTasksPhp(path)
	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
	tsResult := ParseTasksPhp(path)

	if len(regexResult.Tasks) != len(tsResult.Tasks) {
		t.Fatalf("task count mismatch: regex=%d treesitter=%d", len(regexResult.Tasks), len(tsResult.Tasks))
	}
	for i := range regexResult.Tasks {
		if regexResult.Tasks[i] != tsResult.Tasks[i] {
			t.Errorf("task %d mismatch: regex=%+v treesitter=%+v", i, regexResult.Tasks[i], tsResult.Tasks[i])
		}
	}
}

// TestParseTasksPhp_TreesitterBackendParity_RealFiles runs both backends against every real
// db/tasks.php across all 4 real Moodle installations and confirms they agree.
func TestParseTasksPhp_TreesitterBackendParity_RealFiles(t *testing.T) {
	var checked int
	for _, name := range realMoodleRoots {
		root := filepath.Join("/srv/workspace/www/html/mdle", name)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Base(path) != "tasks.php" || filepath.Base(filepath.Dir(path)) != "db" {
				return nil
			}
			checked++

			t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
			regexResult := ParseTasksPhp(path)
			t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
			tsResult := ParseTasksPhp(path)

			if len(regexResult.Tasks) != len(tsResult.Tasks) {
				t.Errorf("%s: task count mismatch: regex=%d treesitter=%d", path, len(regexResult.Tasks), len(tsResult.Tasks))
				return nil
			}
			for i := range regexResult.Tasks {
				if regexResult.Tasks[i] != tsResult.Tasks[i] {
					t.Errorf("%s: task %d mismatch: regex=%+v treesitter=%+v", path, i, regexResult.Tasks[i], tsResult.Tasks[i])
				}
			}
			return nil
		})
	}
	if checked == 0 {
		t.Skip("no db/tasks.php files found across real Moodle installations")
	}
}

// TestGetTaskClassnames_NilInputDoesNotPanic confirms GetTaskClassnames handles a nil
// *TasksExtraction (a plugin with no db/tasks.php, the common case) by returning nil instead of
// panicking on a nil pointer dereference.
func TestGetTaskClassnames_NilInputDoesNotPanic(t *testing.T) {
	if got := GetTaskClassnames(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestGetTaskClassnames_SortedNotDeduped(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	content := `<?php
$tasks = [
    ['classname' => '\\\\b\\\\task'],
    ['classname' => '\\\\a\\\\task'],
    ['classname' => '\\\\a\\\\task'],
];`
	mustWriteFile(t, filepath.Join(dir, "db", "tasks.php"), content)

	result := ParseTasksPhp(filepath.Join(dir, "db", "tasks.php"))
	names := GetTaskClassnames(result)
	if len(names) != 3 {
		t.Fatalf("expected 3 (not deduped), got %d: %v", len(names), names)
	}
	if names[0] > names[1] || names[1] > names[2] {
		t.Errorf("expected sorted order, got %v", names)
	}
}
