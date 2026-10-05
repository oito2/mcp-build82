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

package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/config"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/generators"
)

type InitInput struct {
	MoodlePath string `json:"moodle_path" jsonschema:"Absolute path to the Moodle installation root directory"`
	Force      bool   `json:"force,omitempty" jsonschema:"Re-initialize even if configuration already exists"`
	Format     Format `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

// InitOutput is the JSON-format shape for init_moodle_context — the template decision every other
// tool's format:"json" output follows: a struct mirroring the same data the text response renders.
type InitOutput struct {
	Success            bool         `json:"success"`
	AlreadyInitialized bool         `json:"already_initialized,omitempty"`
	MoodlePath         string       `json:"moodle_path"`
	MoodleVersion      string       `json:"moodle_version"`
	MoodleFullVersion  string       `json:"moodle_full_version"`
	ConfigPath         string       `json:"config_path"`
	Generated          []string     `json:"generated,omitempty"`
	Skipped            []string     `json:"skipped,omitempty"`
	Failed             []FailedFile `json:"failed,omitempty"`
}

func RegisterInitTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "init_moodle_context",
		Description: "Initializes AI context for a Moodle installation. Validates the path, detects " +
			"the version, saves configuration, and generates all global index files (API index, events, " +
			"tasks, services, DB tables, classes, capabilities, plugin map, dev rules, workspace). " +
			"Run this first before using any other build82 tool.",
	}, withRecover(handleInit))
}

func validateMoodlePath(path string) (string, bool) {
	if !dirExists(path) {
		return "directory does not exist", false
	}
	if !fileExists(joinPath(path, "version.php")) {
		return "version.php not found — this doesn't look like a Moodle root", false
	}
	if !dirExists(joinPath(path, "lib")) {
		return "lib/ directory not found", false
	}
	if !fileExists(joinPath(path, "config.php")) && !fileExists(joinPath(path, "config-dist.php")) {
		return "neither config.php nor config-dist.php found", false
	}
	return "", true
}

func handleInit(ctx context.Context, req *mcp.CallToolRequest, in InitInput) (*mcp.CallToolResult, struct{}, error) {
	// Resolved once up front and threaded through explicitly (rather than each of config.Load/
	// config.FilePath/buildInitOutput/renderInitReport re-resolving it). config.FilePath surfaces a
	// failure to resolve the home directory as a real error instead of silently proceeding with a
	// cwd-relative path, so that failure must be handled here just like any other.
	configPath, err := config.FilePath()
	if err != nil {
		return textResult(true, "❌ Failed to resolve build82 configuration: "+err.Error()), struct{}{}, nil
	}

	existing, err := config.Load()
	if err != nil {
		return textResult(true, "❌ Failed to resolve build82 configuration: "+err.Error()), struct{}{}, nil
	}
	if existing != nil && !in.Force {
		msg := fmt.Sprintf(
			"ℹ️ build82 is already initialized.\n\nMoodle path: %s\nVersion: %s\n\n"+
				"Pass force: true to re-initialize, or use `update_indexes` to refresh existing context.",
			existing.MoodlePath, existing.MoodleVersion)
		if in.Format == FormatJSON {
			return jsonResult(false, InitOutput{
				Success: true, AlreadyInitialized: true,
				MoodlePath: existing.MoodlePath, MoodleVersion: existing.MoodleVersion,
				MoodleFullVersion: existing.MoodleFullVersion, ConfigPath: configPath,
			}), struct{}{}, nil
		}
		return textResult(false, msg), struct{}{}, nil
	}

	if reason, ok := validateMoodlePath(in.MoodlePath); !ok {
		if in.Format == FormatJSON {
			return jsonResult(true, InitOutput{Success: false, MoodlePath: in.MoodlePath}), struct{}{}, nil
		}
		return textResult(true, "❌ Invalid Moodle path: "+reason), struct{}{}, nil
	}

	installInfo := extractors.DetectMoodleInstall(in.MoodlePath)
	version, fullVersion := "", ""
	if installInfo != nil {
		version, fullVersion = installInfo.Version, installInfo.Build
	}

	if err := config.Save(config.Config{MoodlePath: in.MoodlePath, MoodleVersion: version, MoodleFullVersion: fullVersion}); err != nil {
		return textResult(true, "❌ Failed to save config: "+err.Error()), struct{}{}, nil
	}

	results := generators.GenerateAll(in.MoodlePath, version)

	if in.Format == FormatJSON {
		return jsonResult(false, buildInitOutput(in.MoodlePath, version, fullVersion, configPath, results)), struct{}{}, nil
	}
	return textResult(false, renderInitReport(in.MoodlePath, version, fullVersion, configPath, results)), struct{}{}, nil
}

func buildInitOutput(moodlePath, version, fullVersion, configPath string, results []generators.GeneratorResult) InitOutput {
	generated, skipped, failed := classifyResults(results, moodlePath)
	return InitOutput{
		Success: true, MoodlePath: moodlePath, MoodleVersion: version,
		MoodleFullVersion: fullVersion, ConfigPath: configPath,
		Generated: generated, Skipped: skipped, Failed: failed,
	}
}

func renderInitReport(moodlePath, version, fullVersion, configPath string, results []generators.GeneratorResult) string {
	var b strings.Builder
	b.WriteString("✅ build82 initialized successfully.\n\n")
	fmt.Fprintf(&b, "Moodle path: %s\nVersion: %s\nFull version: %s\nConfig file: %s\n\n", moodlePath, version, fullVersion, configPath)

	generated, skipped, failed := classifyResults(results, moodlePath)
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

	b.WriteString("\n## Next Steps\n\n" +
		"- Run `generate_plugin_context` on a specific plugin you're about to work on.\n" +
		"- Run `update_indexes` after making structural changes to the installation.\n" +
		"- Use `search_plugins`/`search_api` to find existing code.\n")

	return b.String()
}
