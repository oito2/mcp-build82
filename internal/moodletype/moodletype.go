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

package moodletype

import (
	"os"
	"path/filepath"
	"strings"
)

// PluginTypeToDir is the canonical Moodle plugin type -> directory map (37 entries).
var PluginTypeToDir = map[string]string{
	"mod":                "mod",
	"block":              "blocks",
	"local":              "local",
	"tool":               "admin/tool",
	"auth":               "auth",
	"enrol":              "enrol",
	"theme":              "theme",
	"report":             "report",
	"format":             "course/format",
	"filter":             "filter",
	"qtype":              "question/type",
	"availability":       "availability/condition",
	"assignsubmission":   "mod/assign/submission",
	"assignfeedback":     "mod/assign/feedback",
	"gradereport":        "grade/report",
	"gradeimport":        "grade/import",
	"gradeexport":        "grade/export",
	"plagiarism":         "plagiarism",
	"portfolio":          "portfolio/type",
	"repository":         "repository",
	"profilefield":       "user/profile/field",
	"workshopform":       "mod/workshop/form",
	"workshopallocation": "mod/workshop/allocation",
	"workshopeval":       "mod/workshop/evaluation",
	"datafield":          "mod/data/field",
	"datapreset":         "mod/data/preset",
	"ltisource":          "mod/lti/source",
	"ltiservice":         "mod/lti/service",
	"quizaccess":         "mod/quiz/accessrule",
	"scormreport":        "mod/scorm/report",
	"tinymce":            "lib/editor/tinymce/plugins",
	"atto":               "lib/editor/atto/plugins",
	"editor":             "lib/editor",
	"adminpresets":       "admin/presets",
	"antivirus":          "lib/antivirus",
	"calendartype":       "calendar/type",
	"logstore":           "admin/tool/log/store",
	"paygw":              "payment/gateway",
	"mlbackend":          "lib/mlbackend",
	"search":             "search/engine",
}

// IsWithinMoodle reports whether absolutePath is located inside moodlePath (or equal to it).
// Unlike a naive string-prefix check, this normalizes both paths first (filepath.Abs, which also
// Cleans), so an absolutePath containing ".." segments (e.g. moodlePath+"/local/x/../../..") that
// lexically escapes moodlePath is reported as outside it.
func IsWithinMoodle(absolutePath, moodlePath string) bool {
	resolvedMoodle, err := filepath.Abs(moodlePath)
	if err != nil {
		return false
	}
	resolvedTarget, err := filepath.Abs(absolutePath)
	if err != nil {
		return false
	}

	// When both paths exist on disk, resolve symlinks before comparing, so a symlink inside the
	// Moodle root pointing outside it (e.g. local/evil -> /etc) is detected. Only applied when
	// EvalSymlinks succeeds for BOTH sides: a target that doesn't exist yet falls through to the
	// plain lexical comparison, and resolving only one side could mismatch when moodlePath itself
	// sits behind a symlink.
	if em, errM := filepath.EvalSymlinks(resolvedMoodle); errM == nil {
		if et, errT := filepath.EvalSymlinks(resolvedTarget); errT == nil {
			resolvedMoodle, resolvedTarget = em, et
		}
	}

	rel, err := filepath.Rel(resolvedMoodle, resolvedTarget)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// ResolvePluginPath resolves a plugin identifier — an absolute path, a moodle-root-relative path
// containing "/", or a component string ("type_name") — to an absolute path on disk. Returns
// ("", false) if the identifier can't be resolved to an existing path. The returned path is
// always filepath.Clean-ed, so it can be passed straight into IsWithinMoodle.
func ResolvePluginPath(identifier, moodlePath string) (string, bool) {
	if filepath.IsAbs(identifier) {
		return existsOrEmpty(filepath.Clean(identifier))
	}
	if strings.Contains(identifier, "/") {
		return existsOrEmpty(filepath.Join(moodlePath, identifier))
	}
	if idx := strings.Index(identifier, "_"); idx != -1 {
		typ := identifier[:idx]
		name := identifier[idx+1:]
		dir, ok := PluginTypeToDir[typ]
		if !ok {
			dir = typ
		}
		return existsOrEmpty(filepath.Join(moodlePath, dir, name))
	}
	return "", false
}

func existsOrEmpty(path string) (string, bool) {
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	return path, true
}
