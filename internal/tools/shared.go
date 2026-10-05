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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/config"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/moodletype"
	"github.com/oito2/mcp-build82/internal/toolutil"
)

// textResult is the shared response builder used by every tool.
func textResult(isError bool, text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: isError,
	}
}

// withRecover wraps a tool handler so a panic anywhere inside it (most likely deep in an
// extractor/generator triggered by malformed or adversarial plugin source) becomes an IsError text
// result instead of an unrecovered panic. The MCP SDK's tool dispatch has no recover of its own, so
// an unrecovered panic here kills the whole server process for every connected client/session, not
// just the one request that triggered it. It is applied to every registered tool via
// RegisterXxxTool.
func withRecover[In any](
	fn func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, struct{}, error),
) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, struct{}, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (result *mcp.CallToolResult, out struct{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				result = textResult(true, fmt.Sprintf("❌ Internal error while handling this request: %v", r))
			}
		}()
		return fn(ctx, req, in)
	}
}

// Format is the shared opt-in output-format field every tool's input struct embeds.
// "text" (the zero value) renders Markdown/plain text; "json" renders the same underlying
// data as a JSON string in that same single text block.
type Format string

const (
	FormatText Format = ""
	FormatJSON Format = "json"
)

// jsonResult marshals data as the tool's single text block when a JSON format was requested.
func jsonResult(isError bool, data any) *mcp.CallToolResult {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return textResult(true, "❌ Failed to encode JSON response: "+err.Error())
	}
	return textResult(isError, string(b))
}

// FailedFile is the shared JSON shape for one failed-to-generate file — used by every tool whose
// output distinguishes generated/skipped/failed generator results.
type FailedFile struct {
	File  string `json:"file"`
	Error string `json:"error"`
}

// classifyResults splits generator results into generated/skipped file names and full failed
// records, all relativized against base (moodlePath for global generators, the plugin's own path
// for per-plugin ones) so that generated output never leaks an absolute host filesystem path.
// Shared by every tool's build*Output/render*Report.
func classifyResults(results []generators.GeneratorResult, base string) (generated, skipped []string, failed []FailedFile) {
	for _, r := range results {
		rel := relativeToMoodle(base, r.File)
		switch {
		case !r.Success:
			failed = append(failed, FailedFile{File: rel, Error: r.Error})
		case r.Skipped:
			skipped = append(skipped, rel)
		default:
			generated = append(generated, rel)
		}
	}
	return
}

// resolvedPlugin is the outcome of resolving+validating a plugin path argument, shared by every
// tool that takes a plugin identifier (generate_plugin_context, explain_plugin, and any future
// tool that needs it, via the same helper).
type resolvedPlugin struct {
	Path string
	Info extractors.PluginInfo
}

// resolveAndValidatePlugin resolves identifier — a frankenstyle component ("local_myplugin"), a
// path relative to the Moodle root ("local/myplugin"), or an absolute path — against moodlePath,
// checks it's within the Moodle root, exists, and looks like a plugin (has version.php). Returns a
// non-nil *mcp.CallToolResult (already IsError:true) on any failure — callers should return that
// result immediately when it is non-nil.
func resolveAndValidatePlugin(identifier, moodlePath string) (resolvedPlugin, *mcp.CallToolResult) {
	full, _ := filepath.Abs(resolvePluginIdentifier(identifier, moodlePath))

	// None of the messages below embed moodlePath or the resolved absolute `full` path directly —
	// an absolute host filesystem path must never leak into tool/resource
	// output, since it reveals host directory structure (often an OS username) and could end up
	// quoted back into a third-party AI chat. Anywhere a path is useful to the caller, it's
	// relativized to moodlePath via relativeToMoodle first.
	if !moodletype.IsWithinMoodle(full, moodlePath) {
		return resolvedPlugin{}, textResult(true,
			"❌ Invalid plugin path: must be within the Moodle installation.")
	}
	if _, err := os.Stat(full); err != nil {
		return resolvedPlugin{}, textResult(true, fmt.Sprintf(
			"❌ Plugin directory not found: %s\n\nCheck that the path is correct relative to the Moodle root.",
			relativeToMoodle(moodlePath, full)))
	}
	if _, err := os.Stat(filepath.Join(full, "version.php")); err != nil {
		return resolvedPlugin{}, textResult(true, fmt.Sprintf(
			"❌ %s does not appear to be a Moodle plugin.\n\nA plugin must have a version.php file at its root.",
			relativeToMoodle(moodlePath, full)))
	}

	info, err := extractors.DetectPlugin(full)
	if err != nil {
		return resolvedPlugin{}, textResult(true, "❌ Failed to detect plugin: "+err.Error())
	}
	info.MoodlePath = moodlePath
	return resolvedPlugin{Path: full, Info: info}, nil
}

// resolvePluginIdentifier maps identifier to a filesystem path without checking containment or
// existence — callers must still run moodletype.IsWithinMoodle on the result. It first tries
// moodletype.ResolvePluginPath (component, relative, or absolute, existing paths only); when that
// finds nothing, it falls back to treating identifier as a path (absolute, or joined onto
// moodlePath), so a directory whose name merely looks like a component (e.g. a literal "local_x"
// folder) still resolves, and a missing plugin still gets a meaningful "not found" message.
func resolvePluginIdentifier(identifier, moodlePath string) string {
	if path, ok := moodletype.ResolvePluginPath(identifier, moodlePath); ok {
		return path
	}
	if filepath.IsAbs(identifier) {
		return identifier
	}
	return filepath.Join(moodlePath, identifier)
}

// resolvePluginPathWithinMoodle resolves identifier (component, relative path, or absolute path)
// via moodletype.ResolvePluginPath, then checks moodletype.IsWithinMoodle — the shared containment
// check for get_plugin_info and plugin_batch mode=list. Unlike resolveAndValidatePlugin it does
// not require version.php to exist, a stricter check those two tools deliberately don't want.
// Returns ("", false) if resolution or containment fails.
func resolvePluginPathWithinMoodle(identifier, moodlePath string) (string, bool) {
	path, ok := moodletype.ResolvePluginPath(identifier, moodlePath)
	if ok && !moodletype.IsWithinMoodle(path, moodlePath) {
		ok = false
	}
	return path, ok
}

// findAllPlugins globs {typeDir}/*/version.php for every value in moodletype.PluginTypeToDir and
// dedupes the parent dirs — unsorted, matching generators.findPluginDirs' own "sorting is the
// caller's responsibility" convention.
func findAllPlugins(moodlePath string) []string {
	seen := map[string]struct{}{}
	var dirs []string
	for _, typeDir := range moodletype.PluginTypeToDir {
		matches, _ := filepath.Glob(filepath.Join(moodlePath, typeDir, "*", "version.php"))
		for _, m := range matches {
			d := filepath.Dir(m)
			if _, ok := seen[d]; !ok {
				seen[d] = struct{}{}
				dirs = append(dirs, d)
			}
		}
	}
	return dirs
}

// requireConfig is the shared NOT_INITIALIZED guard. The returned error is non-nil only for a
// genuine failure resolving config (e.g. os.UserHomeDir() failing) —
// never for the ordinary "no config yet" case, which still comes back as (nil, nil) for callers to
// render as NOT_INITIALIZED.
func requireConfig() (*config.Config, error) {
	return config.Load()
}

// readPluginFileTruncated reads a per-plugin generated file (under the plugin's .build82/),
// truncated to maxChars. Returns "" if the file doesn't exist — callers treat that as "not
// available", not an error.
func readPluginFileTruncated(pluginPath, filename string, maxChars int) string {
	return toolutil.ReadFileTruncated(generators.PluginOutputPath(pluginPath, filename), maxChars)
}

func joinPath(parts ...string) string {
	return filepath.Join(parts...)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func relativeToMoodle(moodlePath, absPath string) string {
	rel, err := filepath.Rel(moodlePath, absPath)
	if err != nil {
		return absPath
	}
	return filepath.ToSlash(rel)
}

// displayPathWithinMoodle returns absPath relative to moodlePath (forward slashes) when absPath
// lies inside the Moodle root, and ok=false otherwise — so a caller can pick its own non-leaking
// fallback instead of echoing an absolute host path.
func displayPathWithinMoodle(moodlePath, absPath string) (string, bool) {
	if !moodletype.IsWithinMoodle(absPath, moodlePath) {
		return "", false
	}
	abs, err := filepath.Abs(absPath)
	if err != nil {
		return "", false
	}
	root, err := filepath.Abs(moodlePath)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		// Contained only through a symlink: the lexical relative form would climb out of the
		// root, which is both misleading and not a Moodle-relative path.
		return "", false
	}
	return filepath.ToSlash(rel), true
}
