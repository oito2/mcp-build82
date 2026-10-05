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

package tsbackend

import (
	"github.com/odvcencio/gotreesitter"

	"github.com/oito2/mcp-build82/internal/phptypes"
)

// ParseEventsPhp parses a db/events.php file's $observers array. Returns nil if the file can't be
// read or parsed — same failure semantics as the regex backend's own ParseEventsPhp.
func ParseEventsPhp(filePath string) *phptypes.EventsExtraction {
	tree, src, err := ParseFile(filePath)
	if err != nil {
		return nil
	}
	defer tree.Release()

	observersArray := FindAssignment(tree.RootNode(), MatchVariable(src, "observers"))
	if observersArray == nil {
		return &phptypes.EventsExtraction{File: filePath, Observers: []phptypes.EventObserver{}}
	}

	observers := []phptypes.EventObserver{}
	for _, entry := range ArrayElements(observersArray) {
		_, inner := KeyValue(entry) // observer entries are positional: [ [...], [...] ]
		if inner == nil {
			continue
		}
		obs := parseObserver(inner, src)
		// Same "requires either eventname or callback" inclusion rule as the regex backend.
		if obs.EventName == "" && obs.Callback == "" {
			continue
		}
		observers = append(observers, obs)
	}
	return &phptypes.EventsExtraction{File: filePath, Observers: observers}
}

// parseObserver reads one observer entry's keyed fields (eventname, callback, priority, internal).
// Absent fields keep their zero value — priority defaults 0, internal defaults false, matching the
// regex backend exactly.
func parseObserver(inner *gotreesitter.Node, src []byte) phptypes.EventObserver {
	var obs phptypes.EventObserver
	for _, element := range ArrayElements(inner) {
		key, value := KeyValue(element)
		k, ok := StringValue(key, src)
		if !ok {
			continue
		}
		switch k {
		case "eventname":
			obs.EventName, _ = StringValue(value, src)
		case "callback":
			obs.Callback, _ = StringValue(value, src)
		case "priority":
			obs.Priority, _ = IntValue(value, src)
		case "internal":
			obs.Internal, _ = BoolValue(value, src)
		}
	}
	return obs
}
