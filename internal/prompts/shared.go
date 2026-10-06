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
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/config"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/moodletype"
	"github.com/oito2/mcp-build82/internal/toolutil"
)

// maxPromptArgLen is the maximum length, in bytes, of a free-text prompt argument (e.g.
// debug_plugin's "error"/"context", review_plugin's "files", scaffold_plugin's
// "description"/"features") before it is interpolated into the rendered prompt. The cap bounds the
// prompt size an untrusted client can force with one request while still fitting a sizeable stack
// trace or feature list.
const maxPromptArgLen = 20_000

// truncateArg returns `s` unchanged when it is at most maxPromptArgLen bytes long; otherwise it
// returns the longest rune-aligned prefix of at most maxPromptArgLen bytes followed by a "...(truncated)" marker, so truncation is
// visible in the rendered prompt. It is meant for free-text arguments only; enum-like or
// identifier-like arguments (e.g. "focus", "type", "name") do not need it.
func truncateArg(s string) string {
	if len(s) > maxPromptArgLen {
		cut := maxPromptArgLen
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		return s[:cut] + "\n...(truncated)"
	}
	return s
}

// withRecoverPrompt wraps the prompt handler `fn` so that a panic inside it (most likely in
// resolvePluginForPrompt/extractors.DetectPlugin on a malformed or adversarial plugin directory)
// is returned as an error with a nil result instead of propagating. The SDK does not recover from
// a panicking PromptHandler, so an unrecovered panic would terminate the whole server process for
// every connected session. Results and errors from a non-panicking `fn` pass through unchanged.
func withRecoverPrompt(fn mcp.PromptHandler) mcp.PromptHandler {
	return func(ctx context.Context, req *mcp.GetPromptRequest) (result *mcp.GetPromptResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				result = nil
				err = fmt.Errorf("internal error while rendering this prompt: %v", r)
			}
		}()
		return fn(ctx, req)
	}
}

// requireArgs checks that every name in `required` has a non-blank (after trimming whitespace)
// value in `args`. It returns nil when all are present, or a jsonrpc invalid-params error naming
// every missing argument at once. The SDK does not enforce a Prompt's Required declarations before
// invoking the handler, so this check prevents a prompt from being rendered from empty values.
func requireArgs(args map[string]string, required ...string) error {
	var missing []string
	for _, name := range required {
		if strings.TrimSpace(args[name]) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return &jsonrpc.Error{
		Code:    jsonrpc.CodeInvalidParams,
		Message: "missing required argument(s): " + strings.Join(missing, ", "),
	}
}

// readFileTruncated returns at most `maxChars` bytes of the file at `path` without buffering the
// whole file. It is a thin wrapper over toolutil.ReadFileTruncated and follows its behavior for
// unreadable files.
func readFileTruncated(path string, maxChars int) string {
	return toolutil.ReadFileTruncated(path, maxChars)
}

// resolvedPluginContext is the outcome of resolvePluginForPrompt. Unlike tools, prompts never
// require a configuration: Path is empty when the plugin could not be safely resolved, and
// Component then falls back to the raw plugin argument for display only.
type resolvedPluginContext struct {
	Path            string
	Component       string
	Type            string
	Version         string
	MoodleVersion   string
	MoodlePath      string
	ConfigAvailable bool
}

// resolvePluginForPrompt resolves the client-supplied `plugin` argument (a component, a path
// relative to the Moodle root, or an absolute path) into a resolvedPluginContext shared by
// review_plugin and debug_plugin. When no configuration is available the argument is used as a
// literal path; otherwise it is resolved through moodletype.ResolvePluginPath against the
// configured Moodle root. Plugin metadata is then detected from disk, and a detection failure
// leaves the metadata fields empty. It never returns an error or panics on its own.
//
// A config.Load error is treated the same as a missing configuration (cfg == nil) instead of
// failing, because prompts must still render without one.
func resolvePluginForPrompt(plugin string) resolvedPluginContext {
	var rc resolvedPluginContext

	cfg, err := config.Load()
	if err != nil || cfg == nil {
		rc.Path = plugin // no config to validate against, so the plugin path is used as given
	} else {
		rc.ConfigAvailable = true
		rc.MoodlePath = cfg.MoodlePath
		rc.MoodleVersion = cfg.MoodleVersion
		candidate, ok := moodletype.ResolvePluginPath(plugin, cfg.MoodlePath)
		if !ok {
			candidate = filepath.Join(cfg.MoodlePath, plugin)
		}
		// The plugin argument is client-controlled, so a path outside the Moodle root must not
		// be read: its metadata and file content would leak into the prompt. In that case
		// rc.Path stays empty (the raw argument is not a safe fallback, since it is the escaping
		// value itself), DetectPlugin is skipped, and rc.Component falls back to the raw string
		// for display only.
		if moodletype.IsWithinMoodle(candidate, cfg.MoodlePath) {
			rc.Path = candidate
		}
	}

	if rc.Path != "" {
		if info, err := extractors.DetectPlugin(rc.Path); err == nil {
			rc.Component, rc.Type, rc.Version = info.Component, info.Type, info.Version
		}
	}
	if rc.Component == "" {
		rc.Component = plugin
	}

	return rc
}

// readPluginFileTruncated returns at most `maxChars` bytes of the generated plugin file named
// `filename` under the plugin's output directory, or an empty string when rc.Path is empty or the
// file cannot be read.
func (rc resolvedPluginContext) readPluginFileTruncated(filename string, maxChars int) string {
	return readFileTruncated(generators.PluginOutputPath(rc.Path, filename), maxChars)
}
