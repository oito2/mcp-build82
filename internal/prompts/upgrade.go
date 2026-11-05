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

package prompts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/genutil"
	"github.com/oito2/mcp-build82/internal/tools"
)

// Row caps of the upgrade_plugin prompt's tables, so a large plugin cannot flood the prompt.
const (
	maxUpgradeDeprecatedRows = 100
	maxUpgradeFindingRows    = 50
)

// targetVersionPattern matches a Moodle release such as "4.5", "5.0" or "4.5.1"; groups 1 and 2
// are the major and minor numbers.
var targetVersionPattern = regexp.MustCompile(`^([1-9]\d?)\.(\d{1,2})(?:\.\d{1,3})?$`)

// RegisterUpgradePrompt registers the upgrade_plugin MCP prompt.
func RegisterUpgradePrompt(server *mcp.Server) {
	server.AddPrompt(&mcp.Prompt{
		Name: "upgrade_plugin",
		Description: "Generates a migration-plan prompt to make a Moodle plugin work on a newer Moodle release: " +
			"its version metadata, the deprecated core API calls and legacy callbacks found in it, its " +
			"upgrade-step consistency findings, and step-by-step instructions.",
		Arguments: []*mcp.PromptArgument{
			{Name: "plugin", Description: "Component, relative path, or absolute path", Required: true},
			{Name: "target_version", Description: "Moodle release to migrate to, such as 4.5 or 5.0", Required: true},
		},
	}, withRecoverPrompt(handleUpgradePrompt))
}

// handleUpgradePrompt renders the upgrade_plugin prompt from the required "plugin" and
// "target_version" arguments. The plugin analysis (deprecated core calls, legacy callbacks with a
// Hook API replacement, upgrade-step consistency) runs only when build82 is configured and the
// plugin resolves to a directory with a version.php inside the Moodle root; otherwise a single
// section says so. It returns an
// invalid-params error when an argument is missing or target_version is not a Moodle release.
func handleUpgradePrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments
	if err := requireArgs(args, "plugin", "target_version"); err != nil {
		return nil, err
	}
	target := strings.TrimSpace(args["target_version"])
	m := targetVersionPattern.FindStringSubmatch(target)
	if m == nil {
		return nil, &jsonrpc.Error{
			Code:    jsonrpc.CodeInvalidParams,
			Message: fmt.Sprintf("invalid target_version %q: expected a Moodle release such as 4.5 or 5.0", truncateArg(target)),
		}
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	branch := major*100 + minor

	rc := resolvePluginForPrompt(args["plugin"])
	analyzable := rc.ConfigAvailable && rc.Path != "" && isFile(filepath.Join(rc.Path, "version.php"))

	var b strings.Builder
	b.WriteString("## Plugin Upgrade Request\n\n| Field | Value |\n|---|---|\n")
	supported := ""
	if analyzable {
		supported = extractors.ReadSupported(rc.Path)
	}
	for _, row := range [][2]string{
		{"Component", rc.Component}, {"Type", rc.Type}, {"Plugin version", rc.Version},
		{"$plugin->requires", rc.Requires}, {"$plugin->supported", supported},
		{"Installed Moodle", rc.MoodleVersion}, {"Target Moodle", fmt.Sprintf("%s (branch %d)", target, branch)},
	} {
		if row[1] != "" {
			fmt.Fprintf(&b, "| %s | %s |\n", row[0], genutil.EscapeMdCell(row[1]))
		}
	}
	b.WriteString("\n")

	if analyzable {
		if pluginContext := rc.readPluginFileTruncated("PLUGIN_AI_CONTEXT.md", 2000); pluginContext != "" {
			fmt.Fprintf(&b, "### Plugin Context\n\n%s\n\n", pluginContext)
		}
		writeDeprecatedCalls(&b, rc)
		writeLegacyCallbacks(&b, rc)
		writeUpgradeFindings(&b, rc)
	} else {
		b.WriteString("### Plugin Analysis\n\nNot available: build82 is not initialized, or no plugin (a directory with a " +
			"version.php) was found at that location inside the configured Moodle root. Run `init_moodle_context` and check the plugin argument, then " +
			"request this prompt again.\n\n")
	}

	b.WriteString("### Notes\n\n")
	installed := rc.MoodleVersion
	if installed == "" {
		installed = "unknown"
	}
	fmt.Fprintf(&b, "- The deprecated-function list comes from the installed Moodle (%s), not from %s: functions "+
		"deprecated or removed between the two releases are missing from it.\n", installed, target)
	b.WriteString("- Read the upgrade notes of every release up to the target: from Moodle 4.5 onwards `UPGRADING.md` " +
		"at the root of the Moodle source; before 4.5, `lib/upgrade.txt` and each component's `upgrade.txt`.\n\n")

	b.WriteString("### Instructions\n\n")
	fmt.Fprintf(&b, "Write a step-by-step migration plan for `%s` to Moodle %s, then carry it out when asked:\n\n", genutil.EscapeMdCell(rc.Component), target)
	fmt.Fprintf(&b, "1. Set `$plugin->requires` to the build number of the oldest Moodle release the plugin will keep "+
		"supporting (from that release's `version.php`), and `$plugin->supported` so its range includes %d.\n", branch)
	b.WriteString("2. Replace each deprecated call listed above with its documented replacement (the `@deprecated` " +
		"docblock of the core function names it).\n")
	b.WriteString("3. Move each legacy callback listed above to the Hook API: add a callback class and register it in " +
		"`db/hooks.php`, keeping the old callback only while releases without the hook stay supported.\n")
	b.WriteString("4. Fix the upgrade-step findings listed above.\n")
	b.WriteString("5. Raise the plugin version with the `bump_plugin_version` tool; pass `upgrade_step: true` when the " +
		"migration changes the database schema or data, and fill in the generated step.\n")
	b.WriteString("6. Check the result with the `doctor` tool and, when available, `check_coding_style`.\n\n")
	b.WriteString("### Output Format\n\nA numbered plan grouped by the steps above; for each change, the **File**, the " +
		"**Change** and the **Reason** (the deprecation, hook or finding it resolves). End with the risks and what " +
		"to test on the target release.\n")

	return &mcp.GetPromptResult{
		Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: b.String()}}},
	}, nil
}

// isFile reports whether `path` is an existing regular file.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// writeDeprecatedCalls writes the section listing the plugin's calls to core functions marked
// @deprecated in the installed Moodle, in the order FindDeprecatedApiUsage reports them (by file
// path, then line), at most maxUpgradeDeprecatedRows.
func writeDeprecatedCalls(b *strings.Builder, rc resolvedPluginContext) {
	calls := extractors.FindDeprecatedApiUsage(rc.Path, tools.DeprecatedFunctionNames(rc.MoodlePath))
	fmt.Fprintf(b, "### Deprecated Core API Calls (%d)\n\n", len(calls))
	if len(calls) == 0 {
		b.WriteString("None found against the installed Moodle's API index.\n\n")
		return
	}
	b.WriteString("| File | Line | Function |\n|---|---|---|\n")
	for i, c := range calls {
		if i == maxUpgradeDeprecatedRows {
			fmt.Fprintf(b, "\n...and %d more.\n", len(calls)-i)
			break
		}
		fmt.Fprintf(b, "| %s | %d | `%s()` |\n", genutil.EscapeMdCell(c.File), c.Line, genutil.EscapeMdCell(c.Function))
	}
	b.WriteString("\n")
}

// writeLegacyCallbacks writes the section listing the plugin's legacy lib.php callbacks that have a
// Hook API replacement.
func writeLegacyCallbacks(b *strings.Builder, rc resolvedPluginContext) {
	hooks, _ := extractors.ExtractPluginHooks(rc.Path, rc.Component)
	warnings := hooks.LegacyWarnings
	fmt.Fprintf(b, "### Legacy Callbacks With a Hook API Replacement (%d)\n\n", len(warnings))
	if len(warnings) == 0 {
		b.WriteString("None found.\n\n")
		return
	}
	b.WriteString("| Legacy callback | Replaced by | Guidance |\n|---|---|---|\n")
	for _, w := range warnings {
		fmt.Fprintf(b, "| `%s` | `%s` | %s |\n", genutil.EscapeMdCell(w.LegacyFunction), genutil.EscapeMdCell(w.ReplacedBy), genutil.EscapeMdCell(w.Guidance))
	}
	b.WriteString("\n")
}

// writeUpgradeFindings writes the section listing the plugin's upgrade-step consistency problems
// (db/upgrade.php against db/install.xml and version.php), at most maxUpgradeFindingRows.
func writeUpgradeFindings(b *strings.Builder, rc resolvedPluginContext) {
	findings := tools.UpgradeConsistencyFindings(rc.Path)
	fmt.Fprintf(b, "### Upgrade-Step Findings (%d)\n\n", len(findings))
	if len(findings) == 0 {
		b.WriteString("None: db/upgrade.php, db/install.xml and version.php agree (or the plugin has no upgrade steps).\n\n")
		return
	}
	for i, f := range findings {
		if i == maxUpgradeFindingRows {
			fmt.Fprintf(b, "- ...and %d more.\n", len(findings)-i)
			break
		}
		fmt.Fprintf(b, "- **%s:** %s\n", f.Status, genutil.EscapeMdCell(f.Detail))
	}
	b.WriteString("\n")
}
