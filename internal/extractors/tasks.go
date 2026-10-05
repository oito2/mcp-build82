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
	"path/filepath"
	"sort"
	"strings"

	"github.com/oito2/mcp-build82/internal/extractors/tsbackend"
	"github.com/oito2/mcp-build82/internal/phparray"
	"github.com/oito2/mcp-build82/internal/phptypes"
)

// ScheduledTask and TasksExtraction are aliases for phptypes' types.
type ScheduledTask = phptypes.ScheduledTask
type TasksExtraction = phptypes.TasksExtraction

// ParseTasksPhp parses a db/tasks.php file. Returns nil if the file can't be read.
func ParseTasksPhp(filePath string) *TasksExtraction {
	if useTreesitter() {
		return tsbackend.ParseTasksPhp(filePath)
	}
	content, err := readFileCapped(filePath)
	if err != nil {
		return nil
	}
	body, ok := phparray.ExtractArrayBody(string(content), "tasks")
	if !ok {
		return &TasksExtraction{File: filePath, Tasks: []ScheduledTask{}}
	}

	tasks := []ScheduledTask{}
	for _, block := range phparray.SplitIntoBlocks(body) {
		classname := phparray.ExtractString(block, "classname")
		if classname == "" {
			continue
		}
		tasks = append(tasks, ScheduledTask{
			ClassName: classname,
			Blocking:  phparray.ExtractBool(block, "blocking", false),
			Minute:    orDefault(phparray.ExtractString(block, "minute"), "*"),
			Hour:      orDefault(phparray.ExtractString(block, "hour"), "*"),
			Day:       orDefault(phparray.ExtractString(block, "day"), "*"),
			Month:     orDefault(phparray.ExtractString(block, "month"), "*"),
			DayOfWeek: orDefault(phparray.ExtractString(block, "dayofweek"), "*"),
			Disabled:  phparray.ExtractBool(block, "disabled", false),
		})
	}
	return &TasksExtraction{File: filePath, Tasks: tasks}
}

// ExtractPluginTasks parses pluginPath/db/tasks.php.
func ExtractPluginTasks(pluginPath string) *TasksExtraction {
	return ParseTasksPhp(filepath.Join(pluginPath, "db", "tasks.php"))
}

// FormatCronSchedule renders a task's cron fields as a standard 5-field cron expression.
func FormatCronSchedule(t ScheduledTask) string {
	return strings.Join([]string{t.Minute, t.Hour, t.Day, t.Month, t.DayOfWeek}, " ")
}

// GetTaskClassnames returns the sorted (not deduped) list of task classnames. Safe to call with a
// nil e (the plugin has no db/tasks.php, the common case) — returns nil rather than panicking.
func GetTaskClassnames(e *TasksExtraction) []string {
	if e == nil {
		return nil
	}
	names := make([]string, len(e.Tasks))
	for i, t := range e.Tasks {
		names[i] = t.ClassName
	}
	sort.Strings(names)
	return names
}
