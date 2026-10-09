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
	"fmt"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/toolutil"
)

// ExplainSection selects which part of a plugin explain_plugin returns.
type ExplainSection string

// Supported values of ExplainSection.
const (
	SectionAll      ExplainSection = "all"
	SectionOverview ExplainSection = "overview"
	SectionDatabase ExplainSection = "database"
	SectionEvents   ExplainSection = "events"
	SectionClasses  ExplainSection = "classes"
	SectionServices ExplainSection = "services"
	SectionFlow     ExplainSection = "flow"
)

// ExplainPluginInput is the input of the explain_plugin tool.
type ExplainPluginInput struct {
	Plugin  string         `json:"plugin" jsonschema:"Component, relative path, or absolute path"`
	Section ExplainSection `json:"section,omitempty" jsonschema:"'all' (default), 'overview', 'database', 'events', 'classes', 'services', or 'flow'"`
}

// RegisterExplainTool registers the explain_plugin tool on `server`.
func RegisterExplainTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "explain_plugin",
		Annotations: toolAnnotations("Explain Plugin", true, false, true, false),
		Description: "Returns a compact, section-selectable explanation of a plugin — metadata, database, " +
			"events, classes, services/capabilities, or runtime flow — optimized to be cheaper to read than " +
			"the full generated index files.",
	}, withRecover(handleExplainPlugin))
}

// validExplainSections is the set of ExplainSection values handleExplainPlugin accepts. Any other
// value (most often a typo) is rejected up front, since it would otherwise match none of the
// section checks and silently produce a truncated response.
var validExplainSections = map[ExplainSection]bool{
	SectionAll:      true,
	SectionOverview: true,
	SectionDatabase: true,
	SectionEvents:   true,
	SectionClasses:  true,
	SectionServices: true,
	SectionFlow:     true,
}

// handleExplainPlugin returns a compact explanation of the plugin `in.Plugin`, limited to
// `in.Section` (default "all"). For "all" it returns the cached PLUGIN_AI_CONTEXT.md when present
// and otherwise builds the sections from the plugin source. It returns an error result for an
// unknown section, a missing configuration, or an invalid plugin; the error return is always nil.
func handleExplainPlugin(ctx context.Context, req *mcp.CallToolRequest, in ExplainPluginInput) (*mcp.CallToolResult, any, error) {
	if in.Section == "" {
		in.Section = SectionAll
	}
	if !validExplainSections[in.Section] {
		return textResult(true, fmt.Sprintf(
			"❌ Unknown section %q. Valid values: all, overview, database, events, classes, services, flow.",
			in.Section)), nil, nil
	}

	cfg, err := requireConfig()
	if err != nil {
		return textResult(true, "❌ Failed to resolve build82 configuration: "+err.Error()), nil, nil
	}
	if cfg == nil {
		return toolutil.NotInitialized(), nil, nil
	}

	rp, errResult := resolveAndValidatePlugin(in.Plugin, cfg.MoodlePath)
	if errResult != nil {
		return errResult, nil, nil
	}

	// Fast path: for section "all", reuse the cached PLUGIN_AI_CONTEXT.md when it exists.
	if in.Section == SectionAll {
		if content := readPluginFileTruncated(rp.Path, "PLUGIN_AI_CONTEXT.md", 1<<20); content != "" {
			return textResult(false, content), nil, nil
		}
	}

	var b strings.Builder

	// Overview: metadata table and key files.
	fmt.Fprintf(&b, "# %s\n\n| Field | Value |\n|---|---|\n| Type | %s |\n| Version | %s |\n"+
		"| Requires | %s |\n| Display name | %s |\n| Path | %s |\n\n",
		rp.Info.Component, rp.Info.Type, rp.Info.Version, rp.Info.Requires, rp.Info.DisplayName,
		relativeToMoodle(cfg.MoodlePath, rp.Path))

	b.WriteString("## Key Files Present\n\n")
	for _, f := range []string{"version.php", "lib.php", "settings.php", "db/install.xml", "db/access.php", "db/events.php", "db/tasks.php", "db/services.php"} {
		mark := ""
		if fileExists(filepath.Join(rp.Path, f)) {
			mark = "✔"
		}
		fmt.Fprintf(&b, "- %s `%s`\n", mark, f)
	}

	if in.Section == SectionOverview {
		return textResult(false, b.String()), nil, nil
	}
	b.WriteString("\n---\n\n")

	if in.Section == SectionAll || in.Section == SectionDatabase {
		schema := extractors.ExtractPluginSchema(rp.Path)
		b.WriteString("## Database\n\n| Table | Fields | Keys |\n|---|---|---|\n")
		if schema != nil {
			for _, t := range schema.Tables {
				fmt.Fprintf(&b, "| `%s` | %d | %d |\n", t.Name, len(t.Fields), len(t.Keys))
			}
		}
		if in.Section == SectionDatabase {
			return textResult(false, b.String()), nil, nil
		}
		b.WriteString("\n---\n\n")
	}

	if in.Section == SectionAll || in.Section == SectionClasses {
		classes := extractors.ExtractPluginClasses(rp.Path)
		b.WriteString("## Classes\n\n| FQN | Kind | Extends |\n|---|---|---|\n")
		for _, c := range classes.Classes {
			fmt.Fprintf(&b, "| `%s` | %s | %s |\n", c.FQN, c.Kind, c.Extends)
		}
		if in.Section == SectionClasses {
			return textResult(false, b.String()), nil, nil
		}
		b.WriteString("\n---\n\n")
	}

	if in.Section == SectionAll || in.Section == SectionEvents {
		events := extractors.ExtractPluginEvents(rp.Path)
		b.WriteString("## Events\n\n")
		if events != nil {
			for _, o := range events.Observers {
				fmt.Fprintf(&b, "- `%s` → `%s`\n", o.EventName, o.Callback)
			}
		}
		if in.Section == SectionEvents {
			return textResult(false, b.String()), nil, nil
		}
		b.WriteString("\n---\n\n")
	}

	if in.Section == SectionAll || in.Section == SectionServices {
		services := extractors.ExtractPluginServices(rp.Path)
		caps := extractors.ExtractPluginCapabilities(rp.Path)
		b.WriteString("## Web Services\n\n| Function | Class |\n|---|---|\n")
		if services != nil {
			for _, fn := range services.Functions {
				fmt.Fprintf(&b, "| `%s` | `%s` |\n", fn.Name, fn.ClassName)
			}
		}
		b.WriteString("\n### Capabilities\n\n| Capability | Type |\n|---|---|\n")
		if caps != nil {
			for _, c := range caps.Capabilities {
				fmt.Fprintf(&b, "| `%s` | %s |\n", c.Name, c.CapType)
			}
		}
		if in.Section == SectionServices {
			return textResult(false, b.String()), nil, nil
		}
		b.WriteString("\n---\n\n")
	}

	if in.Section == SectionAll || in.Section == SectionFlow {
		tasks := extractors.ExtractPluginTasks(rp.Path)
		b.WriteString("## Runtime Flow\n\n### Entry Points\n\n")
		for _, f := range []string{"index.php", "view.php", "edit.php", "manage.php", "ajax.php"} {
			mark := ""
			if fileExists(filepath.Join(rp.Path, f)) {
				mark = "✔"
			}
			fmt.Fprintf(&b, "- %s `%s`\n", mark, f)
		}
		b.WriteString("\n### Scheduled Tasks\n\n")
		if tasks != nil {
			for _, t := range tasks.Tasks {
				fmt.Fprintf(&b, "- `%s` — `%s`\n", t.ClassName, extractors.FormatCronSchedule(t))
			}
		}
		if in.Section == SectionFlow {
			return textResult(false, b.String()), nil, nil
		}
	}

	b.WriteString("\n_Run `generate_plugin_context` to create the full, cached AI context file for this plugin._\n")
	return textResult(false, b.String()), nil, nil
}
