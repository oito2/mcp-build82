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
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/cache"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/toolutil"
)

type BatchMode string

const (
	BatchModeDev  BatchMode = "dev"
	BatchModeAll  BatchMode = "all"
	BatchModeList BatchMode = "list"
)

type BatchInput struct {
	Mode      BatchMode `json:"mode,omitempty" jsonschema:"'dev' (default) — only .indevelopment plugins. 'all' — every plugin. 'list' — plugins in the plugins parameter."`
	Plugins   []string  `json:"plugins,omitempty" jsonschema:"Required when mode is 'list'. Component (local_myplugin), relative path (local/myplugin), or absolute path per entry."`
	Force     bool      `json:"force,omitempty" jsonschema:"Bypass the mtime cache and regenerate all context files unconditionally."`
	MarkAsDev bool      `json:"mark_as_dev,omitempty" jsonschema:"For mode 'all'/'list', also mark processed plugins with .indevelopment."`
	Parallel  int       `json:"parallel,omitempty" jsonschema:"Opt-in bounded worker pool size (e.g. 4). 0 (default) processes plugins sequentially, avoiding I/O saturation on large installations."`
	Format    Format    `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

const maxBatchParallel = 16

// maxListPlugins caps mode="list"'s Plugins array — without a limit, a single request could force
// thousands of DetectPlugin/GenerateAllForPlugin calls (PHP parsing + file generation each), a
// resource-exhaustion vector independent of the path-traversal concern IsWithinMoodle guards
// against above.
const maxListPlugins = 500

type BatchPluginResult struct {
	Path      string
	Component string
	Generated int
	Skipped   int
	Failed    int
	Error     string
}

func RegisterBatchTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "plugin_batch",
		Description: "Bulk-generates context for multiple plugins at once: 'dev' mode processes every " +
			".indevelopment-marked plugin (default), 'all' processes every plugin in the installation, " +
			"'list' processes an explicit set of plugin identifiers.",
	}, withRecover(handleBatch))
}

func handleBatch(ctx context.Context, req *mcp.CallToolRequest, in BatchInput) (*mcp.CallToolResult, struct{}, error) {
	cfg, err := requireConfig()
	if err != nil {
		return textResult(true, "❌ Failed to resolve build82 configuration: "+err.Error()), struct{}{}, nil
	}
	if cfg == nil {
		return toolutil.NotInitialized(), struct{}{}, nil
	}
	if in.Mode == "" {
		in.Mode = BatchModeDev
	}

	pluginPaths, modeDescription, errResult := resolveBatchPlugins(in, cfg.MoodlePath)
	if errResult != nil {
		return errResult, struct{}{}, nil
	}

	sort.Strings(pluginPaths)
	shouldMarkAsDev := in.Mode == BatchModeDev || in.MarkAsDev

	cache.Global.EnsureLoaded(cfg.MoodlePath)
	var results []BatchPluginResult
	if in.Parallel > 0 {
		results = processBatchParallel(pluginPaths, cfg.MoodlePath, in.Force, shouldMarkAsDev, in.Parallel)
	} else {
		results = make([]BatchPluginResult, 0, len(pluginPaths))
		for _, p := range pluginPaths {
			results = append(results, processPlugin(p, cfg.MoodlePath, in.Force, shouldMarkAsDev))
		}
	}
	if err := cache.Global.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "[build82] warning: failed to persist cache:", err)
	}

	// A failure writing MOODLE_AI_INDEX.md is reported on stderr as a warning.
	if r := generators.GenerateAiIndex(cfg.MoodlePath, cfg.MoodleVersion); !r.Success {
		fmt.Fprintln(os.Stderr, "[build82] warning: failed to update MOODLE_AI_INDEX.md:", r.Error)
	}

	if in.Format == FormatJSON {
		return jsonResult(false, results), struct{}{}, nil
	}
	return textResult(false, renderBatchReport(in, modeDescription, results)), struct{}{}, nil
}

// processBatchParallel runs processPlugin across a bounded worker pool (the adopted improvement),
// preserving the "never abort the batch on one bad plugin" guarantee — each worker's panic/error is
// contained within processPlugin's own recover, exactly as in the sequential path.
func processBatchParallel(pluginPaths []string, moodlePath string, force, markAsDev bool, requested int) []BatchPluginResult {
	n := requested
	if n > maxBatchParallel {
		n = maxBatchParallel
	}
	if n > len(pluginPaths) {
		n = len(pluginPaths)
	}
	if n < 1 {
		n = 1
	}

	results := make([]BatchPluginResult, len(pluginPaths))
	jobs := make(chan int)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			for idx := range jobs {
				results[idx] = processPlugin(pluginPaths[idx], moodlePath, force, markAsDev)
			}
		}()
	}
	for i := range pluginPaths {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results
}

func resolveBatchPlugins(in BatchInput, moodlePath string) ([]string, string, *mcp.CallToolResult) {
	switch in.Mode {
	case BatchModeAll:
		dirs := findAllPlugins(moodlePath)
		if len(dirs) == 0 {
			return nil, "", textResult(true, "❌ No plugins found in this Moodle installation.")
		}
		return dirs, "all plugins", nil

	case BatchModeList:
		if len(in.Plugins) == 0 {
			return nil, "", textResult(true, "❌ mode='list' requires a non-empty 'plugins' array.")
		}
		if len(in.Plugins) > maxListPlugins {
			return nil, "", textResult(true, fmt.Sprintf("❌ too many plugins in 'plugins' (max %d).", maxListPlugins))
		}
		var resolved []string
		var unresolved []string
		for _, id := range in.Plugins {
			// The containment check is required here, not just resolution — this mode is the only
			// batch entry point that accepts caller-supplied absolute/relative paths (dev/all modes
			// only ever glob paths under moodlePath themselves), so it's the one place a crafted
			// identifier (e.g. an absolute path, or one containing "..") could otherwise make
			// GenerateAllForPlugin read/write outside the Moodle installation entirely.
			path, ok := resolvePluginPathWithinMoodle(id, moodlePath)
			if !ok {
				unresolved = append(unresolved, id)
				continue
			}
			resolved = append(resolved, path)
		}
		if len(unresolved) > 0 {
			return nil, "", textResult(true, "❌ Could not resolve the following plugin identifiers: "+strings.Join(unresolved, ", "))
		}
		return resolved, fmt.Sprintf("%d listed plugin(s)", len(resolved)), nil

	default: // dev
		dirs := generators.FindDevPlugins(moodlePath)
		if len(dirs) == 0 {
			return nil, "", textResult(false,
				"ℹ️ No .indevelopment plugins found.\n\nRun `generate_plugin_context` on a plugin to mark it, "+
					"or use mode: 'all' to process every plugin.")
		}
		return dirs, "dev plugins", nil
	}
}

func processPlugin(pluginPath, moodlePath string, force, markAsDev bool) (result BatchPluginResult) {
	result.Path = pluginPath
	result.Component = filepath.Base(pluginPath) // fallback label, overwritten by DetectPlugin's component below on success

	defer func() {
		if r := recover(); r != nil {
			result.Failed = 1
			result.Error = fmt.Sprintf("%v", r)
		}
	}()

	if info, err := extractors.DetectPlugin(pluginPath); err == nil {
		result.Component = info.Component
	}

	if force {
		for _, f := range generators.PluginContextFiles {
			cache.Global.Invalidate(generators.PluginOutputPath(pluginPath, f))
		}
	}

	genResult := generators.GenerateAllForPluginCore(pluginPath, moodlePath, markAsDev, nil)
	for _, f := range genResult.Files {
		switch {
		case !f.Success:
			result.Failed++
		case f.Skipped:
			result.Skipped++
		default:
			result.Generated++
		}
	}
	return result
}

func renderBatchReport(in BatchInput, modeDescription string, results []BatchPluginResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Plugin Batch — %s\n\nTotal: %d, mode: %s, force: %v\n\n", modeDescription, len(results), in.Mode, in.Force)

	var regenerated, cached, failed []BatchPluginResult
	for _, r := range results {
		switch {
		case r.Failed > 0 && r.Generated == 0 && r.Skipped == 0:
			failed = append(failed, r)
		case r.Generated > 0:
			regenerated = append(regenerated, r)
		default:
			cached = append(cached, r)
		}
	}
	fmt.Fprintf(&b, "## Results\n\nRegenerated: %d, Cached: %d, Failed: %d\n\n", len(regenerated), len(cached), len(failed))

	if len(regenerated) > 0 {
		b.WriteString("### Regenerated\n\n")
		for _, r := range regenerated {
			fmt.Fprintf(&b, "✔ %s (%d generated, %d cached, %d failed)\n", r.Component, r.Generated, r.Skipped, r.Failed)
		}
		b.WriteString("\n")
	}
	if len(cached) > 0 {
		b.WriteString("### Cached\n\n")
		for _, r := range cached {
			fmt.Fprintf(&b, "✔ %s (%d cached)\n", r.Component, r.Skipped)
		}
		b.WriteString("\n")
	}
	if len(failed) > 0 {
		b.WriteString("### Failed\n\n")
		for _, r := range failed {
			fmt.Fprintf(&b, "✖ %s: %s\n", r.Component, r.Error)
		}
		b.WriteString("\n")
	}

	stats := cache.Global.Stats()
	fmt.Fprintf(&b, "Cache: %d hits, %d misses, %d skips\n", stats.Hits, stats.Misses, stats.Skips)

	if (in.Mode == BatchModeAll || in.Mode == BatchModeList) && len(regenerated)+len(cached) > 0 {
		if in.MarkAsDev {
			b.WriteString(fmt.Sprintf("\nMarked %d plugin(s) as .indevelopment.\n", len(regenerated)+len(cached)))
		} else {
			b.WriteString("\nTip: pass mark_as_dev: true to also mark these plugins for the watcher.\n")
		}
	}

	return b.String()
}
