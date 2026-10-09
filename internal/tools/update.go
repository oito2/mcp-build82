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
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/cache"
	"github.com/oito2/mcp-build82/internal/config"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/toolutil"
	"github.com/oito2/mcp-build82/internal/watcher"
)

// --- update_indexes -----------------------------------------------------------

// UpdateIndexesInput is the input of the update_indexes tool.
type UpdateIndexesInput struct {
	IncludePlugins bool   `json:"include_plugins,omitempty" jsonschema:"Also regenerate context for all .indevelopment plugins"`
	Force          bool   `json:"force,omitempty" jsonschema:"Bypass the cache entirely"`
	Format         Format `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

// RegisterUpdateTool registers the update_indexes and watch_plugins tools on `server`.
func RegisterUpdateTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_indexes",
		Annotations: toolAnnotations("Update Indexes", false, false, true, false),
		Description: "Regenerates the 13 global index files, re-detecting the Moodle version in case " +
			"the installation was upgraded since init. Optionally also regenerates every .indevelopment plugin.",
	}, withRecover(handleUpdateIndexes))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "watch_plugins",
		Annotations: toolAnnotations("Watch Dev Plugins", false, false, true, false),
		Description: "Starts, stops, or reports the status of the file-watcher that auto-regenerates " +
			"dev-plugin context on change. Every currently connected session is notified when a " +
			"regeneration completes, not just the one that started the watcher.",
	}, withRecover(makeHandleWatch(server)))
}

// handleUpdateIndexes re-detects the Moodle version (persisting it when it changed) and
// regenerates the global indexes, and the .indevelopment plugins when `in.IncludePlugins` is set.
// `in.Force` bypasses the cache. The structured output is an UpdateIndexesOutput in either format.
// A missing or invalid configuration is returned as an error, so that result has no structured
// output.
func handleUpdateIndexes(ctx context.Context, req *mcp.CallToolRequest, in UpdateIndexesInput) (*mcp.CallToolResult, UpdateIndexesOutput, error) {
	cfg, err := requireConfig()
	if err != nil {
		return nil, UpdateIndexesOutput{}, configError(err)
	}
	if cfg == nil {
		return nil, UpdateIndexesOutput{}, resultError(toolutil.NotInitialized())
	}

	installInfo := extractors.DetectMoodleInstall(cfg.MoodlePath)
	moodleVersion, moodleFullVersion := cfg.MoodleVersion, cfg.MoodleFullVersion
	if installInfo != nil {
		moodleVersion, moodleFullVersion = installInfo.Version, installInfo.Build
	}
	if moodleVersion != cfg.MoodleVersion || moodleFullVersion != cfg.MoodleFullVersion {
		// A failed save is only logged, so the stored configuration keeps the old version.
		if err := config.Save(config.Config{MoodlePath: cfg.MoodlePath, MoodleVersion: moodleVersion, MoodleFullVersion: moodleFullVersion}); err != nil {
			fmt.Fprintln(os.Stderr, "[build82] warning: failed to persist updated Moodle version:", err)
		}
	}

	if in.Force {
		forceGlobalRegeneration(cfg.MoodlePath)
	}

	globalResults := generators.GenerateAll(cfg.MoodlePath, moodleVersion)

	var pluginLines []string
	if in.IncludePlugins {
		pluginLines = updatePlugins(cfg.MoodlePath, in.Force)
	}

	out := buildUpdateIndexesOutput(cfg.MoodlePath, moodleVersion, globalResults, pluginLines)
	return structuredResult(in.Format, false, renderUpdateIndexesReport(cfg.MoodlePath, moodleVersion, globalResults, in.IncludePlugins, pluginLines), out)
}

// forceGlobalRegeneration invalidates the cache entries of every output GenerateAll produces for
// `moodlePath`, so each one is rebuilt regardless of modification times. The cache is bound to
// `moodlePath` first so the invalidation applies to the state GenerateAll will use.
func forceGlobalRegeneration(moodlePath string) {
	cache.Global.EnsureLoaded(moodlePath)
	for _, f := range generators.GlobalContextFilenames {
		cache.Global.Invalidate(generators.GlobalOutputPath(moodlePath, f))
	}
	// The ctags file is produced by GenerateAll too, but is not one of the Markdown context files.
	cache.Global.Invalidate(generators.GlobalOutputPath(moodlePath, "tags"))
}

// pluginPanicLine formats the summary line for a plugin whose processing panicked with `r`,
// naming the plugin directory relative to `moodlePath`.
func pluginPanicLine(moodlePath, dir string, r any) string {
	return fmt.Sprintf("✖ %s: %v", relativeToMoodle(moodlePath, dir), r)
}

// updatePlugins regenerates the context of every .indevelopment plugin under `moodlePath`
// (bypassing the cache when `force` is set), persists the cache, and returns one summary line per
// plugin. A panic while processing a plugin is recovered and reported in that plugin's line.
func updatePlugins(moodlePath string, force bool) []string {
	devDirs := generators.FindDevPlugins(moodlePath)
	lines := make([]string, 0, len(devDirs))

	cache.Global.EnsureLoaded(moodlePath)
	defer func() {
		if err := cache.Global.Save(); err != nil {
			fmt.Fprintln(os.Stderr, "[build82] warning: failed to persist cache:", err)
		}
	}()

	for _, dir := range devDirs {
		lines = append(lines, func() (line string) {
			defer func() {
				if r := recover(); r != nil {
					line = pluginPanicLine(moodlePath, dir, r)
				}
			}()
			if force {
				for _, f := range generators.PluginContextFiles {
					cache.Global.Invalidate(generators.PluginOutputPath(dir, f))
				}
			}
			result := generators.GenerateAllForPluginCore(dir, moodlePath, true, nil)
			ok, fail := 0, 0
			for _, f := range result.Files {
				if f.Success && !f.Skipped {
					ok++
				} else if !f.Success {
					fail++
				}
			}
			icon := "✔"
			if fail > 0 {
				icon = "⚠"
			}
			skipped := len(result.Files) - ok - fail
			return fmt.Sprintf("%s %s (%d regenerated, %d cached, %d failed)", icon, result.Plugin, ok, skipped, fail)
		}())
	}
	return lines
}

// UpdateIndexesOutput is the structured output of update_indexes: the detected Moodle version, the
// global index files regenerated, served from the cache and failed (relative to the Moodle root),
// and one summary line per .indevelopment plugin when include_plugins is set.
type UpdateIndexesOutput struct {
	MoodleVersion string       `json:"moodle_version"`
	Regenerated   []string     `json:"regenerated,omitempty"`
	Skipped       []string     `json:"skipped,omitempty"`
	Failed        []FailedFile `json:"failed,omitempty"`
	Plugins       []string     `json:"plugins,omitempty"`
}

// buildUpdateIndexesOutput builds the structured output from the detected `moodleVersion`, the
// global generator `results` (file names relative to `moodlePath`) and the per-plugin summary
// `pluginLines`.
func buildUpdateIndexesOutput(moodlePath, moodleVersion string, results []generators.GeneratorResult, pluginLines []string) UpdateIndexesOutput {
	regenerated, skipped, failed := classifyResults(results, moodlePath)
	return UpdateIndexesOutput{MoodleVersion: moodleVersion, Regenerated: regenerated, Skipped: skipped, Failed: failed, Plugins: pluginLines}
}

// renderUpdateIndexesReport renders the Markdown report: global index counts, the dev plugin
// summary lines when `includePlugins` is set, and cache statistics.
func renderUpdateIndexesReport(moodlePath, moodleVersion string, results []generators.GeneratorResult, includePlugins bool, pluginLines []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Update Indexes\n\nMoodle version: %s\n\n", moodleVersion)

	regenerated, skipped, failed := classifyResults(results, moodlePath)
	fmt.Fprintf(&b, "## Global Indexes\n\nRegenerated: %d, Cached: %d, Failed: %d\n\n", len(regenerated), len(skipped), len(failed))

	if includePlugins {
		b.WriteString("## Dev Plugins\n\n")
		if len(pluginLines) == 0 {
			b.WriteString("_(no .indevelopment plugins found)_\n\n")
		} else {
			for _, l := range pluginLines {
				fmt.Fprintf(&b, "%s\n", l)
			}
			b.WriteString("\n")
		}
	}

	stats := cache.Global.Stats()
	fmt.Fprintf(&b, "Cache: %d hits, %d misses, %d skips\n", stats.Hits, stats.Misses, stats.Skips)
	return b.String()
}

// --- watch_plugins --------------------------------------------------------------

// activeWatcher is the process-wide file watcher started by watch_plugins, guarded by
// activeWatcherMu.
var (
	activeWatcher   *watcher.MoodleWatcher
	activeWatcherMu sync.Mutex
)

// startWatcher starts `w` and returns the number of watched files. Tests replace it to act on
// the watcher before the watch_plugins handler continues.
var startWatcher = (*watcher.MoodleWatcher).Start

// WatchAction is the operation requested from the watch_plugins tool.
type WatchAction string

// Supported values of WatchAction.
const (
	WatchStart  WatchAction = "start"
	WatchStop   WatchAction = "stop"
	WatchStatus WatchAction = "status"
)

// WatchInput is the input of the watch_plugins tool.
type WatchInput struct {
	Action WatchAction `json:"action,omitempty" jsonschema:"'start' (default) to begin watching, 'stop' to stop, 'status' to check."`
}

// makeHandleWatch returns the watch_plugins handler bound to `server`. The handler starts, stops
// or reports the status of the single process-wide watcher. When the watcher regenerates a
// plugin's context, a log notification is sent to every session connected at that moment, not only
// the one that started it. Starting returns an informational result when no watchable files exist;
// a missing or invalid configuration yields an error result. The error return is always nil.
func makeHandleWatch(server *mcp.Server) func(context.Context, *mcp.CallToolRequest, WatchInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in WatchInput) (*mcp.CallToolResult, any, error) {
		if in.Action == "" {
			in.Action = WatchStart
		}
		activeWatcherMu.Lock()
		defer activeWatcherMu.Unlock()

		switch in.Action {
		case WatchStatus:
			running := activeWatcher != nil && activeWatcher.Running()
			if running {
				return textResult(false, "✔ Watcher is active."), nil, nil
			}
			return textResult(false, "Watcher is not running."), nil, nil

		case WatchStop:
			if activeWatcher == nil {
				return textResult(false, "No active watcher to stop."), nil, nil
			}
			activeWatcher.Stop()
			activeWatcher = nil
			return textResult(false, "✔ Watcher stopped."), nil, nil

		default: // start
			if activeWatcher != nil && activeWatcher.Running() {
				return textResult(false, "⚠ Watcher is already running. Use action: 'stop' first."), nil, nil
			}
			cfg, err := requireConfig()
			if err != nil {
				return textResult(true, "❌ Failed to resolve build82 configuration: "+err.Error()), nil, nil
			}
			if cfg == nil {
				return toolutil.NotInitialized(), nil, nil
			}
			if activeWatcher != nil {
				// A previous watcher that has stopped running; discard it before replacing it.
				activeWatcher.Stop()
				activeWatcher = nil
			}
			w := watcher.NewMoodleWatcher(cfg.MoodlePath, cfg.MoodleVersion)
			w.OnChange(func(ev watcher.WatchEvent) {
				// The callback fires asynchronously, possibly long after this tool call returned and
				// its request context was cancelled, so it uses a fresh background context.
				logCtx := context.Background()
				for session := range server.Sessions() {
					// MCP logging is deprecated but still part of the protocol; it is the channel through
					// which every connected client is told about a regeneration.
					_ = session.Log(logCtx, &mcp.LoggingMessageParams{ //nolint:staticcheck // best-effort; ignore error (e.g. transport closed)
						Level:  "info",
						Logger: "build82/watcher",
						Data:   fmt.Sprintf("[watcher] %s — context regenerated (%s)", ev.Component, ev.File),
					})
				}
			})
			count := startWatcher(w)
			if count == 0 {
				// Nothing is being watched, so nothing runs: release whatever Start allocated and
				// leave no active watcher behind, so status/stop report the real state.
				w.Stop()
				return textResult(false,
					"ℹ️ Watcher not started — no watchable files found.\n\n"+
						"No .indevelopment plugin with watchable source files (version.php, db/*.php, ...) exists. "+
						"Run `generate_plugin_context` on a plugin (or `plugin_batch` with mark_as_dev: true) to mark it, "+
						"then start the watcher again."), nil, nil
			}
			activeWatcher = w

			return textResult(false, fmt.Sprintf(
				"✅ Watcher started — monitoring %d files across dev plugins.\n\n"+
					"Context will be regenerated automatically when db/*.php or version.php change.\n"+
					"Use `watch_plugins action='stop'` to stop.", count)), nil, nil
		}
	}
}
