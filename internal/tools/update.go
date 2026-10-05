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

type UpdateIndexesInput struct {
	IncludePlugins bool   `json:"include_plugins,omitempty" jsonschema:"Also regenerate context for all .indevelopment plugins"`
	Force          bool   `json:"force,omitempty" jsonschema:"Bypass the cache entirely"`
	Format         Format `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

func RegisterUpdateTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "update_indexes",
		Description: "Regenerates the 13 global index files, re-detecting the Moodle version in case " +
			"the installation was upgraded since init. Optionally also regenerates every .indevelopment plugin.",
	}, withRecover(handleUpdateIndexes))

	mcp.AddTool(server, &mcp.Tool{
		Name: "watch_plugins",
		Description: "Starts, stops, or reports the status of the file-watcher that auto-regenerates " +
			"dev-plugin context on change. Every currently connected session is notified when a " +
			"regeneration completes, not just the one that started the watcher.",
	}, withRecover(makeHandleWatch(server)))
}

func handleUpdateIndexes(ctx context.Context, req *mcp.CallToolRequest, in UpdateIndexesInput) (*mcp.CallToolResult, struct{}, error) {
	cfg, err := requireConfig()
	if err != nil {
		return textResult(true, "❌ Failed to resolve build82 configuration: "+err.Error()), struct{}{}, nil
	}
	if cfg == nil {
		return toolutil.NotInitialized(), struct{}{}, nil
	}

	installInfo := extractors.DetectMoodleInstall(cfg.MoodlePath)
	moodleVersion, moodleFullVersion := cfg.MoodleVersion, cfg.MoodleFullVersion
	if installInfo != nil {
		moodleVersion, moodleFullVersion = installInfo.Version, installInfo.Build
	}
	if moodleVersion != cfg.MoodleVersion || moodleFullVersion != cfg.MoodleFullVersion {
		// A failed config.Save is logged as a warning; ~/.build82 then keeps reporting the old
		// version.
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

	if in.Format == FormatJSON {
		return jsonResult(false, buildUpdateIndexesOutput(cfg.MoodlePath, globalResults, pluginLines)), struct{}{}, nil
	}
	return textResult(false, renderUpdateIndexesReport(cfg.MoodlePath, moodleVersion, globalResults, in.IncludePlugins, pluginLines)), struct{}{}, nil
}

// forceGlobalRegeneration invalidates every output GenerateAll produces, so each one is rebuilt
// regardless of mtimes. The cache is bound to moodlePath first so the invalidation is applied to
// the same state GenerateAll will use; Invalidate itself also survives a later EnsureLoaded.
func forceGlobalRegeneration(moodlePath string) {
	cache.Global.EnsureLoaded(moodlePath)
	for _, f := range generators.GlobalContextFilenames {
		cache.Global.Invalidate(generators.GlobalOutputPath(moodlePath, f))
	}
	// The ctags file is produced by GenerateAll too, but is not one of the Markdown context files.
	cache.Global.Invalidate(generators.GlobalOutputPath(moodlePath, "tags"))
}

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
					line = fmt.Sprintf("✖ %s: %v", dir, r)
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

type UpdateIndexesOutput struct {
	Regenerated []string     `json:"regenerated,omitempty"`
	Skipped     []string     `json:"skipped,omitempty"`
	Failed      []FailedFile `json:"failed,omitempty"`
	Plugins     []string     `json:"plugins,omitempty"`
}

func buildUpdateIndexesOutput(moodlePath string, results []generators.GeneratorResult, pluginLines []string) UpdateIndexesOutput {
	regenerated, skipped, failed := classifyResults(results, moodlePath)
	return UpdateIndexesOutput{Regenerated: regenerated, Skipped: skipped, Failed: failed, Plugins: pluginLines}
}

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

var (
	activeWatcher   *watcher.MoodleWatcher
	activeWatcherMu sync.Mutex
)

type WatchAction string

const (
	WatchStart  WatchAction = "start"
	WatchStop   WatchAction = "stop"
	WatchStatus WatchAction = "status"
)

type WatchInput struct {
	Action WatchAction `json:"action,omitempty" jsonschema:"'start' (default) to begin watching, 'stop' to stop, 'status' to check."`
}

// makeHandleWatch binds the handler to server so its OnChange callback can fan a regeneration
// notification out to every currently connected session, instead of only the one that called
// action=start — in --http mode, every other client working
// against the same Moodle installation is also told when a plugin's context changed.
// server.Sessions() returns a live snapshot (the SDK itself adds/removes sessions on
// connect/disconnect), so a session that connects after the watcher started, or one that
// disconnects before a later change, is picked up or dropped automatically — no manual subscriber
// bookkeeping needed here.
func makeHandleWatch(server *mcp.Server) func(context.Context, *mcp.CallToolRequest, WatchInput) (*mcp.CallToolResult, struct{}, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in WatchInput) (*mcp.CallToolResult, struct{}, error) {
		if in.Action == "" {
			in.Action = WatchStart
		}
		activeWatcherMu.Lock()
		defer activeWatcherMu.Unlock()

		switch in.Action {
		case WatchStatus:
			running := activeWatcher != nil && activeWatcher.Running()
			if running {
				return textResult(false, "✔ Watcher is active."), struct{}{}, nil
			}
			return textResult(false, "Watcher is not running."), struct{}{}, nil

		case WatchStop:
			if activeWatcher == nil {
				return textResult(false, "No active watcher to stop."), struct{}{}, nil
			}
			activeWatcher.Stop()
			activeWatcher = nil
			return textResult(false, "✔ Watcher stopped."), struct{}{}, nil

		default: // start
			if activeWatcher != nil && activeWatcher.Running() {
				return textResult(false, "⚠ Watcher is already running. Use action: 'stop' first."), struct{}{}, nil
			}
			cfg, err := requireConfig()
			if err != nil {
				return textResult(true, "❌ Failed to resolve build82 configuration: "+err.Error()), struct{}{}, nil
			}
			if cfg == nil {
				return toolutil.NotInitialized(), struct{}{}, nil
			}
			if activeWatcher != nil {
				// A previous watcher that has stopped running; discard it before replacing it.
				activeWatcher.Stop()
				activeWatcher = nil
			}
			w := watcher.NewMoodleWatcher(cfg.MoodlePath, cfg.MoodleVersion)
			count := w.Start()
			if count == 0 {
				// Nothing is being watched, so nothing runs: release whatever Start allocated and
				// leave no active watcher behind, so status/stop report the real state.
				w.Stop()
				return textResult(false,
					"ℹ️ Watcher not started — no watchable files found.\n\n"+
						"No .indevelopment plugin with watchable source files (version.php, db/*.php, ...) exists. "+
						"Run `generate_plugin_context` on a plugin (or `plugin_batch` with mark_as_dev: true) to mark it, "+
						"then start the watcher again."), struct{}{}, nil
			}
			activeWatcher = w

			activeWatcher.OnChange(func(ev watcher.WatchEvent) {
				// context.Background(), not the "start" call's own ctx: this callback fires later,
				// asynchronously, whenever a future filesystem event happens — potentially long
				// after the tool call that registered it has already returned and its request
				// context been cancelled. Using that stale ctx would make every session.Log call
				// fail, so no session would receive the notification.
				logCtx := context.Background()
				for session := range server.Sessions() {
					_ = session.Log(logCtx, &mcp.LoggingMessageParams{ // best-effort; ignore error (e.g. transport closed)
						Level:  "info",
						Logger: "build82/watcher",
						Data:   fmt.Sprintf("[watcher] %s — context regenerated (%s)", ev.Component, ev.File),
					})
				}
			})

			return textResult(false, fmt.Sprintf(
				"✅ Watcher started — monitoring %d files across dev plugins.\n\n"+
					"Context will be regenerated automatically when db/*.php or version.php change.\n"+
					"Use `watch_plugins action='stop'` to stop.", count)), struct{}{}, nil
		}
	}
}
