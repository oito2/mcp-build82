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

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterDebugPrompt registers the debug_plugin MCP prompt.
func RegisterDebugPrompt(server *mcp.Server) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "debug_plugin",
		Description: "Generates a debugging prompt for a Moodle plugin error, with keyword-matched Moodle-specific hints and a few-shot example.",
		Arguments: []*mcp.PromptArgument{
			{Name: "plugin", Description: "Component, relative path, or absolute path", Required: true},
			{Name: "error", Description: "Full error message / stack trace", Required: true},
			{Name: "context", Description: "When/how the error occurs"},
		},
	}, withRecoverPrompt(handleDebugPrompt))
}

type debugHintCategory struct {
	keywords []string
	hints    []string
}

var debugHintCategories = []debugHintCategory{
	{
		keywords: []string{"capability", "access denied"},
		hints: []string{
			"Check the capability is declared in `db/access.php` and its name starts with the plugin's " +
				"component prefix (e.g. `local/demo:view`).",
			"Verify the context level the capability check uses matches where it's assigned in `db/access.php`.",
			"Run `admin/cli/purge_caches.php` after any change to `db/access.php` — capability definitions are cached.",
		},
	},
	{
		keywords: []string{"table", "column", "sql"},
		hints: []string{
			"Run `php admin/cli/upgrade.php` to apply any pending `db/upgrade.php` steps.",
			"Verify the XMLDB syntax in `db/install.xml` — field/table names are case-sensitive and must match exactly what the code queries.",
			"Confirm `version.php`'s version number matches (or exceeds) the latest step in `db/upgrade.php`.",
		},
	},
	{
		keywords: []string{"class not found", "autoload", "namespace"},
		hints: []string{
			"Moodle's autoloader maps namespace to path 1:1: `\\{component}\\task\\foo` must live at `classes/task/foo.php`.",
			"Run `admin/cli/purge_caches.php` — the autoloader's class map is cached and won't pick up a newly added file until purged.",
		},
	},
	{
		keywords: []string{"event", "observer"},
		hints: []string{
			"Check `db/events.php`'s `eventname` is the fully-qualified class name of the event, including the leading backslash.",
			"Run `admin/cli/purge_caches.php` — observer registrations are cached.",
		},
	},
	{
		keywords: []string{"task", "cron"},
		hints: []string{
			"The task class must extend `\\core\\task\\scheduled_task` (or `\\core\\task\\adhoc_task`).",
			"Run the task manually to see its real error: `php admin/cli/scheduled_task.php --execute='\\{component}\\task\\{name}'`.",
		},
	},
	{
		keywords: []string{"web service", "external", "ajax"},
		hints: []string{
			"Confirm the function is registered in `db/services.php` with the correct `classname`/`methodname`.",
			"Check `execute_parameters()`/`execute_returns()` match the actual arguments/return value shape exactly — a mismatch throws before your code even runs.",
			"Enable DEVELOPER debugging (Site admin > Development > Debugging) to see the full AJAX error response instead of a generic failure.",
		},
	},
}

var genericDebugHints = []string{
	"Enable DEVELOPER-level debugging (Site admin > Development > Debugging) to see full error details.",
	"Check the PHP error log and Moodle's own error log for the full stack trace.",
	"Run `admin/cli/purge_caches.php` — many \"phantom\" errors after a code change are actually stale caches.",
}

func getDebuggingTips(errorText string) []string {
	lower := strings.ToLower(errorText)
	var tips []string
	for _, cat := range debugHintCategories {
		for _, kw := range cat.keywords {
			if strings.Contains(lower, kw) {
				tips = append(tips, cat.hints...)
				break
			}
		}
	}
	if len(tips) == 0 {
		return genericDebugHints
	}
	return tips
}

const debugFewShotUser = "Debug local_demo: error 'Call to a member function get_name() on null' happening " +
	"when the scheduled task runs via cron."

const debugFewShotAssistant = "## Root Cause\n\nThe cron runner is calling `get_name()` on a task object " +
	"that failed to instantiate — almost always because the task's class doesn't extend " +
	"`\\core\\task\\scheduled_task`, so Moodle's task API returns `null` instead of a valid task instance " +
	"when it tries to construct it from the class name in `db/tasks.php`.\n\n" +
	"## Evidence\n\nThe error occurs specifically inside the cron dispatch loop (`get_name()` is called by " +
	"the scheduler to log which task is running), which only happens after the class has already failed to " +
	"resolve — the null is the scheduler's own defensive check surfacing, not a bug in the task's own logic.\n\n" +
	"## Fix\n\nConfirm the class in `classes/task/{name}.php` declares " +
	"`class {name} extends \\core\\task\\scheduled_task` (not a bare class, and not extending " +
	"`adhoc_task` if `db/tasks.php` registers it as scheduled). Re-run " +
	"`php admin/cli/scheduled_task.php --execute='\\local_demo\\task\\{name}'` to confirm the fix.\n\n" +
	"## Prevention\n\nAdd a unit test that instantiates the task class directly and asserts it's an " +
	"instance of `\\core\\task\\scheduled_task`, so a future class-signature regression fails in CI instead " +
	"of surfacing only in production cron logs."

func handleDebugPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments
	if err := requireArgs(args, "plugin", "error"); err != nil {
		return nil, err
	}
	plugin := args["plugin"]
	errorText := truncateArg(args["error"])
	errContext := truncateArg(args["context"])

	rc := resolvePluginForPrompt(plugin)
	pluginContext := rc.readPluginFileTruncated("PLUGIN_AI_CONTEXT.md", 3000)
	runtimeFlow := rc.readPluginFileTruncated("PLUGIN_RUNTIME_FLOW.md", 2000)
	dbTables := rc.readPluginFileTruncated("PLUGIN_DB_TABLES.md", 1500)

	var b strings.Builder
	fmt.Fprintf(&b, "## Debug Request\n\n| Field | Value |\n|---|---|\n| Component | %s |\n| Type | %s |\n"+
		"| Version | %s |\n| Moodle version | %s |\n\n", rc.Component, rc.Type, rc.Version, rc.MoodleVersion)

	fmt.Fprintf(&b, "### Error\n\n```\n%s\n```\n\n", errorText)
	if errContext != "" {
		fmt.Fprintf(&b, "### When It Occurs\n\n%s\n\n", errContext)
	}

	b.WriteString("### Moodle Debugging Hints\n\n")
	for _, tip := range getDebuggingTips(errorText) {
		fmt.Fprintf(&b, "- %s\n", tip)
	}
	b.WriteString("\n")

	if runtimeFlow != "" {
		fmt.Fprintf(&b, "### Plugin Runtime Flow\n\n%s\n\n", runtimeFlow)
	}
	if dbTables != "" {
		fmt.Fprintf(&b, "### Database Schema\n\n%s\n\n", dbTables)
	}
	if pluginContext != "" {
		fmt.Fprintf(&b, "### Plugin Context\n\n%s\n\n", pluginContext)
	}

	b.WriteString("### Task\n\nProvide, in order: `## Root Cause`, `## Evidence`, `## Fix`, and " +
		"`## Prevention`. Reference APIs, hooks, and patterns by name.\n")

	return &mcp.GetPromptResult{
		Messages: []*mcp.PromptMessage{
			{Role: "user", Content: &mcp.TextContent{Text: debugFewShotUser}},
			{Role: "assistant", Content: &mcp.TextContent{Text: debugFewShotAssistant}},
			{Role: "user", Content: &mcp.TextContent{Text: b.String()}},
		},
	}, nil
}
