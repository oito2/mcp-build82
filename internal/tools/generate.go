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
	"os"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/toolutil"
)

// GenerateContextInput is the input of the generate_plugin_context tool.
type GenerateContextInput struct {
	PluginPath string `json:"plugin_path" jsonschema:"Component (e.g. 'local_myplugin'), path relative to the Moodle root (e.g. 'local/myplugin'), or absolute path"`
	Format     Format `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

// RegisterGenerateContextTool registers the generate_plugin_context tool on `server`.
func RegisterGenerateContextTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "generate_plugin_context",
		Annotations: toolAnnotations("Generate Plugin Context", false, false, true, false),
		Description: "Generates the 12 PLUGIN_*.md context files for a single Moodle plugin: metadata, " +
			"structure, DB tables, events, dependencies, function/callback/endpoint indexes, runtime flow, " +
			"architecture, settings, and the combined AI context file.",
	}, withRecover(handleGenerateContext))
}

// handleGenerateContext generates the per-plugin context files for the plugin named by
// `in.PluginPath` and refreshes the global AI index. It returns an error result when the
// configuration is missing or invalid or the plugin path is rejected — returned as an error, so
// that result has no structured output; failures of individual files are reported inside the
// successful report. The structured output is a PluginContextOutput in either format.
func handleGenerateContext(ctx context.Context, req *mcp.CallToolRequest, in GenerateContextInput) (*mcp.CallToolResult, PluginContextOutput, error) {
	cfg, err := requireConfig()
	if err != nil {
		return nil, PluginContextOutput{}, configError(err)
	}
	if cfg == nil {
		return nil, PluginContextOutput{}, resultError(toolutil.NotInitialized())
	}

	rp, errResult := resolveAndValidatePlugin(in.PluginPath, cfg.MoodlePath)
	if errResult != nil {
		return nil, PluginContextOutput{}, resultError(errResult)
	}

	result := generators.GenerateAllForPlugin(rp.Path, cfg.MoodlePath, true, &rp.Info)
	// A failure writing MOODLE_AI_INDEX.md is reported as a stderr warning rather than in the
	// report below, whose paths are relative to the plugin directory, not the Moodle root where
	// this global file lives.
	if r := generators.GenerateAiIndex(cfg.MoodlePath, cfg.MoodleVersion); !r.Success {
		fmt.Fprintln(os.Stderr, "[build82] warning: failed to update MOODLE_AI_INDEX.md:", r.Error)
	}

	return structuredResult(in.Format, false, renderPluginContextReport(rp, cfg.MoodlePath, result), buildPluginContextOutput(rp, cfg.MoodlePath, result))
}

// PluginContextOutput is the structured output of generate_plugin_context: the plugin's component,
// type, version and path relative to the Moodle root, and the context files generated, served from
// the cache and failed (relative to the plugin directory).
type PluginContextOutput struct {
	Component string       `json:"component"`
	Type      string       `json:"type"`
	Version   string       `json:"version"`
	Path      string       `json:"path"`
	Generated []string     `json:"generated,omitempty"`
	Skipped   []string     `json:"skipped,omitempty"`
	Failed    []FailedFile `json:"failed,omitempty"`
}

// buildPluginContextOutput builds the structured output for plugin `rp` from the generator `result`.
// The plugin Path is relative to `moodlePath`, never an absolute host path.
func buildPluginContextOutput(rp resolvedPlugin, moodlePath string, result generators.PluginGeneratorResult) PluginContextOutput {
	generated, skipped, failed := classifyResults(result.Files, rp.Path)
	return PluginContextOutput{
		Component: rp.Info.Component, Type: rp.Info.Type, Version: rp.Info.Version,
		Path:      relativeToMoodle(moodlePath, rp.Path),
		Generated: generated, Skipped: skipped, Failed: failed,
	}
}

// renderPluginContextReport renders the Markdown report for plugin `rp` from the generator
// `result`, listing failed, generated and cached files relative to the plugin directory;
// `moodlePath` is used to display the plugin path.
func renderPluginContextReport(rp resolvedPlugin, moodlePath string, result generators.PluginGeneratorResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "✅ Generated context for %s (%s), version %s.\n\nPath: %s\n\n",
		rp.Info.Component, rp.Info.Type, rp.Info.Version, relativeToMoodle(moodlePath, rp.Path))

	generated, skipped, failed := classifyResults(result.Files, rp.Path)
	fmt.Fprintf(&b, "## Results\n\n%d generated, %d cached, %d failed\n\n", len(generated), len(skipped), len(failed))

	if len(failed) > 0 {
		b.WriteString("### Failed\n\n")
		for _, f := range failed {
			fmt.Fprintf(&b, "✖ %s: %s\n", f.File, f.Error)
		}
		b.WriteString("\n")
	}

	sort.Strings(generated)
	b.WriteString("### Generated\n\n")
	for _, n := range generated {
		fmt.Fprintf(&b, "✔ %s\n", n)
	}

	if len(skipped) > 0 {
		sort.Strings(skipped)
		b.WriteString("\n### Cached\n\n")
		for _, n := range skipped {
			fmt.Fprintf(&b, "✔ %s\n", n)
		}
	}

	b.WriteString("\nPrimary context file: `.build82/PLUGIN_AI_CONTEXT.md`\n")
	return b.String()
}
