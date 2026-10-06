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

// textResult builds a single-text-block tool response with the given `text`, flagged as an error
// when `isError` is true.
func textResult(isError bool, text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: isError,
	}
}

// withRecover wraps the tool handler `fn` so that a panic anywhere inside it (most likely deep in
// an extractor or generator fed malformed plugin source) is converted into an IsError text result
// instead of propagating. The MCP SDK's tool dispatch does not recover panics, so an unrecovered
// one would terminate the whole server process for every connected client. Every registered tool
// is wrapped with it.
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

// Format is the optional output-format field shared by every tool's input struct.
// "text" (the zero value) renders Markdown/plain text; "json" renders the same underlying
// data as a JSON string in that same single text block.
type Format string

// Supported values of Format.
const (
	FormatText Format = ""
	FormatJSON Format = "json"
)

// jsonResult marshals `data` as indented JSON into a single text block flagged as an error when
// `isError` is true. If marshaling fails, it returns an error result describing the failure.
func jsonResult(isError bool, data any) *mcp.CallToolResult {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return textResult(true, "❌ Failed to encode JSON response: "+err.Error())
	}
	return textResult(isError, string(b))
}

// FailedFile is the JSON shape of one file that failed to generate, used by every tool whose
// output distinguishes generated, skipped and failed generator results.
type FailedFile struct {
	File  string `json:"file"`
	Error string `json:"error"`
}

// classifyResults splits generator `results` into the names of generated files, the names of
// skipped (cached) files, and the records of failed files. Every file name is made relative to
// `base` (the Moodle root for global generators, the plugin's own path for per-plugin ones) so the
// output never exposes an absolute host path.
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

// resolvedPlugin is the outcome of resolving and validating a plugin identifier: the absolute
// plugin directory and its detected metadata.
type resolvedPlugin struct {
	Path string
	Info extractors.PluginInfo
}

// resolveAndValidatePlugin resolves `identifier` — a frankenstyle component ("local_myplugin"), a
// path relative to the Moodle root ("local/myplugin"), or an absolute path — against `moodlePath`,
// and checks that it lies within the Moodle root, exists, and contains a version.php. On any
// failure it returns a non-nil error result (IsError set) that the caller should return as is; on
// success the result is nil and the resolvedPlugin is populated.
func resolveAndValidatePlugin(identifier, moodlePath string) (resolvedPlugin, *mcp.CallToolResult) {
	full, _ := filepath.Abs(resolvePluginIdentifier(identifier, moodlePath))

	// The messages below never embed moodlePath or the absolute `full` path: absolute host paths
	// reveal the directory structure (often an OS username), so any path shown is first made
	// relative to moodlePath via relativeToMoodle.
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

// resolvePluginIdentifier maps `identifier` to a filesystem path without checking containment or
// existence, so callers must still run moodletype.IsWithinMoodle on the result. It first tries
// moodletype.ResolvePluginPath (component, relative, or absolute, existing paths only); when that
// finds nothing, it treats `identifier` as a path (absolute, or joined onto `moodlePath`). That
// way a directory whose name merely looks like a component (e.g. a literal "local_x" folder) still
// resolves, and a missing plugin yields a meaningful "not found" message.
func resolvePluginIdentifier(identifier, moodlePath string) string {
	if path, ok := moodletype.ResolvePluginPath(identifier, moodlePath); ok {
		return path
	}
	if filepath.IsAbs(identifier) {
		return identifier
	}
	return filepath.Join(moodlePath, identifier)
}

// resolvePluginPathWithinMoodle resolves `identifier` (component, relative path, or absolute path)
// via moodletype.ResolvePluginPath and checks it with moodletype.IsWithinMoodle against
// `moodlePath`. Unlike resolveAndValidatePlugin it does not require version.php to exist. It
// returns the resolved path and true, or ("", false) when resolution or containment fails.
func resolvePluginPathWithinMoodle(identifier, moodlePath string) (string, bool) {
	path, ok := moodletype.ResolvePluginPath(identifier, moodlePath)
	if ok && !moodletype.IsWithinMoodle(path, moodlePath) {
		ok = false
	}
	return path, ok
}

// findAllPlugins returns the de-duplicated plugin directories under `moodlePath`, found by globbing
// {typeDir}/*/version.php for every value in moodletype.PluginTypeToDir. The result is unsorted;
// sorting is the caller's responsibility.
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

// requireConfig loads the build82 configuration. It returns (nil, nil) when no configuration
// exists yet, which callers report with toolutil.NotInitialized, and a non-nil error only when the
// configuration location cannot be resolved or the file cannot be read.
func requireConfig() (*config.Config, error) {
	return config.Load()
}

// readPluginFileTruncated reads the generated file `filename` from the plugin's .build82/
// directory (`pluginPath` is the plugin root), truncated to `maxChars` bytes. It returns "" when
// the file is missing or unreadable; callers treat that as "not available".
func readPluginFileTruncated(pluginPath, filename string, maxChars int) string {
	return toolutil.ReadFileTruncated(generators.PluginOutputPath(pluginPath, filename), maxChars)
}

// joinPath joins path `parts` with the OS path separator.
func joinPath(parts ...string) string {
	return filepath.Join(parts...)
}

// fileExists reports whether `path` exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dirExists reports whether `path` exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// relativeToMoodle returns `absPath` relative to `moodlePath` with forward slashes, or `absPath`
// unchanged when no relative form exists.
func relativeToMoodle(moodlePath, absPath string) string {
	rel, err := filepath.Rel(moodlePath, absPath)
	if err != nil {
		return absPath
	}
	return filepath.ToSlash(rel)
}

// displayPathWithinMoodle returns `absPath` relative to `moodlePath` (forward slashes) and true
// when `absPath` lies inside the Moodle root, and ("", false) otherwise, so a caller can pick its
// own fallback instead of echoing an absolute host path.
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
