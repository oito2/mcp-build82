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

package moodletype

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// PluginTypeToDir maps a Moodle plugin type (the part of a component name before the first
// underscore) to its directory relative to the Moodle root.
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
	"portfolio":          "portfolio",
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
	"aiplacement":        "ai/placement",
	"aiprovider":         "ai/provider",
	"cachelock":          "cache/locks",
	"cachestore":         "cache/stores",
	"communication":      "communication/provider",
	"contenttype":        "contentbank/contenttype",
	"coursereport":       "course/report",
	"customfield":        "customfield/field",
	"dataformat":         "dataformat",
	"fileconverter":      "files/converter",
	"gradepenalty":       "grade/penalty",
	"gradingform":        "grade/grading/form",
	"h5plib":             "h5p/h5plib",
	"media":              "media/player",
	"message":            "message/output",
	"mnetservice":        "mnet/service",
	"qbank":              "question/bank",
	"qbehaviour":         "question/behaviour",
	"qformat":            "question/format",
	"smsgateway":         "sms/gateway",
	"tiny":               "lib/editor/tiny/plugins",
	"webservice":         "webservice",
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

	// When the Moodle root exists, resolve symlinks on both sides before comparing, so a symlink
	// inside the root pointing outside it (e.g. local/evil -> /etc) is detected. A target that does
	// not exist yet is resolved through its deepest existing ancestor (evalExisting), so a new path
	// under a symlinked directory (local -> /outside, then local/newplugin) is detected too. When
	// the root does not exist, both sides stay lexical, since resolving only one side could
	// mismatch when moodlePath itself sits behind a symlink.
	if em, errM := filepath.EvalSymlinks(resolvedMoodle); errM == nil {
		et, ok := evalExisting(resolvedTarget)
		if !ok {
			return false
		}
		resolvedMoodle, resolvedTarget = em, et
	}

	rel, err := filepath.Rel(resolvedMoodle, resolvedTarget)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// evalExisting returns `p` with the symbolic links of its deepest existing ancestor resolved and the
// missing elements re-appended. It returns false when an element exists but cannot be resolved,
// such as a dangling symbolic link, whose eventual target cannot be checked.
func evalExisting(p string) (string, bool) {
	rest := ""
	for {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(r, rest), true
		}
		if _, err := os.Lstat(p); err == nil {
			return "", false
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Join(p, rest), true
		}
		rest, p = filepath.Join(filepath.Base(p), rest), parent
	}
}

// LangFileName returns the base name, without ".php", of the English language file of the plugin
// `component`: the module name for an activity module ("mod_forum" → "forum", as in
// mod/forum/lang/en/forum.php), the component itself for every other plugin type.
func LangFileName(component string) string {
	if name, ok := strings.CutPrefix(component, "mod_"); ok {
		return name
	}
	return component
}

// ComponentNames returns every spelling Moodle accepts for the plugin `component` as a
// get_string() component and as the prefix of its lib.php callbacks: an activity module is known
// both as "mod_forum" and as "forum"; every other plugin only by its component.
func ComponentNames(component string) []string {
	if name, ok := strings.CutPrefix(component, "mod_"); ok {
		return []string{component, name}
	}
	return []string{component}
}

// IsRoot reports whether `path` is the Moodle root `moodlePath` itself, comparing absolute paths
// with symbolic links resolved when both exist.
func IsRoot(path, moodlePath string) bool {
	a, errA := filepath.Abs(path)
	b, errB := filepath.Abs(moodlePath)
	if errA != nil || errB != nil {
		return false
	}
	if ra, err := filepath.EvalSymlinks(a); err == nil {
		if rb, err := filepath.EvalSymlinks(b); err == nil {
			a, b = ra, rb
		}
	}
	return a == b
}

// componentPartsPattern matches the type and name of a component identifier: a lowercase type, an
// underscore, then lowercase letters, digits and underscores. It keeps "." and ".." (e.g.
// "mod_..") from being joined into a path.
var componentPartsPattern = regexp.MustCompile(`^([a-z][a-z0-9]*)_([a-z0-9_]+)$`)

// ResolvePluginPath resolves `identifier` — an absolute path, a path relative to `moodlePath`
// containing "/", or a component string ("type_name", see componentPartsPattern) — to a path on
// disk. A component whose type is missing from PluginTypeToDir uses the type itself as the
// directory. Returns ("", false) if the identifier can't be resolved to an existing path or is a
// malformed component. The returned path is cleaned, so it can
// be passed straight into IsWithinMoodle.
func ResolvePluginPath(identifier, moodlePath string) (string, bool) {
	if filepath.IsAbs(identifier) {
		return existsOrEmpty(filepath.Clean(identifier))
	}
	if strings.Contains(identifier, "/") {
		return existsOrEmpty(filepath.Join(moodlePath, identifier))
	}
	if m := componentPartsPattern.FindStringSubmatch(identifier); m != nil {
		typ, name := m[1], m[2]
		dir, ok := PluginTypeToDir[typ]
		if !ok {
			dir = typ
		}
		return existsOrEmpty(filepath.Join(moodlePath, dir, name))
	}
	return "", false
}

// existsOrEmpty returns path and true when it exists on disk, or ("", false) otherwise.
func existsOrEmpty(path string) (string, bool) {
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	return path, true
}

// upgradeVersionPattern matches a Moodle version number as an upgrade step compares it: ten digits
// (YYYYMMDDXX), optionally followed by a decimal part.
var upgradeVersionPattern = regexp.MustCompile(`^\d{10}(?:\.\d+)?$`)

// IsUpgradeVersion reports whether `s` is a version number an `if ($oldversion < V)` upgrade step
// can name: ten digits, optionally with a decimal part (2024010100 or 2024010100.01).
func IsUpgradeVersion(s string) bool {
	return upgradeVersionPattern.MatchString(s)
}

// UpgradeVersionLess reports whether the version number `a` is lower than `b`, comparing them as
// numbers so a decimal part counts (2024010100 < 2024010100.01).
func UpgradeVersionLess(a, b string) bool {
	va, _ := strconv.ParseFloat(a, 64)
	vb, _ := strconv.ParseFloat(b, 64)
	return va < vb
}
