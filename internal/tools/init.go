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

	"github.com/oito2/mcp-build82/internal/config"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/resources"
)

// InitInput is the input of the init_moodle_context tool.
type InitInput struct {
	MoodlePath string `json:"moodle_path" jsonschema:"Absolute path to the Moodle installation root directory"`
	Force      bool   `json:"force,omitempty" jsonschema:"Re-initialize even if configuration already exists"`
	Format     Format `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

// InitOutput is the structured output of init_moodle_context. It mirrors the data rendered by the
// text response.
type InitOutput struct {
	Success            bool         `json:"success" jsonschema:"Whether build82 is initialized for the Moodle path"`
	AlreadyInitialized bool         `json:"already_initialized,omitempty" jsonschema:"True when a configuration already existed and force was not set, so nothing was regenerated"`
	MoodlePath         string       `json:"moodle_path" jsonschema:"Absolute path of the configured Moodle root"`
	MoodleVersion      string       `json:"moodle_version" jsonschema:"Detected Moodle release, such as 4.5"`
	MoodleFullVersion  string       `json:"moodle_full_version" jsonschema:"Detected Moodle build number (the $version of version.php)"`
	ConfigPath         string       `json:"config_path" jsonschema:"Absolute path of the build82 configuration file"`
	Generated          []string     `json:"generated,omitempty" jsonschema:"Global index files written by this call"`
	Skipped            []string     `json:"skipped,omitempty" jsonschema:"Global index files served from the cache because their sources did not change"`
	Failed             []FailedFile `json:"failed,omitempty" jsonschema:"Global index files that could not be generated"`
}

// RegisterInitTool registers the init_moodle_context tool on `server`.
func RegisterInitTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "init_moodle_context",
		InputSchema: inputSchemaFor[InitInput](),
		Annotations: toolAnnotations("Initialize Moodle Context", false, false, true, false),
		Description: "Initializes AI context for a Moodle installation. Validates the path, detects " +
			"the version, saves configuration, and generates all global index files (API index, events, " +
			"tasks, services, DB tables, classes, capabilities, plugin map, dev rules, workspace). " +
			"Run this first before using any other build82 tool.",
	}, withRecover(withResourceNotifications(server, handleInit, func(_ InitInput, out InitOutput) []string {
		if out.AlreadyInitialized {
			return nil
		}
		return resources.GlobalURIs()
	})))
}

// validateMoodlePath checks that `path` looks like a Moodle root: an existing directory with
// version.php, a lib/ directory, and config.php or config-dist.php. It returns ("", true) when
// valid, otherwise a human-readable reason and false.
func validateMoodlePath(path string) (string, bool) {
	if !dirExists(path) {
		return "directory does not exist", false
	}
	if !fileExists(filepath.Join(path, "version.php")) {
		return "version.php not found — this doesn't look like a Moodle root", false
	}
	if !dirExists(filepath.Join(path, "lib")) {
		return "lib/ directory not found", false
	}
	if !fileExists(filepath.Join(path, "config.php")) && !fileExists(filepath.Join(path, "config-dist.php")) {
		return "neither config.php nor config-dist.php found", false
	}
	return "", true
}

// handleInit validates `in.MoodlePath`, detects the Moodle version, saves the configuration and
// generates all global index files. When a configuration already exists and `in.Force` is false,
// it reports that without changing anything. The structured output is an InitOutput in either
// format; problems are returned as errors, so those results have no structured output.
func handleInit(ctx context.Context, req *mcp.CallToolRequest, in InitInput) (*mcp.CallToolResult, InitOutput, error) {
	// The config path is resolved once and passed to the output builders. A failure to resolve
	// the home directory is reported as an error rather than falling back to a relative path.
	configPath, err := config.FilePath()
	if err != nil {
		return nil, InitOutput{}, configError(err)
	}

	existing, err := config.Load()
	if err != nil {
		return nil, InitOutput{}, configError(err)
	}
	if existing != nil && !in.Force {
		msg := fmt.Sprintf(
			"ℹ️ build82 is already initialized.\n\nMoodle path: %s\nVersion: %s\n\n"+
				"Pass force: true to re-initialize, or use `update_indexes` to refresh existing context.",
			existing.MoodlePath, existing.MoodleVersion)
		return structuredResult(in.Format, false, msg, InitOutput{
			Success: true, AlreadyInitialized: true,
			MoodlePath: existing.MoodlePath, MoodleVersion: existing.MoodleVersion,
			MoodleFullVersion: existing.MoodleFullVersion, ConfigPath: configPath,
		})
	}

	// The path is stored and used as an absolute, cleaned path: a relative one would be resolved
	// against this process's working directory now and against another one by every later
	// process (each MCP client starts the server from its own directory).
	raw := strings.TrimSpace(in.MoodlePath)
	if raw == "" {
		return nil, InitOutput{}, toolError("❌ Invalid Moodle path: moodle_path is required")
	}
	moodlePath, err := filepath.Abs(raw)
	if err != nil {
		return nil, InitOutput{}, toolError("❌ Invalid Moodle path: " + err.Error())
	}
	if reason, ok := validateMoodlePath(moodlePath); !ok {
		return nil, InitOutput{}, toolError("❌ Invalid Moodle path: " + reason)
	}

	installInfo := extractors.DetectMoodleInstall(moodlePath)
	version, fullVersion := "", ""
	if installInfo != nil {
		version, fullVersion = installInfo.Version, installInfo.Build
	}

	if err := config.Save(config.Config{MoodlePath: moodlePath, MoodleVersion: version, MoodleFullVersion: fullVersion}); err != nil {
		return nil, InitOutput{}, toolError("❌ Failed to save config: " + err.Error())
	}

	results := generators.GenerateAll(moodlePath, version)

	out := buildInitOutput(moodlePath, version, fullVersion, configPath, results)
	report := renderInitReport(out)
	if existing != nil && existing.Source == config.SourceEnv {
		report += "\n⚠ BUILD82_MOODLE_PATH is set in this server's environment, so it keeps taking precedence " +
			"over the saved file; change it in the MCP client's configuration (or run `build82 install` again) to switch installations.\n"
	}
	return structuredResult(in.Format, false, report, out)
}

// buildInitOutput builds the structured output of a successful initialization from the generator
// `results`, with file names relative to `moodlePath`.
func buildInitOutput(moodlePath, version, fullVersion, configPath string, results []generators.GeneratorResult) InitOutput {
	generated, skipped, failed := classifyResults(results, moodlePath)
	return InitOutput{
		Success: true, MoodlePath: moodlePath, MoodleVersion: version,
		MoodleFullVersion: fullVersion, ConfigPath: configPath,
		Generated: generated, Skipped: skipped, Failed: failed,
	}
}

// renderInitReport renders the Markdown report of the successful initialization `out`: the detected
// Moodle details, the failed, generated and cached files relative to the Moodle root, and next
// steps.
func renderInitReport(out InitOutput) string {
	var b strings.Builder
	b.WriteString("✅ build82 initialized successfully.\n\n")
	fmt.Fprintf(&b, "Moodle path: %s\nVersion: %s\nFull version: %s\nConfig file: %s\n\n",
		out.MoodlePath, out.MoodleVersion, out.MoodleFullVersion, out.ConfigPath)
	writeGenerationResults(&b, out.Generated, out.Skipped, out.Failed)
	b.WriteString("\n## Next Steps\n\n" +
		"- Run `generate_plugin_context` on a specific plugin you're about to work on.\n" +
		"- Run `update_indexes` after making structural changes to the installation.\n" +
		"- Use `search_plugins`/`search_api` to find existing code.\n")

	return b.String()
}
