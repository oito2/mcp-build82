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
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/moodletype"
	"github.com/oito2/mcp-build82/internal/phpcs"
)

// defaultCodingStyleLimit is the default maximum number of messages check_coding_style returns.
const defaultCodingStyleLimit = 200

// CheckCodingStyleInput is the input of the check_coding_style tool.
type CheckCodingStyleInput struct {
	Plugin   string   `json:"plugin" jsonschema:"Component (e.g. 'local_myplugin'), path relative to the Moodle root (e.g. 'local/myplugin'), or absolute path"`
	Severity string   `json:"severity,omitempty" jsonschema:"'error' (default) reports errors only; 'warning' also reports warnings"`
	Files    []string `json:"files,omitempty" jsonschema:"Files or directories to check, relative to the plugin directory (default: the whole plugin)"`
	Limit    int      `json:"limit,omitempty" jsonschema:"Maximum number of messages returned (default: 200)"`
	Format   Format   `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

// CodingStyleMessage is one coding-style violation.
type CodingStyleMessage struct {
	Line     int    `json:"line" jsonschema:"Line of the violation"`
	Column   int    `json:"column" jsonschema:"Column of the violation"`
	Severity string `json:"severity" jsonschema:"error or warning"`
	Source   string `json:"source" jsonschema:"phpcs sniff that reported it, such as moodle.Commenting.MissingDocblock.Function"`
	Message  string `json:"message" jsonschema:"Description of the violation"`
	Fixable  bool   `json:"fixable,omitempty" jsonschema:"Whether phpcbf can fix it automatically"`
}

// CodingStyleFile is the violations of one file.
type CodingStyleFile struct {
	File     string               `json:"file" jsonschema:"File path relative to the plugin directory"`
	Errors   int                  `json:"errors" jsonschema:"Number of errors in the file"`
	Warnings int                  `json:"warnings" jsonschema:"Number of warnings in the file (0 when severity is error)"`
	Messages []CodingStyleMessage `json:"messages" jsonschema:"Violations of the file returned within the limit, by line and column"`
}

// CheckCodingStyleOutput is the structured output of check_coding_style.
type CheckCodingStyleOutput struct {
	Component    string            `json:"component" jsonschema:"Frankenstyle component of the plugin"`
	Path         string            `json:"path" jsonschema:"Plugin directory relative to the Moodle root"`
	Standard     string            `json:"standard" jsonschema:"Coding standard used (moodle)"`
	Severity     string            `json:"severity" jsonschema:"Lowest severity reported: error or warning"`
	Errors       int               `json:"errors" jsonschema:"Total number of errors"`
	Warnings     int               `json:"warnings" jsonschema:"Total number of warnings (0 when severity is error)"`
	Fixable      int               `json:"fixable" jsonschema:"Number of violations phpcbf can fix automatically"`
	FilesChecked int               `json:"files_checked" jsonschema:"Number of PHP files checked"`
	Files        []CodingStyleFile `json:"files" jsonschema:"Files with violations, sorted by path"`
	Returned     int               `json:"returned" jsonschema:"Number of messages returned"`
	Truncated    bool              `json:"truncated,omitempty" jsonschema:"Whether messages were left out because of the limit"`
}

// RegisterCodingStyleTool registers the check_coding_style tool on `server`.
func RegisterCodingStyleTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "check_coding_style",
		InputSchema: inputSchemaFor[CheckCodingStyleInput](),
		Annotations: toolAnnotations("Check Coding Style", true, false, true, false),
		Description: "Checks a Moodle plugin's PHP files against the Moodle coding standard with phpcs " +
			"(PHP_CodeSniffer with moodlehq/moodle-cs, an optional dependency) and returns the violations " +
			"per file: line, column, severity, sniff and message. Errors only by default; severity " +
			"'warning' adds warnings. Skips .build82, vendor and node_modules. Fails with installation " +
			"instructions when phpcs or the moodle standard is missing.",
	}, withRecover(handleCheckCodingStyle))
}

// handleCheckCodingStyle runs phpcs with the moodle standard on the plugin named by `in.Plugin`
// (or on `in.Files` inside it) and returns its violations, at most `in.Limit` messages. It
// returns an error, with no structured output, for a missing configuration, a rejected plugin or
// file, an invalid argument, a missing phpcs or moodle standard, a timeout, a phpcs failure, or a
// report that checked no file.
func handleCheckCodingStyle(ctx context.Context, req *mcp.CallToolRequest, in CheckCodingStyleInput) (*mcp.CallToolResult, CheckCodingStyleOutput, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, CheckCodingStyleOutput{}, err
	}
	rp, errResult := resolveAndValidatePlugin(in.Plugin, cfg.MoodlePath)
	if errResult != nil {
		return nil, CheckCodingStyleOutput{}, resultError(errResult)
	}

	severity := in.Severity
	switch severity {
	case "":
		severity = "error"
	case "error", "warning":
	default:
		return nil, CheckCodingStyleOutput{}, toolError(fmt.Sprintf("❌ severity must be \"error\" or \"warning\", got %q.", in.Severity))
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultCodingStyleLimit
	}
	paths, err := codingStylePaths(rp.Path, in.Files)
	if err != nil {
		return nil, CheckCodingStyleOutput{}, toolError("❌ " + err.Error())
	}

	switch s := phpcs.Check(ctx); s.State {
	case phpcs.Missing:
		return nil, CheckCodingStyleOutput{}, toolError("❌ phpcs was not found on PATH. To check the Moodle coding style, " + phpcs.InstallHint + ".")
	case phpcs.Unusable:
		return nil, CheckCodingStyleOutput{}, toolError("❌ phpcs was found but does not work: " + s.Detail)
	case phpcs.NoStandard:
		return nil, CheckCodingStyleOutput{}, toolError("❌ phpcs was found, but the moodle coding standard is not installed. " +
			"Run `composer global require moodlehq/moodle-cs`, or point phpcs at an existing copy with " +
			"`phpcs --config-set installed_paths <path to moodle-cs>`.")
	}

	report, err := phpcs.Run(ctx, rp.Path, paths, severity == "warning")
	if err != nil {
		return nil, CheckCodingStyleOutput{}, toolError("❌ " + err.Error())
	}
	if len(report.Files) == 0 {
		// An empty report would read as "no violations" although nothing was checked.
		return nil, CheckCodingStyleOutput{}, toolError("❌ phpcs checked no PHP file, so there is no result. " +
			"Check that the selected files include PHP files and, when phpcs runs through a wrapper " +
			"(for example in a container), that it sees the plugin directory at the same path.")
	}

	out := buildCodingStyleOutput(report, rp.Path, relativeToMoodle(cfg.MoodlePath, rp.Path), limit)
	out.Component, out.Path, out.Severity = rp.Info.Component, relativeToMoodle(cfg.MoodlePath, rp.Path), severity
	return structuredResult(in.Format, false, renderCodingStyleReport(out), out)
}

// codingStylePaths returns the absolute paths to check: the plugin directory `pluginPath` when
// `files` is empty, otherwise each of `files`, which must be relative paths naming an existing
// file or directory inside the plugin.
func codingStylePaths(pluginPath string, files []string) ([]string, error) {
	if len(files) == 0 {
		return []string{pluginPath}, nil
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		if f == "" || filepath.IsAbs(f) || filepath.VolumeName(f) != "" {
			return nil, fmt.Errorf("files must be paths relative to the plugin directory, got %q", f)
		}
		full := filepath.Join(pluginPath, filepath.FromSlash(f))
		if !moodletype.IsWithinMoodle(full, pluginPath) {
			return nil, fmt.Errorf("%q is outside the plugin directory", f)
		}
		if _, err := os.Stat(full); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("%q does not exist in the plugin", f)
			}
			return nil, fmt.Errorf("cannot access %q: %v", f, err)
		}
		paths = append(paths, full)
	}
	return paths, nil
}

// buildCodingStyleOutput converts the phpcs `report` into the structured output: totals, and the
// files with violations sorted by path (relative to the plugin, see pluginRelative, using the
// plugin directory `pluginPath` and its Moodle-relative path `pluginRel`), each with its messages
// sorted by line and column. At most `limit` messages are kept overall; Truncated tells when some
// were left out.
func buildCodingStyleOutput(report *phpcs.Report, pluginPath, pluginRel string, limit int) CheckCodingStyleOutput {
	out := CheckCodingStyleOutput{
		Standard: phpcs.Standard, Errors: report.Totals.Errors, Warnings: report.Totals.Warnings,
		Fixable: report.Totals.Fixable, FilesChecked: len(report.Files), Files: []CodingStyleFile{},
	}
	for reported, fr := range report.Files {
		if fr == nil || len(fr.Messages) == 0 {
			continue
		}
		f := CodingStyleFile{File: pluginRelative(reported, pluginPath, pluginRel), Errors: fr.Errors, Warnings: fr.Warnings}
		for _, m := range fr.Messages {
			f.Messages = append(f.Messages, CodingStyleMessage{
				Line: m.Line, Column: m.Column, Severity: strings.ToLower(m.Type),
				Source: m.Source, Message: m.Message, Fixable: m.Fixable,
			})
		}
		sort.SliceStable(f.Messages, func(i, j int) bool {
			a, b := f.Messages[i], f.Messages[j]
			return a.Line < b.Line || (a.Line == b.Line && a.Column < b.Column)
		})
		out.Files = append(out.Files, f)
	}
	sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].File < out.Files[j].File })

	remaining := limit
	for i := range out.Files {
		msgs := out.Files[i].Messages
		if len(msgs) > remaining {
			msgs = msgs[:remaining]
			out.Truncated = true
		}
		out.Files[i].Messages = msgs
		out.Returned += len(msgs)
		remaining -= len(msgs)
	}
	if out.Truncated {
		kept := out.Files[:0]
		for _, f := range out.Files {
			if len(f.Messages) > 0 {
				kept = append(kept, f)
			}
		}
		out.Files = kept
	}
	return out
}

// pluginRelative returns the path phpcs reported, `reported`, relative to the plugin directory
// `pluginPath`, with forward slashes. When phpcs runs elsewhere (for example in a container
// through a wrapper script) and reports another absolute path, the part after the last occurrence
// of the plugin's Moodle-relative path `pluginRel` is used; failing that, the file name.
func pluginRelative(reported, pluginPath, pluginRel string) string {
	if rel, err := filepath.Rel(pluginPath, reported); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
		return filepath.ToSlash(rel)
	}
	slashed := strings.ReplaceAll(reported, `\`, "/")
	marker := "/" + strings.Trim(filepath.ToSlash(pluginRel), "/") + "/"
	if i := strings.LastIndex(slashed, marker); i >= 0 {
		return slashed[i+len(marker):]
	}
	return slashed[strings.LastIndex(slashed, "/")+1:]
}

// renderCodingStyleReport renders the Markdown report of `out`: the totals, then each file's
// violations as a table.
func renderCodingStyleReport(out CheckCodingStyleOutput) string {
	var b strings.Builder
	what := "errors"
	if out.Severity == "warning" {
		what = "errors and warnings"
	}
	fmt.Fprintf(&b, "## Coding style: %s (%s standard, %s)\n\nPath: %s\n\n", out.Component, out.Standard, what, out.Path)
	fmt.Fprintf(&b, "%d error(s), %d warning(s) in %d file(s) checked; %d fixable with phpcbf.\n\n",
		out.Errors, out.Warnings, out.FilesChecked, out.Fixable)
	if out.Errors == 0 && out.Warnings == 0 {
		b.WriteString("✅ No violations found.\n")
		return b.String()
	}
	for _, f := range out.Files {
		fmt.Fprintf(&b, "### %s (%d error(s), %d warning(s))\n\n| Line | Col | Severity | Message | Sniff |\n|---|---|---|---|---|\n", f.File, f.Errors, f.Warnings)
		for _, m := range f.Messages {
			fmt.Fprintf(&b, "| %d | %d | %s | %s | `%s` |\n", m.Line, m.Column, m.Severity,
				strings.ReplaceAll(m.Message, "|", `\|`), m.Source)
		}
		b.WriteString("\n")
	}
	if out.Truncated {
		fmt.Fprintf(&b, "⚠ Showing %d of %d message(s); raise `limit` or check fewer `files` to see the rest.\n", out.Returned, out.Errors+out.Warnings)
	}
	return b.String()
}
