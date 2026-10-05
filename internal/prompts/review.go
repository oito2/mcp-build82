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

package prompts

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/generators"
)

// ReviewFocus selects which section(s) of the review criteria checklist are included in the
// review_plugin prompt.
type ReviewFocus string

const (
	FocusAll         ReviewFocus = "all"
	FocusSecurity    ReviewFocus = "security"
	FocusPerformance ReviewFocus = "performance"
	FocusStandards   ReviewFocus = "standards"
	FocusDatabase    ReviewFocus = "database"
	FocusApis        ReviewFocus = "apis"
)

// RegisterReviewPrompt registers the review_plugin MCP prompt.
func RegisterReviewPrompt(server *mcp.Server) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "review_plugin",
		Description: "Generates a code-review prompt for a Moodle plugin, with a focus-area-specific criteria checklist and a few-shot example.",
		Arguments: []*mcp.PromptArgument{
			{Name: "plugin", Description: "Component, relative path, or absolute path", Required: true},
			{Name: "focus", Description: "'all' (default), 'security', 'performance', 'standards', 'database', or 'apis'"},
			{Name: "files", Description: "Comma-separated list of specific files to review"},
		},
	}, withRecoverPrompt(handleReviewPrompt))
}

var focusCriteria = map[ReviewFocus]string{
	FocusSecurity: "## Security\n\n" +
		"- **Authentication & Authorization**: `require_login()` called before any protected page; " +
		"`require_capability()`/`has_capability()` checked before any protected action.\n" +
		"- **Input Validation**: all input read via `required_param()`/`optional_param()` with explicit " +
		"`PARAM_*` types — never raw `$_GET`/`$_POST`/`$_REQUEST`; `sesskey()` checked before any " +
		"state-changing action.\n" +
		"- **Output Escaping**: all dynamic output passed through `s()`, `format_string()`, or " +
		"`format_text()` — never raw `echo`; all SQL parameterised, never string-concatenated.\n",
	FocusPerformance: "## Performance\n\n" +
		"- **Database Queries**: no queries inside loops (N+1); `get_records()` calls select only the " +
		"columns actually used; large result sets are paginated; the Moodle Universal Cache (MUC) is used " +
		"for expensive repeated lookups.\n" +
		"- **Memory & Tasks**: large files are streamed, not loaded fully into memory; scheduled tasks " +
		"handle their own timeout/retry behavior.\n",
	FocusStandards: "## Standards\n\n" +
		"- **Naming Conventions**: functions are `snake_case` and prefixed with the component name; " +
		"classes are `PascalCase` matching their namespace; constants are `UPPER_CASE`.\n" +
		"- **Code Style**: 4-space indentation; PHPDoc on every function/class; " +
		"`defined('MOODLE_INTERNAL') || die();` at the top of every non-entry-point file.\n",
	FocusDatabase: "## Database\n\n" +
		"- **Schema**: `db/install.xml` uses correct XMLDB column types, has appropriate indexes, and " +
		"declares foreign-key relationships where applicable.\n" +
		"- **Queries**: parameterised, using `MUST_EXIST`/`IGNORE_MISSING` deliberately; multi-step writes " +
		"wrapped in a DB transaction.\n" +
		"- **Upgrades**: `db/upgrade.php` present and version-gated; `version.php`'s version matches the " +
		"latest upgrade step.\n",
	FocusApis: "## API Usage\n\n" +
		"- **Moodle API Compliance**: uses the new event system (not legacy event handlers); scheduled " +
		"tasks extend `\\core\\task\\scheduled_task`; web services use " +
		"`external_function_parameters`/`external_value`.\n" +
		"- **Hooks & Callbacks**: legacy callbacks named correctly; deprecated callbacks migrated to " +
		"modern Hook API equivalents where a replacement exists.\n" +
		"- **Output & Forms**: renderers/Mustache templates used for output; `moodleform` pattern used for " +
		"forms.\n",
}

// validFocuses lists every accepted value of the review_plugin "focus" argument, in the order
// they are reported back to the client when an unknown value is given.
var validFocuses = []ReviewFocus{FocusAll, FocusSecurity, FocusPerformance, FocusStandards, FocusDatabase, FocusApis}

// parseFocus maps the raw "focus" argument to a ReviewFocus, defaulting to FocusAll when it is
// empty. An unknown value is rejected with an invalid-params error naming the accepted values,
// instead of silently rendering a prompt with no review criteria.
func parseFocus(raw string) (ReviewFocus, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return FocusAll, nil
	}
	accepted := make([]string, len(validFocuses))
	for i, f := range validFocuses {
		if string(f) == value {
			return f, nil
		}
		accepted[i] = string(f)
	}
	return "", &jsonrpc.Error{
		Code:    jsonrpc.CodeInvalidParams,
		Message: fmt.Sprintf("invalid focus %q: must be one of %s", value, strings.Join(accepted, ", ")),
	}
}

func getFocusCriteria(focus ReviewFocus) string {
	if focus == FocusAll {
		var b strings.Builder
		for _, f := range []ReviewFocus{FocusSecurity, FocusPerformance, FocusStandards, FocusDatabase, FocusApis} {
			b.WriteString(focusCriteria[f])
			b.WriteString("\n")
		}
		return b.String()
	}
	return focusCriteria[focus]
}

const reviewFewShotUser = "Review local_demo for security issues. Here's the relevant code:\n\n```php\n" +
	"function local_demo_view() {\n    $id = $_GET['id'];\n    $record = $DB->get_record_sql(\"SELECT * FROM {local_demo_records} WHERE id = $id\");\n    echo $record->name;\n}\n```"

const reviewFewShotAssistant = "## Review: local_demo (focus: security)\n\n" +
	"### Issue 1 — Critical\n**File:** lib.php\n**Problem:** Raw `$_GET['id']` used directly, with no " +
	"validation, and no `require_login()`/capability check before the action executes.\n**Fix:** Replace " +
	"with `$id = required_param('id', PARAM_INT);` and add `require_login($courseid); " +
	"require_capability('local/demo:view', $context);` at the top of the function.\n\n" +
	"### Issue 2 — Critical\n**File:** lib.php\n**Problem:** SQL built via string interpolation " +
	"(`\"...WHERE id = $id\"`) — a SQL injection vector even after PARAM_INT validation, since it bypasses " +
	"Moodle's parameterised query layer.\n**Fix:** Use " +
	"`$DB->get_record('local_demo_records', ['id' => $id])` instead of raw SQL.\n\n" +
	"### Issue 3 — High\n**File:** lib.php\n**Problem:** `echo $record->name` renders the value with no " +
	"escaping — a stored-XSS vector if `name` can contain user-supplied HTML.\n**Fix:** Use " +
	"`echo s($record->name);` or `format_string($record->name)` depending on whether HTML should be " +
	"preserved.\n\n## Summary\n\nOverall assessment: not safe to ship — all three issues are exploitable " +
	"as written. Top 3 priorities: (1) add auth/capability checks, (2) parameterise the query, (3) escape " +
	"output. No positive patterns to note in this snippet."

func handleReviewPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments
	if err := requireArgs(args, "plugin"); err != nil {
		return nil, err
	}
	plugin := args["plugin"]
	focus, err := parseFocus(args["focus"])
	if err != nil {
		return nil, err
	}
	files := truncateArg(args["files"])

	rc := resolvePluginForPrompt(plugin)
	pluginContext := rc.readPluginFileTruncated("PLUGIN_AI_CONTEXT.md", 3000)
	devRules := ""
	if rc.MoodlePath != "" {
		devRules = readFileTruncated(generators.GlobalOutputPath(rc.MoodlePath, "MOODLE_DEV_RULES.md"), 1500)
	}

	var b strings.Builder
	b.WriteString("## Code Review Request\n\n| Field | Value |\n|---|---|\n")
	for _, row := range [][2]string{
		{"Component", rc.Component}, {"Moodle version", rc.MoodleVersion},
		{"Type", rc.Type}, {"Version", rc.Version}, {"Focus", string(focus)},
	} {
		if row[1] != "" {
			fmt.Fprintf(&b, "| %s | %s |\n", row[0], row[1])
		}
	}
	b.WriteString("\n")

	if pluginContext != "" {
		fmt.Fprintf(&b, "### Plugin Context\n\n%s\n\n", pluginContext)
	}
	if devRules != "" {
		fmt.Fprintf(&b, "### Standards Reference\n\n%s\n\n", devRules)
	}

	fmt.Fprintf(&b, "### Review Criteria\n\n%s\n", getFocusCriteria(focus))

	b.WriteString("### Instructions\n\n")
	if files != "" {
		fmt.Fprintf(&b, "Review the following files: %s\n\n", files)
	} else {
		fmt.Fprintf(&b, "Perform a complete review of `%s`.\n\n", rc.Component)
	}

	b.WriteString("### Output Format\n\nFor each issue, use a numbered `## Issue N — {Severity}` heading " +
		"(Critical/High/Medium/Low) with **File**, **Problem**, and **Fix** fields, grouped Critical-first. " +
		"End with a `## Summary` covering overall assessment, top 3 priorities, and any positive patterns worth keeping.\n")

	return &mcp.GetPromptResult{
		Messages: []*mcp.PromptMessage{
			{Role: "user", Content: &mcp.TextContent{Text: reviewFewShotUser}},
			{Role: "assistant", Content: &mcp.TextContent{Text: reviewFewShotAssistant}},
			{Role: "user", Content: &mcp.TextContent{Text: b.String()}},
		},
	}, nil
}
