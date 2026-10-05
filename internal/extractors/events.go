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

	"github.com/oito2/mcp-build82/internal/extractors/tsbackend"
	"github.com/oito2/mcp-build82/internal/phparray"
	"github.com/oito2/mcp-build82/internal/phptypes"
)

// EventObserver and EventsExtraction are aliases for phptypes' types.
type EventObserver = phptypes.EventObserver
type EventsExtraction = phptypes.EventsExtraction

// ParseEventsPhp parses a db/events.php file. Returns nil if the file can't be read.
func ParseEventsPhp(filePath string) *EventsExtraction {
	if useTreesitter() {
		return tsbackend.ParseEventsPhp(filePath)
	}
	content, err := readFileCapped(filePath)
	if err != nil {
		return nil
	}
	body, ok := phparray.ExtractArrayBody(string(content), "observers")
	if !ok {
		return &EventsExtraction{File: filePath, Observers: []EventObserver{}}
	}

	observers := []EventObserver{}
	for _, block := range phparray.SplitIntoBlocks(body) {
		eventname := phparray.ExtractString(block, "eventname")
		callback := phparray.ExtractString(block, "callback")
		if eventname == "" && callback == "" {
			continue
		}
		observers = append(observers, EventObserver{
			EventName: eventname,
			Callback:  callback,
			Priority:  phparray.ExtractInt(block, "priority", 0),
			Internal:  phparray.ExtractBool(block, "internal", false),
		})
	}
	return &EventsExtraction{File: filePath, Observers: observers}
}

// ExtractPluginEvents parses pluginPath/db/events.php.
func ExtractPluginEvents(pluginPath string) *EventsExtraction {
	return ParseEventsPhp(filepath.Join(pluginPath, "db", "events.php"))
}

// GetEventNames returns the deduped, sorted set of event names observed. Safe to call with a nil
// e (e.g. GetEventNames(ExtractPluginEvents(path)) when the plugin has no db/events.php, the
// common case, not the exception) — returns nil rather than panicking.
func GetEventNames(e *EventsExtraction) []string {
	if e == nil {
		return nil
	}
	seen := map[string]struct{}{}
	for _, o := range e.Observers {
		seen[o.EventName] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
