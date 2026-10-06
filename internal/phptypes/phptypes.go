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

// Package phptypes holds the pure data-shape types shared by both extraction backends
// (internal/extractors' regex backend and internal/extractors/tsbackend's tree-sitter backend).
// Neither backend can import the other, so this neutral package contains only struct/type
// definitions, no logic, and both backends return the exact same Go types. internal/extractors
// re-exports every type here as a type alias (e.g. `type ApiFunction = phptypes.ApiFunction`);
// internal/extractors/tsbackend uses these types directly.
package phptypes

import "github.com/oito2/mcp-build82/internal/phpdoc"

// ApiFunction is one top-level function found in a Moodle lib file.
type ApiFunction struct {
	Name       string
	File       string // basename, e.g. "moodlelib.php"
	FilePath   string // absolute path
	Visibility phpdoc.Visibility
	Doc        *phpdoc.PhpDocBlock
	Line       int // 1-based
}

// Capability is one entry in db/access.php's $capabilities map.
type Capability struct {
	Name         string
	CapType      string // default "read"
	ContextLevel string
	RiskBitmask  string
	Archetypes   map[string]string
}

// CapabilitiesExtraction is the parsed content of a db/access.php file.
type CapabilitiesExtraction struct {
	File         string
	Capabilities []Capability
}

// ClassKind is the kind of a class-like declaration: one of "class", "abstract class", "interface", "trait", "enum".
type ClassKind string

// PhpClass is one class/interface/trait/enum declaration found in a plugin or Moodle core.
type PhpClass struct {
	Name, Namespace, FQN string
	Kind                 ClassKind
	File                 string // relative to the scan root
	Extends              string // empty if none
	Implements           []string
}

// RenamedClass is one entry in db/renamedclasses.php's autoload map. NewName has any `::class`
// suffix, surrounding quotes, and leading backslash stripped, so it's a bare FQN comparable with
// PhpClass.FQN.
type RenamedClass struct {
	OldName string // e.g. "block_accessreview"
	NewName string // e.g. "block_accessreview\output\main"
}

// ClassesExtraction is the parsed content of a plugin/directory's class declarations plus its
// renamed-class autoload map.
type ClassesExtraction struct {
	Classes        []PhpClass
	RenamedClasses []RenamedClass
}

// EventObserver is one entry in db/events.php's $observers array.
type EventObserver struct {
	EventName string // e.g. \core\event\course_viewed
	Callback  string
	Priority  int  // default 0
	Internal  bool // default false
}

// EventsExtraction is the parsed content of a db/events.php file.
type EventsExtraction struct {
	File      string
	Observers []EventObserver
}

// HookCallback is one entry in db/hooks.php's $callbacks array: this plugin listens to a hook.
type HookCallback struct {
	HookName       string
	Callback       string
	Priority       int  // default 0
	DefaultEnabled bool // default true
}

// HookDefinition is a hook this plugin defines, discovered under classes/hook/*.php.
type HookDefinition struct {
	ClassName   string // FQN, e.g. \local_test\hook\data_submitted
	Description string
	Tags        []string
	Replaces    string // legacy lib.php callback this hook definition supersedes, if any
}

// LegacyCallbackWarning flags a legacy lib.php callback with a known Hook API replacement.
type LegacyCallbackWarning struct {
	LegacyFunction string
	ReplacedBy     string
	Guidance       string
}

// HooksExtraction is the combined result of scanning a plugin's registered hook callbacks, its own
// hook definitions, and its legacy-callback usage.
type HooksExtraction struct {
	Callbacks      []HookCallback
	Definitions    []HookDefinition
	LegacyWarnings []LegacyCallbackWarning
}

// WebServiceFunction is one entry in db/services.php's $functions map.
type WebServiceFunction struct {
	Name, ClassName, MethodName, Description, Type, Capabilities string
	Ajax, LoginRequired                                          bool
}

// ServicesExtraction is the parsed content of a db/services.php file.
type ServicesExtraction struct {
	File      string
	Functions []WebServiceFunction
}

// ScheduledTask is one entry in db/tasks.php's $tasks array.
type ScheduledTask struct {
	ClassName                           string
	Blocking                            bool
	Minute, Hour, Day, Month, DayOfWeek string
	Disabled                            bool
}

// TasksExtraction is the parsed content of a db/tasks.php file.
type TasksExtraction struct {
	File  string
	Tasks []ScheduledTask
}

// UpgradeStep is one version-gated step in a db/upgrade.php xmldb_{component}_upgrade() function.
type UpgradeStep struct {
	Version     string // 10-digit YYYYMMDDXX
	Description string
}

// UpgradeExtraction is the parsed content of a db/upgrade.php file.
type UpgradeExtraction struct {
	File  string
	Steps []UpgradeStep
}
