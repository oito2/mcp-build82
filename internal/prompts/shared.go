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

package prompts

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/config"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/moodletype"
	"github.com/oito2/mcp-build82/internal/toolutil"
)

// maxPromptArgLen caps any free-text prompt argument (e.g. debug_plugin's "error"/"context",
// review_plugin's "files", scaffold_plugin's "description"/"features") before it's interpolated
// into the final prompt string. Without a cap, a potentially untrusted client could force an
// arbitrarily large prompt from a single request; 20,000 characters comfortably fits a sizeable
// stack trace or feature list while bounding the worst case.
const maxPromptArgLen = 20_000

// truncateArg caps a free-text prompt argument at maxPromptArgLen, appending a visible marker so
// the truncation is never silent to whoever reads the rendered prompt. Fields with an already
// restrictive format (an enum like review_plugin's "focus", or a short identifier like
// scaffold_plugin's "type"/"name") don't need this — only genuinely free-text, potentially large
// fields do.
func truncateArg(s string) string {
	if len(s) > maxPromptArgLen {
		return s[:maxPromptArgLen] + "\n...(truncated)"
	}
	return s
}

// withRecoverPrompt wraps a prompt handler so a panic anywhere inside it (most likely deep in
// resolvePluginForPrompt/extractors.DetectPlugin, triggered by a malformed or adversarial plugin
// directory) becomes a normal error return instead of an unrecovered panic. The SDK's prompt
// dispatch does not recover from a panicking PromptHandler, so an unrecovered panic would kill the
// whole server process for every connected client/session, not just the request that triggered it.
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

// requireArgs checks that every name in required has a non-empty value in args, returning a
// jsonrpc.Error (mcp.GetPromptHandler's SDK-native way to signal a bad request) naming every
// missing one at once. The SDK's prompt dispatch does not validate a Prompt's Required argument
// declarations before invoking the handler, so this check turns an omitted required argument (e.g.
// "type"/"name"/"description" for scaffold_plugin, "plugin" for review_plugin, "plugin"/"error"
// for debug_plugin) into a clear error instead of a prompt rendered from empty strings.
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

// readFileTruncated reads at most maxChars bytes from path, avoiding buffering an entire large
// file (real lib.php files can run hundreds of KB) just to discard everything past the truncation
// point. Thin wrapper over toolutil.ReadFileTruncated.
func readFileTruncated(path string, maxChars int) string {
	return toolutil.ReadFileTruncated(path, maxChars)
}

// resolvedPluginContext is the outcome of resolvePluginForPrompt — unlike tools, prompts never
// hard-require config: if resolution fails (or config is unset), the raw plugin argument is used
// as a literal path so the prompt still renders something useful.
type resolvedPluginContext struct {
	Path            string
	Component       string
	Type            string
	Version         string
	MoodleVersion   string
	MoodlePath      string
	ConfigAvailable bool
}

// resolvePluginForPrompt mirrors review_plugin/debug_plugin's shared resolution logic: resolve via
// moodletype.ResolvePluginPath, falling back to the raw string as a literal path if resolution
// fails and config is unset. Live-detects metadata, silently ignoring detection failure (keeps
// defaults empty).
//
// A config.Load error (e.g. os.UserHomeDir() failing) is deliberately folded into the same
// "no config available" branch as cfg == nil, not surfaced as a hard failure: prompts never
// hard-require config, and config.Load returns an explicit error rather than building a wrong path
// from an empty home directory, so this is the same graceful fallback used when no config file
// exists yet.
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
		// A resolved/fallback path outside the Moodle root must never be treated as a real
		// filesystem path — plugin is client-controlled (review_plugin/debug_plugin arguments),
		// and reading an arbitrary external file's content/metadata into a prompt response is
		// exactly the leak IsWithinMoodle exists to prevent.
		// rc.Path is deliberately left "" in that case: falling back to the *raw* plugin argument
		// as a path is NOT safe, since the raw argument is itself the escaping value in the
		// realistic attack (an absolute path outside the root).
		// DetectPlugin is skipped below when rc.Path is empty; rc.Component still falls back to the
		// raw string afterward, for display purposes only, never as a filesystem path.
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

func (rc resolvedPluginContext) readPluginFileTruncated(filename string, maxChars int) string {
	return readFileTruncated(generators.PluginOutputPath(rc.Path, filename), maxChars)
}
