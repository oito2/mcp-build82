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
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/config"
	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/moodletype"
)

// RegisterScaffoldPrompt registers the scaffold_plugin MCP prompt.
func RegisterScaffoldPrompt(server *mcp.Server) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "scaffold_plugin",
		Description: "Generates a complete, context-rich prompt for scaffolding a new Moodle plugin. Includes a few-shot example so the AI knows exactly what output format is expected.",
		Arguments: []*mcp.PromptArgument{
			{Name: "type", Description: "Plugin type (local, mod, block, auth, tool, enrol, theme, report, format, filter, qtype)", Required: true},
			{Name: "name", Description: "Plugin name — lowercase, letters and underscores only", Required: true},
			{Name: "description", Description: "What the plugin does", Required: true},
			{Name: "features", Description: "Comma-separated features: database tables, scheduled tasks, web services, events, capabilities, settings"},
		},
	}, withRecoverPrompt(handleScaffoldPrompt))
}

// typeNotes holds the plugin-type-specific implementation notes included in the scaffold prompt,
// keyed by plugin type.
var typeNotes = map[string]string{
	"mod": "Must implement `{component}_add_instance`, `{component}_update_instance`, and " +
		"`{component}_delete_instance` in `lib.php`. Standard entry points are `index.php` (list all " +
		"instances in a course) and `view.php` (view one instance). The course-module object `$cm` is " +
		"central to almost every operation.",
	"local": "The most flexible plugin type — no mandatory entry points or interfaces. Common uses: " +
		"custom APIs, event listeners, scheduled tasks, admin tools, and site-wide extensions.",
	"block": "Must extend `block_base`. Required methods: `init()` (sets `$this->title`) and " +
		"`get_content()` (returns the block's rendered content). The class file must be named " +
		"`block_{name}.php` at the plugin root, where `{name}` is the part of the component after the " +
		"first underscore.",
	"auth": "Must extend `auth_plugin_base`. At minimum, implement `user_login($username, $password)`. " +
		"The class file must be `auth.php` at the plugin root.",
	"tool": "Appears under Site Administration. `index.php` is the standard entry point for the tool's UI.",
}

// getTypeNotes returns the implementation notes for `pluginType`, or a generic sentence when the
// type has no dedicated notes.
func getTypeNotes(pluginType string) string {
	if notes, ok := typeNotes[pluginType]; ok {
		return notes
	}
	return fmt.Sprintf("Follow standard Moodle conventions for %s plugins.", pluginType)
}

// fallbackCodingStandards is the coding-standards list used when no generated MOODLE_DEV_RULES.md
// is available.
var fallbackCodingStandards = []string{
	"Follow the Moodle coding style (4-space indent, snake_case functions, PascalCase classes).",
	"Access the database only through the `$DB` API, never raw SQL.",
	"Call `require_login()` and `require_capability()` before any protected action.",
	"Read request parameters via `required_param()`/`optional_param()` with explicit PARAM types.",
	"Render output through `$OUTPUT`, never raw echoed HTML.",
	"Add `defined('MOODLE_INTERNAL') || die();` at the top of every non-entry-point PHP file.",
}

// parseFeatures scans the comma-separated `features` text (case-insensitively) and reports which
// feature groups are requested: database tables, scheduled tasks, web services, events,
// capabilities, and settings. Each item matches at most one group, and unrecognized items are
// ignored.
func parseFeatures(features string) (hasDb, hasTasks, hasServices, hasEvents, hasCaps, hasSettings bool) {
	for _, f := range strings.Split(strings.ToLower(features), ",") {
		f = strings.TrimSpace(f)
		switch {
		case strings.Contains(f, "database") || strings.Contains(f, "table"):
			hasDb = true
		case strings.Contains(f, "task"):
			hasTasks = true
		case strings.Contains(f, "service") || strings.Contains(f, "api"):
			hasServices = true
		case strings.Contains(f, "event"):
			hasEvents = true
		case strings.Contains(f, "capabilit") || strings.Contains(f, "permission"):
			hasCaps = true
		case strings.Contains(f, "setting"):
			hasSettings = true
		}
	}
	return
}

// scaffoldFewShotUser and scaffoldFewShotAssistant form the few-shot example exchange that
// precedes the real scaffold request, showing the expected output format.
const scaffoldFewShotUser = "Scaffold local_demo's version.php for a plugin that stores demo records in a database table."

const scaffoldFewShotAssistant = "## version.php\n\n```php\n<?php\ndefined('MOODLE_INTERNAL') || die();\n\n" +
	"$plugin->component = 'local_demo';\n$plugin->version   = 2024010100;\n$plugin->requires  = 2023100900;\n" +
	"$plugin->maturity  = MATURITY_STABLE;\n$plugin->release   = '1.0.0';\n```\n\n" +
	"> Decision: version follows the YYYYMMDDXX convention with a 00 build suffix, and requires is set to " +
	"the earliest supported Moodle 4.3 release since the plugin needs no newer core APIs."

// handleScaffoldPrompt renders the scaffold_plugin prompt from the required "type", "name" and
// "description" arguments and the optional "features" argument. The request lists the derived
// component and directory, the requested features, type-specific notes, coding standards (from
// the generated MOODLE_DEV_RULES.md when available), and the files to generate. It returns an
// invalid-params error when a required argument is missing, or the error from config.Load.
func handleScaffoldPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments
	if err := requireArgs(args, "type", "name", "description"); err != nil {
		return nil, err
	}
	pluginType := args["type"]
	name := args["name"]
	description := truncateArg(args["description"])
	features := truncateArg(args["features"])

	component := pluginType + "_" + name
	typeDir, ok := moodletype.PluginTypeToDir[pluginType]
	if !ok {
		typeDir = pluginType
	}
	directory := typeDir + "/" + name

	hasDb, hasTasks, hasServices, hasEvents, hasCaps, hasSettings := parseFeatures(features)

	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	moodleVersion := ""
	devRules := ""
	if cfg != nil {
		moodleVersion = cfg.MoodleVersion
		devRules = readFileTruncated(generators.GlobalOutputPath(cfg.MoodlePath, "MOODLE_DEV_RULES.md"), 2000)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## Scaffold a new Moodle plugin\n\n| Field | Value |\n|---|---|\n"+
		"| Component | `%s` |\n| Type | %s |\n| Directory | `%s` |\n| Moodle version | %s |\n| Description | %s |\n\n",
		component, pluginType, directory, moodleVersion, description)

	if features != "" {
		fmt.Fprintf(&b, "### Features to Implement\n\n%s\n\n", featuresBulletList(hasDb, hasTasks, hasServices, hasEvents, hasCaps, hasSettings))
	}

	fmt.Fprintf(&b, "### Type-Specific Notes\n\n%s\n\n", getTypeNotes(pluginType))

	b.WriteString("### Moodle Coding Standards\n\n")
	if devRules != "" {
		b.WriteString(devRules + "\n\n")
	} else {
		for _, s := range fallbackCodingStandards {
			fmt.Fprintf(&b, "- %s\n", s)
		}
		b.WriteString("\n")
	}

	b.WriteString("### Files to Generate\n\n- `version.php`\n- `lang/en/" + component + ".php`\n")
	switch pluginType {
	case "block":
		fmt.Fprintf(&b, "- `block_%s.php`\n", name)
	case "auth":
		b.WriteString("- `auth.php`\n")
	case "mod":
		b.WriteString("- `lib.php`\n- `index.php`\n- `view.php`\n- `mod_form.php`\n")
	case "tool":
		b.WriteString("- `index.php`\n")
	default:
		b.WriteString("- `lib.php`\n")
	}
	if hasDb {
		b.WriteString("- `db/install.xml`\n")
	}
	if hasTasks {
		b.WriteString("- `db/tasks.php`\n- `classes/task/*.php`\n")
	}
	if hasServices {
		b.WriteString("- `db/services.php`\n- `classes/external/*.php`\n")
	}
	if hasEvents {
		b.WriteString("- `db/events.php`\n- `classes/event/*.php`\n- `classes/observer.php`\n")
	}
	if hasCaps {
		b.WriteString("- `db/access.php`\n")
	}
	if hasSettings {
		b.WriteString("- `settings.php`\n")
	}

	b.WriteString("\n### Output Format\n\n" +
		"For each file, use a numbered `## path` heading followed by a fenced ```php code block, and a " +
		"one-line `> Decision:` note explaining any non-obvious choice. Start with `version.php` and " +
		"proceed in dependency order (e.g. `lib.php`/entry points before files that reference their classes).\n")

	mainPrompt := b.String()

	return &mcp.GetPromptResult{
		Messages: []*mcp.PromptMessage{
			{Role: "user", Content: &mcp.TextContent{Text: scaffoldFewShotUser}},
			{Role: "assistant", Content: &mcp.TextContent{Text: scaffoldFewShotAssistant}},
			{Role: "user", Content: &mcp.TextContent{Text: mainPrompt}},
		},
	}, nil
}

// featuresBulletList renders the enabled feature flags as a Markdown bullet list, one line per
// feature, in a fixed order.
func featuresBulletList(hasDb, hasTasks, hasServices, hasEvents, hasCaps, hasSettings bool) string {
	var lines []string
	if hasDb {
		lines = append(lines, "- Database tables")
	}
	if hasTasks {
		lines = append(lines, "- Scheduled tasks")
	}
	if hasServices {
		lines = append(lines, "- Web services")
	}
	if hasEvents {
		lines = append(lines, "- Events")
	}
	if hasCaps {
		lines = append(lines, "- Capabilities")
	}
	if hasSettings {
		lines = append(lines, "- Settings page")
	}
	return strings.Join(lines, "\n")
}
