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
	"github.com/odvcencio/gotreesitter"

	"github.com/oito2/mcp-build82/internal/phptypes"
)

// ParseTasksPhp parses a db/tasks.php file's $tasks array. Only classname gates inclusion (unlike
// events, which accepts either eventname or callback) — an entry with no classname is skipped
// entirely, matching the regex backend exactly.
func ParseTasksPhp(filePath string) *phptypes.TasksExtraction {
	tree, src, err := ParseFile(filePath)
	if err != nil {
		return nil
	}
	defer tree.Release()

	tasksArray := FindAssignment(tree.RootNode(), MatchVariable(src, "tasks"))
	if tasksArray == nil {
		return &phptypes.TasksExtraction{File: filePath, Tasks: []phptypes.ScheduledTask{}}
	}

	tasks := []phptypes.ScheduledTask{}
	for _, entry := range ArrayElements(tasksArray) {
		_, inner := KeyValue(entry) // task entries are positional: [ [...], [...] ]
		if inner == nil {
			continue
		}
		task, ok := parseTask(inner, src)
		if ok {
			tasks = append(tasks, task)
		}
	}
	return &phptypes.TasksExtraction{File: filePath, Tasks: tasks}
}

// parseTask reads one task entry's keyed fields. The bool return is false when classname is
// absent — the caller skips the entry entirely, matching the regex backend's own gating rule.
// Cron fields default to "*" (not ""), also matching the regex backend exactly.
func parseTask(inner *gotreesitter.Node, src []byte) (phptypes.ScheduledTask, bool) {
	task := phptypes.ScheduledTask{Minute: "*", Hour: "*", Day: "*", Month: "*", DayOfWeek: "*"}
	haveClassname := false

	for _, element := range ArrayElements(inner) {
		key, value := KeyValue(element)
		k, ok := StringValue(key, src)
		if !ok {
			continue
		}
		switch k {
		case "classname":
			if s, ok := StringValue(value, src); ok {
				task.ClassName = s
				haveClassname = true
			}
		case "blocking":
			task.Blocking, _ = BoolValue(value, src)
		case "minute":
			if s, ok := StringValue(value, src); ok {
				task.Minute = orDefault(s, "*")
			}
		case "hour":
			if s, ok := StringValue(value, src); ok {
				task.Hour = orDefault(s, "*")
			}
		case "day":
			if s, ok := StringValue(value, src); ok {
				task.Day = orDefault(s, "*")
			}
		case "month":
			if s, ok := StringValue(value, src); ok {
				task.Month = orDefault(s, "*")
			}
		case "dayofweek":
			if s, ok := StringValue(value, src); ok {
				task.DayOfWeek = orDefault(s, "*")
			}
		case "disabled":
			task.Disabled, _ = BoolValue(value, src)
		}
	}
	return task, haveClassname
}
