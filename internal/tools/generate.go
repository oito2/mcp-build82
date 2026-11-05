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
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/generators"
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
		InputSchema: inputSchemaFor[GenerateContextInput](),
		Annotations: toolAnnotations("Generate Plugin Context", false, false, true, false),
		Description: "Generates the 12 PLUGIN_*.md context files for a single Moodle plugin: metadata, " +
			"structure, DB tables, events, dependencies, function/callback/endpoint indexes, runtime flow, " +
			"architecture, settings, and the combined AI context file.",
	}, withRecover(withResourceNotifications(server, handleGenerateContext, func(_ GenerateContextInput, out PluginContextOutput) []string {
		return pluginResourceURIs(out.Component)
	})))
}

// handleGenerateContext generates the per-plugin context files for the plugin named by
// `in.PluginPath` and refreshes the global AI index. It returns an error result when the
// configuration is missing or invalid or the plugin path is rejected — returned as an error, so
// that result has no structured output; failures of individual files are reported inside the
// successful report. The structured output is a PluginContextOutput in either format.
func handleGenerateContext(ctx context.Context, req *mcp.CallToolRequest, in GenerateContextInput) (*mcp.CallToolResult, PluginContextOutput, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, PluginContextOutput{}, err
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

	out := buildPluginContextOutput(rp, cfg.MoodlePath, result)
	return structuredResult(in.Format, false, renderPluginContextReport(out), out)
}

// PluginContextOutput is the structured output of generate_plugin_context: the plugin's component,
// type, version and path relative to the Moodle root, and the context files generated, served from
// the cache and failed (relative to the plugin directory).
type PluginContextOutput struct {
	Component string       `json:"component" jsonschema:"Frankenstyle component of the plugin, such as local_myplugin"`
	Type      string       `json:"type" jsonschema:"Moodle plugin type, such as local or mod"`
	Version   string       `json:"version" jsonschema:"Plugin version from its version.php"`
	Path      string       `json:"path" jsonschema:"Plugin directory relative to the Moodle root"`
	Generated []string     `json:"generated,omitempty" jsonschema:"Context files written by this call, relative to the plugin directory"`
	Skipped   []string     `json:"skipped,omitempty" jsonschema:"Context files served from the cache because their sources did not change"`
	Failed    []FailedFile `json:"failed,omitempty" jsonschema:"Context files that could not be generated"`
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

// renderPluginContextReport renders the Markdown report of the structured output `out`: the plugin,
// then the failed, generated and cached files relative to the plugin directory.
func renderPluginContextReport(out PluginContextOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "✅ Generated context for %s (%s), version %s.\n\nPath: %s\n\n",
		out.Component, out.Type, out.Version, out.Path)
	writeGenerationResults(&b, out.Generated, out.Skipped, out.Failed)
	b.WriteString("\nPrimary context file: `.build82/PLUGIN_AI_CONTEXT.md`\n")
	return b.String()
}
