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

package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/fsutil"
)

// maxBumpFileSize is the largest version.php or db/upgrade.php bump_plugin_version reads.
const maxBumpFileSize = 1 << 20

// Patterns used by bump_plugin_version.
var (
	// versionAssignPattern matches the start of a $plugin->version, ->release or ->maturity
	// assignment, up to the value; group 1 is the property name.
	versionAssignPattern = regexp.MustCompile(`\$plugin\s*->\s*(version|release|maturity)\s*=\s*`)
	// versionLiteralPattern matches a numeric version literal such as 2024010100 or 2024010100.01.
	versionLiteralPattern = regexp.MustCompile(`^\d+(\.\d+)?$`)
	// newVersionPattern matches an accepted new version: exactly ten digits (YYYYMMDDXX).
	newVersionPattern = regexp.MustCompile(`^\d{10}$`)
	// releaseLiteralPattern matches a single-quoted string, a double-quoted string without
	// variable interpolation, or a number.
	releaseLiteralPattern = regexp.MustCompile(`^(?:'(?:[^'\\]|\\.)*'|"(?:[^"\\$]|\\.)*"|\d+(?:\.\d+)?)$`)
	// maturityLiteralPattern matches a MATURITY_* constant.
	maturityLiteralPattern = regexp.MustCompile(`^MATURITY_[A-Z]+$`)
	// upgradeFunctionPattern matches an xmldb_*_upgrade function declaration up to its opening
	// parenthesis; group 1 is the function name.
	upgradeFunctionPattern = regexp.MustCompile(`\bfunction\s+(xmldb_\w+_upgrade)\s*\(`)
	// finalReturnPattern matches a function body ending with `return true;`, the statement
	// starting its own line; group 1 is the indentation before `return`.
	finalReturnPattern = regexp.MustCompile(`(?i)\n([ \t]*)return\s+true\s*;\s*$`)
)

// BumpPluginVersionInput is the input of the bump_plugin_version tool.
type BumpPluginVersionInput struct {
	Plugin             string `json:"plugin" jsonschema:"Component (e.g. 'local_myplugin'), path relative to the Moodle root (e.g. 'local/myplugin'), or absolute path"`
	Version            string `json:"version,omitempty" jsonschema:"New $plugin->version, ten digits (YYYYMMDDXX) greater than the current one (default: today as YYYYMMDD00, or the current version + 1 when that is not greater)"`
	Release            string `json:"release,omitempty" jsonschema:"New $plugin->release string (default: unchanged)"`
	Maturity           string `json:"maturity,omitempty" jsonschema:"New $plugin->maturity constant: MATURITY_ALPHA, MATURITY_BETA, MATURITY_RC or MATURITY_STABLE (default: unchanged)"`
	UpgradeStep        bool   `json:"upgrade_step,omitempty" jsonschema:"Also add an upgrade step for the new version to db/upgrade.php, creating the file when missing (default: false)"`
	UpgradeDescription string `json:"upgrade_description,omitempty" jsonschema:"One-line description written as a comment in the generated upgrade step"`
	Format             Format `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

// BumpPluginVersionOutput is the structured output of bump_plugin_version.
type BumpPluginVersionOutput struct {
	Component          string   `json:"component" jsonschema:"Frankenstyle component of the plugin"`
	Path               string   `json:"path" jsonschema:"Plugin directory relative to the Moodle root"`
	OldVersion         string   `json:"old_version" jsonschema:"$plugin->version before the call"`
	NewVersion         string   `json:"new_version" jsonschema:"$plugin->version written by the call"`
	Release            string   `json:"release,omitempty" jsonschema:"$plugin->release written by the call, when one was given"`
	Maturity           string   `json:"maturity,omitempty" jsonschema:"$plugin->maturity written by the call, when one was given"`
	FilesChanged       []string `json:"files_changed" jsonschema:"Files written, relative to the plugin directory"`
	UpgradeFileCreated bool     `json:"upgrade_file_created,omitempty" jsonschema:"Whether db/upgrade.php did not exist and was created"`
	UpgradeStep        string   `json:"upgrade_step,omitempty" jsonschema:"PHP code of the upgrade step added to db/upgrade.php"`
}

// RegisterBumpVersionTool registers the bump_plugin_version tool on `server`.
func RegisterBumpVersionTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "bump_plugin_version",
		InputSchema: inputSchemaFor[BumpPluginVersionInput](),
		Annotations: toolAnnotations("Bump Plugin Version", false, true, false, false),
		Description: "Raises a Moodle plugin's $plugin->version in version.php (and optionally its release and " +
			"maturity), rewriting only those assignments. With upgrade_step it also adds an " +
			"`if ($oldversion < V)` block with the savepoint matching the plugin type to db/upgrade.php, " +
			"creating the file from the standard template when missing. Refuses, without writing anything, " +
			"when an assignment is missing, duplicated or not a literal, the new version is not greater " +
			"than the current one, or the upgrade function or its final `return true;` cannot be located.",
	}, withRecover(handleBumpPluginVersion))
}

// bumpNow returns the current time used for the default version. It is a variable so tests can
// fix the date.
var bumpNow = time.Now

// writeBumpFile writes one file atomically. It defaults to fsutil.WriteAtomic and is a variable so
// tests can inject a write failure.
var writeBumpFile = fsutil.WriteAtomic

// handleBumpPluginVersion raises the version of the plugin named by `in.Plugin`, as described on
// the tool. Every change is computed and validated before anything is written; db/upgrade.php is
// written first and restored (or removed, when it was created) if writing version.php then fails.
// It returns an error, with no structured output, for a missing configuration, a rejected plugin,
// an invalid argument, a file that cannot be edited safely, or a write failure.
func handleBumpPluginVersion(ctx context.Context, req *mcp.CallToolRequest, in BumpPluginVersionInput) (*mcp.CallToolResult, BumpPluginVersionOutput, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, BumpPluginVersionOutput{}, err
	}
	rp, errResult := resolveAndValidatePlugin(in.Plugin, cfg.MoodlePath)
	if errResult != nil {
		return nil, BumpPluginVersionOutput{}, resultError(errResult)
	}

	if in.Maturity != "" {
		if _, ok := validMoodleMaturities[in.Maturity]; !ok {
			return nil, BumpPluginVersionOutput{}, toolError(fmt.Sprintf("❌ Unrecognized maturity %q (expected MATURITY_ALPHA, MATURITY_BETA, MATURITY_RC, or MATURITY_STABLE).", in.Maturity))
		}
	}
	if strings.ContainsFunc(in.Release, isControlRune) {
		return nil, BumpPluginVersionOutput{}, toolError("❌ release must be a single line without control characters.")
	}
	if strings.ContainsFunc(in.UpgradeDescription, isControlRune) || strings.Contains(in.UpgradeDescription, "?>") {
		return nil, BumpPluginVersionOutput{}, toolError("❌ upgrade_description must be a single line without control characters or \"?>\".")
	}

	versionPath := filepath.Join(rp.Path, "version.php")
	versionSrc, err := fsutil.ReadRegular(versionPath, maxBumpFileSize)
	if err != nil {
		return nil, BumpPluginVersionOutput{}, toolError("❌ Cannot read version.php: " + err.Error())
	}
	edit, err := planVersionEdit(string(versionSrc), in, bumpNow())
	if err != nil {
		return nil, BumpPluginVersionOutput{}, toolError("❌ " + err.Error())
	}

	component := rp.Info.Component
	out := BumpPluginVersionOutput{
		Component: component, Path: relativeToMoodle(cfg.MoodlePath, rp.Path),
		OldVersion: edit.oldVersion, NewVersion: edit.newVersion,
		Release: in.Release, Maturity: in.Maturity,
	}

	var up *upgradeEdit
	if in.UpgradeStep {
		up, err = planUpgradeEdit(rp.Path, component, edit.newVersion, in.UpgradeDescription)
		if err != nil {
			return nil, BumpPluginVersionOutput{}, toolError("❌ " + err.Error())
		}
		out.UpgradeFileCreated = up.created
		out.UpgradeStep = up.step
	}

	if up != nil {
		if err := writeBumpFile(up.path, []byte(up.content), up.perm); err != nil {
			return nil, BumpPluginVersionOutput{}, toolError("❌ Failed to write db/upgrade.php: " + err.Error())
		}
		out.FilesChanged = append(out.FilesChanged, "db/upgrade.php")
	}
	if err := writeBumpFile(versionPath, []byte(edit.content), fileMode(versionPath)); err != nil {
		msg := "❌ Failed to write version.php: " + err.Error()
		if up != nil {
			if rbErr := up.rollback(); rbErr != nil {
				msg += "\n\nRestoring db/upgrade.php also failed: " + rbErr.Error()
			} else {
				msg += "\n\ndb/upgrade.php was restored; nothing was changed."
			}
		}
		return nil, BumpPluginVersionOutput{}, toolError(msg)
	}
	out.FilesChanged = append(out.FilesChanged, "version.php")
	sort.Strings(out.FilesChanged)

	return structuredResult(in.Format, false, renderBumpReport(out), out)
}

// isControlRune reports whether `r` is a control character.
func isControlRune(r rune) bool {
	return r < 0x20 || r == 0x7f
}

// fileMode returns the permission bits of the file at `path`, or 0o644 when it cannot be read.
func fileMode(path string) os.FileMode {
	if info, err := os.Stat(path); err == nil {
		return info.Mode().Perm()
	}
	return 0o644
}

// versionEdit is the planned rewrite of version.php: the current and new version numbers and the
// new file content.
type versionEdit struct {
	oldVersion, newVersion string
	content                string
}

// phpAssignment is one $plugin->{version,release,maturity} assignment: its value literal and the
// byte span of that literal in the file.
type phpAssignment struct {
	value      string
	start, end int
}

// planVersionEdit computes the new content of the version.php source `src` for `in`, at time
// `now` for the default version. Only the literal of each assignment it changes is replaced; every
// other byte is kept. It returns an error when an assignment it needs is missing, duplicated or not
// a literal, or when the new version is invalid or not greater than the current one.
func planVersionEdit(src string, in BumpPluginVersionInput, now time.Time) (versionEdit, error) {
	assigns := findPluginAssignments(src)

	version, err := singleAssignment(assigns, "version", versionLiteralPattern)
	if err != nil {
		return versionEdit{}, err
	}
	newVersion, err := nextVersion(version.value, in.Version, now)
	if err != nil {
		return versionEdit{}, err
	}

	type replacement struct {
		start, end int
		text       string
	}
	repls := []replacement{{version.start, version.end, newVersion}}
	if in.Release != "" {
		a, err := singleAssignment(assigns, "release", releaseLiteralPattern)
		if err != nil {
			return versionEdit{}, err
		}
		repls = append(repls, replacement{a.start, a.end, "'" + escapePhpSingleQuoted(in.Release) + "'"})
	}
	if in.Maturity != "" {
		a, err := singleAssignment(assigns, "maturity", maturityLiteralPattern)
		if err != nil {
			return versionEdit{}, err
		}
		repls = append(repls, replacement{a.start, a.end, in.Maturity})
	}

	sort.Slice(repls, func(i, j int) bool { return repls[i].start > repls[j].start })
	content := src
	for _, r := range repls {
		content = content[:r.start] + r.text + content[r.end:]
	}
	return versionEdit{oldVersion: version.value, newVersion: newVersion, content: content}, nil
}

// findPluginAssignments returns every $plugin->version, ->release and ->maturity assignment of the
// PHP source `src` outside comments and heredocs, keyed by property name, in source order.
func findPluginAssignments(src string) map[string][]phpAssignment {
	code := extractors.BlankCommentsAndHeredocs(src)
	assigns := map[string][]phpAssignment{}
	for _, m := range versionAssignPattern.FindAllStringSubmatchIndex(code, -1) {
		start := m[1]
		end := statementEnd(code, start)
		if end < 0 {
			continue
		}
		raw := src[start:end]
		value := strings.TrimRight(raw, " \t\r\n")
		name := code[m[2]:m[3]]
		assigns[name] = append(assigns[name], phpAssignment{value: value, start: start, end: start + len(value)})
	}
	return assigns
}

// statementEnd returns the offset of the `;` ending the PHP statement that continues at `from` in
// `code`, skipping quoted strings, or -1 when there is none.
func statementEnd(code string, from int) int {
	var quote byte
	for i := from; i < len(code); i++ {
		c := code[i]
		switch {
		case quote != 0:
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == ';':
			return i
		}
	}
	return -1
}

// singleAssignment returns the only $plugin->`name` assignment in `assigns`. It returns an error
// when there is none, more than one, or its value does not match `literal`.
func singleAssignment(assigns map[string][]phpAssignment, name string, literal *regexp.Regexp) (phpAssignment, error) {
	list := assigns[name]
	switch {
	case len(list) == 0:
		return phpAssignment{}, fmt.Errorf("version.php has no $plugin->%s assignment; add it first", name)
	case len(list) > 1:
		return phpAssignment{}, fmt.Errorf("version.php assigns $plugin->%s %d times; keep a single assignment", name, len(list))
	case !literal.MatchString(list[0].value):
		return phpAssignment{}, fmt.Errorf("version.php assigns $plugin->%s a value that is not a plain literal (%s); edit it by hand", name, list[0].value)
	}
	return list[0], nil
}

// nextVersion returns the version that replaces `current`: `requested` when given, after checking
// it has ten digits and is greater than `current`; otherwise today's date (from `now`) as
// YYYYMMDD00, or the integer part of `current` + 1 when that is not greater than `current`.
func nextVersion(current, requested string, now time.Time) (string, error) {
	cur, err := strconv.ParseFloat(current, 64)
	if err != nil {
		return "", fmt.Errorf("version.php has an unreadable $plugin->version %q", current)
	}
	if requested != "" {
		if !newVersionPattern.MatchString(requested) {
			return "", fmt.Errorf("version must have exactly ten digits (YYYYMMDDXX), got %q", requested)
		}
		if v, _ := strconv.ParseFloat(requested, 64); v <= cur {
			return "", fmt.Errorf("version %s is not greater than the current version %s", requested, current)
		}
		return requested, nil
	}
	today, _ := strconv.ParseInt(now.Format("20060102")+"00", 10, 64)
	next := today
	if float64(today) <= cur {
		intPart, _, _ := strings.Cut(current, ".")
		n, err := strconv.ParseInt(intPart, 10, 64)
		if err != nil {
			return "", fmt.Errorf("version.php has an unreadable $plugin->version %q", current)
		}
		next = n + 1
	}
	s := strconv.FormatInt(next, 10)
	if !newVersionPattern.MatchString(s) {
		return "", fmt.Errorf("the next version after %s would be %s, which does not have ten digits; pass version explicitly", current, s)
	}
	return s, nil
}

// upgradeEdit is the planned change to db/upgrade.php: its path, new content and mode, the
// generated step, whether the file is created, and what to restore if a later write fails.
type upgradeEdit struct {
	path        string
	content     string
	perm        os.FileMode
	step        string
	created     bool
	original    []byte
	createdDirs bool
}

// rollback undoes the write of db/upgrade.php: it writes the original content back, or removes
// the created file (and the db directory, when this call created it and it is empty).
func (u *upgradeEdit) rollback() error {
	if !u.created {
		return writeBumpFile(u.path, u.original, u.perm)
	}
	if err := os.Remove(u.path); err != nil {
		return err
	}
	if u.createdDirs {
		_ = os.Remove(filepath.Dir(u.path))
	}
	return nil
}

// planUpgradeEdit computes the db/upgrade.php of the plugin at `pluginPath` (component
// `component`) with a step for `version`, commented with `description`, inserted before the final
// `return true;` of the upgrade function. A missing file is created from the standard template. It
// returns an error when the component has no type prefix, the file cannot be read, a step for this
// version or a later one already exists, or the function or its final `return true;` cannot be
// located.
func planUpgradeEdit(pluginPath, component, version, description string) (*upgradeEdit, error) {
	pluginType, name, ok := strings.Cut(component, "_")
	if !ok || name == "" {
		return nil, fmt.Errorf("cannot derive the plugin type and name from component %q", component)
	}
	path := filepath.Join(pluginPath, "db", "upgrade.php")
	u := &upgradeEdit{path: path, perm: 0o644}

	src, err := fsutil.ReadRegular(path, maxBumpFileSize)
	switch {
	case errors.Is(err, os.ErrNotExist):
		u.created = true
		_, statErr := os.Stat(filepath.Dir(path))
		u.createdDirs = errors.Is(statErr, os.ErrNotExist)
		src = []byte(upgradePhpTemplate(component, upgradeFunctionName(pluginType, name, component)))
	case err != nil:
		return nil, fmt.Errorf("cannot read db/upgrade.php: %v", err)
	default:
		u.original = src
		u.perm = fileMode(path)
		if ops := extractors.ParseUpgradeOperations(pluginPath); ops != nil {
			newV, _ := strconv.ParseFloat(version, 64)
			for _, b := range ops.Blocks {
				if v, err := strconv.ParseFloat(b.Version, 64); err == nil && v >= newV {
					return nil, fmt.Errorf("db/upgrade.php already has a step for version %s (line %d), not older than %s", b.Version, b.Line, version)
				}
			}
		}
	}

	content, step, err := insertUpgradeStep(string(src), upgradeFunctionName(pluginType, name, component), pluginType, name, version, description)
	if err != nil {
		return nil, err
	}
	u.content, u.step = content, step
	return u, nil
}

// upgradeFunctionName returns the name of the upgrade function Moodle calls for a plugin of type
// `pluginType` named `name` (component `component`): xmldb_{name}_upgrade for activity modules,
// xmldb_{component}_upgrade for every other type.
func upgradeFunctionName(pluginType, name, component string) string {
	if pluginType == "mod" {
		return "xmldb_" + name + "_upgrade"
	}
	return "xmldb_" + component + "_upgrade"
}

// upgradePhpTemplate renders a db/upgrade.php for `component` whose upgrade function `function`
// does nothing but `return true;`.
func upgradePhpTemplate(component, function string) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

/**
 * Upgrade steps for %s.
 *
 * @param int $oldversion The version the plugin is upgraded from.
 * @return bool
 */
function %s($oldversion) {
    global $DB;
    $dbman = $DB->get_manager();

    return true;
}
`, component, function)
}

// insertUpgradeStep inserts into the db/upgrade.php source `src` an `if ($oldversion < version)`
// block, before the final `return true;` of the function named `function`, with a TODO body
// (preceded by `description` as a comment, when given) and the savepoint matching the plugin type
// `pluginType` and name `name`. The block uses the indentation of that `return` and the file's line
// endings. It returns the new source and the inserted block, or an error when the function, its
// body or a final `return true;` starting its own line cannot be located.
func insertUpgradeStep(src, function, pluginType, name, version, description string) (content, step string, err error) {
	code := extractors.BlankCommentsAndHeredocs(src)
	bodyOpen := -1
	for _, m := range upgradeFunctionPattern.FindAllStringSubmatchIndex(code, -1) {
		if code[m[2]:m[3]] == function {
			bodyOpen = strings.IndexByte(code[m[1]:], '{')
			if bodyOpen >= 0 {
				bodyOpen += m[1]
			}
			break
		}
	}
	if bodyOpen < 0 {
		return "", "", fmt.Errorf("db/upgrade.php does not define function %s(); add the step by hand", function)
	}
	bodyClose := matchingBrace(code, bodyOpen)
	if bodyClose < 0 {
		return "", "", fmt.Errorf("cannot find the end of %s() in db/upgrade.php; add the step by hand", function)
	}
	ret := finalReturnPattern.FindStringSubmatchIndex(code[bodyOpen+1 : bodyClose])
	if ret == nil {
		return "", "", fmt.Errorf("%s() in db/upgrade.php does not end with a `return true;` on its own line; add the step by hand", function)
	}
	lineStart := bodyOpen + 1 + ret[0] + 1
	indent := code[bodyOpen+1+ret[2] : bodyOpen+1+ret[3]]

	nl := "\n"
	if strings.Contains(src, "\r\n") {
		nl = "\r\n"
	}
	inner := indent + "    "
	if strings.Contains(indent, "\t") {
		inner = indent + "\t"
	}
	fn, args := savepointFor(pluginType, name)
	var b strings.Builder
	fmt.Fprintf(&b, "%sif ($oldversion < %s) {%s", indent, version, nl)
	if description != "" {
		fmt.Fprintf(&b, "%s// %s%s", inner, strings.TrimSpace(description), nl)
	}
	fmt.Fprintf(&b, "%s// TODO: implement this upgrade step.%s%s", inner, nl, nl)
	fmt.Fprintf(&b, "%s// %s_%s savepoint reached.%s", inner, pluginType, name, nl)
	fmt.Fprintf(&b, "%s%s(true, %s, %s);%s", inner, fn, version, quoteArgs(args), nl)
	fmt.Fprintf(&b, "%s}%s", indent, nl)
	step = b.String()
	return src[:lineStart] + step + nl + src[lineStart:], step, nil
}

// matchingBrace returns the offset of the `}` closing the `{` at `open` in `code`, skipping quoted
// strings, or -1 when it is not closed.
func matchingBrace(code string, open int) int {
	depth := 0
	var quote byte
	for i := open; i < len(code); i++ {
		c := code[i]
		switch {
		case quote != 0:
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// renderBumpReport renders the Markdown report of a bump described by `out`.
func renderBumpReport(out BumpPluginVersionOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "✅ Bumped %s from %s to %s.\n\nPath: %s\n\n", out.Component, out.OldVersion, out.NewVersion, out.Path)
	if out.Release != "" {
		fmt.Fprintf(&b, "Release: %s\n", out.Release)
	}
	if out.Maturity != "" {
		fmt.Fprintf(&b, "Maturity: %s\n", out.Maturity)
	}
	if out.Release != "" || out.Maturity != "" {
		b.WriteString("\n")
	}
	b.WriteString("Files changed:\n")
	for _, f := range out.FilesChanged {
		if f == "db/upgrade.php" && out.UpgradeFileCreated {
			f += " (created)"
		}
		fmt.Fprintf(&b, "- %s\n", f)
	}
	if out.UpgradeStep != "" {
		fmt.Fprintf(&b, "\n## Upgrade step\n\n```php\n%s```\n\nReplace the TODO with the schema or data changes, "+
			"then run `doctor` to check the step against db/install.xml.\n", strings.ReplaceAll(out.UpgradeStep, "\r\n", "\n"))
	}
	return b.String()
}
