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

	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/moodletype"
	"github.com/oito2/mcp-build82/internal/toolutil"
)

// maxQueryLen caps search_plugins/search_api's Query — fuzzySearchInFile runs a Levenshtein
// comparison (O(len(word) * len(query))) against every word of every line of the index file, so an
// unbounded query lets a caller force an arbitrarily large amount of CPU work on a single request.
const maxQueryLen = 200

// --- shared index cache --------------------------------------------------------

type indexCacheEntry struct {
	mtime int64
	lines []string
}

var (
	indexCache   = map[string]indexCacheEntry{}
	indexCacheMu sync.Mutex
)

const indexCacheMax = 50

// cachedLines returns filePath's content split into lines, from the shared mtime-invalidated
// cache — the stat+read+cache-or-refresh logic both searchInFile and fuzzySearchInFile share.
// Caching matters most for fuzzySearchInFile, which runs a full Levenshtein comparison against
// every word of every line on the zero-exact-match path. Guarded by a mutex — Go tool handlers
// may run concurrently across sessions.
func cachedLines(filePath string) []string {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil
	}
	mtime := info.ModTime().UnixNano()

	indexCacheMu.Lock()
	defer indexCacheMu.Unlock()

	entry, ok := indexCache[filePath]
	if ok && entry.mtime == mtime {
		return entry.lines
	}
	if len(indexCache) >= indexCacheMax {
		indexCache = map[string]indexCacheEntry{}
	}
	content, readErr := os.ReadFile(filePath)
	if readErr != nil {
		return nil
	}
	entry = indexCacheEntry{mtime: mtime, lines: strings.Split(string(content), "\n")}
	indexCache[filePath] = entry
	return entry.lines
}

// searchInFile is case-insensitive substring match over every line of the given file.
func searchInFile(filePath, query string) []string {
	lines := cachedLines(filePath)
	if lines == nil {
		return nil
	}
	lowerQ := strings.ToLower(query)
	var out []string
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), lowerQ) {
			out = append(out, line)
		}
	}
	return out
}

// fuzzySearchInFile is a fuzzy/typo-tolerant fallback — only tried when the exact substring match
// returns zero results, and never replaces it. Uses a simple token-overlap heuristic: a line
// matches if it contains a word whose Levenshtein distance to the query is small relative to the
// query's length.
func fuzzySearchInFile(filePath, query string) []string {
	lines := cachedLines(filePath)
	if lines == nil {
		return nil
	}
	lowerQ := strings.ToLower(query)
	threshold := maxEditDistance(len(lowerQ))

	var out []string
	for _, line := range lines {
		lowerLine := strings.ToLower(line)
		for _, word := range wordSplit(lowerLine) {
			if levenshtein(word, lowerQ) <= threshold {
				out = append(out, line)
				break
			}
		}
	}
	return out
}

func maxEditDistance(queryLen int) int {
	switch {
	case queryLen <= 3:
		return 0 // too short to fuzz safely
	case queryLen <= 6:
		return 1
	default:
		return 2
	}
}

func wordSplit(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_')
	})
}

func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := cur[j-1] + 1
			sub := prev[j-1] + cost
			cur[j] = min3(del, ins, sub)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

// --- 5a. search_plugins ---------------------------------------------------------

type SearchPluginsInput struct {
	Query  string `json:"query" jsonschema:"Search term (component name, plugin type, etc.)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Max results, 1-100 (default 20)"`
	Format Format `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

func RegisterSearchTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_plugins",
		Description: "Searches the plugin index (component, type, name, version, path) for a query string.",
	}, withRecover(handleSearchPlugins))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_api",
		Description: "Searches the public Moodle API index (lib/ functions) for a query string, filterable by visibility.",
	}, withRecover(handleSearchApi))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_plugin_info",
		Description: "Returns the generated AI context for a plugin (or live-detected metadata if not yet generated).",
	}, withRecover(handleGetPluginInfo))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_dev_plugins",
		Description: "Lists every plugin currently marked .indevelopment.",
	}, withRecover(handleListDevPlugins))
}

func normalizeLimit(limit, def int) int {
	if limit <= 0 {
		return def
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func handleSearchPlugins(ctx context.Context, req *mcp.CallToolRequest, in SearchPluginsInput) (*mcp.CallToolResult, struct{}, error) {
	cfg, err := requireConfig()
	if err != nil {
		return textResult(true, "❌ Failed to resolve build82 configuration: "+err.Error()), struct{}{}, nil
	}
	if cfg == nil {
		return toolutil.NotInitialized(), struct{}{}, nil
	}
	if len(in.Query) > maxQueryLen {
		return textResult(true, fmt.Sprintf("❌ query too long (max %d characters).", maxQueryLen)), struct{}{}, nil
	}
	limit := normalizeLimit(in.Limit, 20)

	indexPath := generators.GlobalOutputPath(cfg.MoodlePath, "MOODLE_PLUGIN_INDEX.md")
	if _, err := os.Stat(indexPath); err != nil {
		return textResult(true, "❌ Plugin index not found. Run `init_moodle_context` or `update_indexes` first."), struct{}{}, nil
	}

	matches := filterTableRows(searchInFile(indexPath, in.Query))
	fuzzy := false
	if len(matches) == 0 {
		matches = filterTableRows(fuzzySearchInFile(indexPath, in.Query))
		fuzzy = len(matches) > 0
	}
	if len(matches) > limit {
		matches = matches[:limit]
	}

	if in.Format == FormatJSON {
		return jsonResult(false, map[string]any{"query": in.Query, "fuzzy": fuzzy, "matches": matches}), struct{}{}, nil
	}
	if len(matches) == 0 {
		return textResult(false, fmt.Sprintf("No plugins matched %q.", in.Query)), struct{}{}, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Plugin Search — %q\n\n", in.Query)
	if fuzzy {
		b.WriteString("_(no exact matches — showing fuzzy matches)_\n\n")
	}
	b.WriteString("| Component | Type | Name | Version | Path |\n|---|---|---|---|---|\n")
	for _, m := range matches {
		fmt.Fprintf(&b, "%s\n", m)
	}
	return textResult(false, b.String()), struct{}{}, nil
}

// filterTableRows keeps only lines starting with "|" and not containing "Component" (the header
// row).
func filterTableRows(lines []string) []string {
	var out []string
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "|") && !strings.Contains(l, "Component") {
			out = append(out, l)
		}
	}
	return out
}

// --- 5b. search_api ---------------------------------------------------------------

type ApiVisibilityFilter string

const (
	ApiVisPublic     ApiVisibilityFilter = "public"
	ApiVisDeprecated ApiVisibilityFilter = "deprecated"
	ApiVisAll        ApiVisibilityFilter = "all"
)

type SearchApiInput struct {
	Query      string              `json:"query" jsonschema:"Search term (function name, keyword in summary, etc.)"`
	Visibility ApiVisibilityFilter `json:"visibility,omitempty" jsonschema:"'public' (default), 'deprecated', or 'all'"`
	Limit      int                 `json:"limit,omitempty" jsonschema:"Max results, 1-100 (default 30)"`
	Format     Format              `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

func handleSearchApi(ctx context.Context, req *mcp.CallToolRequest, in SearchApiInput) (*mcp.CallToolResult, struct{}, error) {
	cfg, err := requireConfig()
	if err != nil {
		return textResult(true, "❌ Failed to resolve build82 configuration: "+err.Error()), struct{}{}, nil
	}
	if cfg == nil {
		return toolutil.NotInitialized(), struct{}{}, nil
	}
	if len(in.Query) > maxQueryLen {
		return textResult(true, fmt.Sprintf("❌ query too long (max %d characters).", maxQueryLen)), struct{}{}, nil
	}
	if in.Visibility == "" {
		in.Visibility = ApiVisPublic
	}
	limit := normalizeLimit(in.Limit, 30)

	indexPath := generators.GlobalOutputPath(cfg.MoodlePath, "MOODLE_API_INDEX.md")
	if _, err := os.Stat(indexPath); err != nil {
		return textResult(true, "❌ API index not found. Run `init_moodle_context` or `update_indexes` first."), struct{}{}, nil
	}

	lines := searchInFile(indexPath, in.Query)
	lines = filterFunctionLines(lines)
	fuzzy := false
	if len(lines) == 0 {
		lines = filterFunctionLines(fuzzySearchInFile(indexPath, in.Query))
		fuzzy = len(lines) > 0
	}

	var matches []string
	for _, l := range lines {
		hasDeprecated := strings.Contains(l, "@deprecated")
		switch in.Visibility {
		case ApiVisPublic:
			if !hasDeprecated {
				matches = append(matches, l)
			}
		case ApiVisDeprecated:
			if hasDeprecated {
				matches = append(matches, l)
			}
		default:
			matches = append(matches, l)
		}
	}
	if len(matches) > limit {
		matches = matches[:limit]
	}

	if in.Format == FormatJSON {
		return jsonResult(false, map[string]any{"query": in.Query, "visibility": in.Visibility, "fuzzy": fuzzy, "matches": matches}), struct{}{}, nil
	}
	if len(matches) == 0 {
		return textResult(false, fmt.Sprintf("No functions matched %q.", in.Query)), struct{}{}, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# API Search — %q (visibility: %s)\n\n", in.Query, in.Visibility)
	if fuzzy {
		b.WriteString("_(no exact matches — showing fuzzy matches)_\n\n")
	}
	for _, m := range matches {
		fmt.Fprintf(&b, "%s\n", m)
	}
	return textResult(false, b.String()), struct{}{}, nil
}

func filterFunctionLines(lines []string) []string {
	var out []string
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "- `") {
			out = append(out, l)
		}
	}
	return out
}

// --- 5c. get_plugin_info ---------------------------------------------------------

type GetPluginInfoInput struct {
	Plugin string `json:"plugin" jsonschema:"Component, relative path, or absolute path"`
	Format Format `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

func handleGetPluginInfo(ctx context.Context, req *mcp.CallToolRequest, in GetPluginInfoInput) (*mcp.CallToolResult, struct{}, error) {
	cfg, err := requireConfig()
	if err != nil {
		return textResult(true, "❌ Failed to resolve build82 configuration: "+err.Error()), struct{}{}, nil
	}
	if cfg == nil {
		return toolutil.NotInitialized(), struct{}{}, nil
	}

	pluginPath, ok := resolvePluginPathWithinMoodle(in.Plugin, cfg.MoodlePath)
	if !ok {
		// resolvePluginPathWithinMoodle returning false covers two distinct cases
		// (unresolvable identifier vs. resolved-but-outside-the-root) — both fall through to the
		// same fuzzy-match-search / not-found path below. A path that resolves outside the Moodle
		// root is treated the same as one that doesn't resolve at all, rather than producing a
		// containment-specific error, because this tool is a forgiving, best-effort lookup.
		pluginPath = filepath.Join(cfg.MoodlePath, in.Plugin)
	}

	if _, err := os.Stat(pluginPath); err != nil || !moodletype.IsWithinMoodle(pluginPath, cfg.MoodlePath) {
		indexPath := generators.GlobalOutputPath(cfg.MoodlePath, "MOODLE_PLUGIN_INDEX.md")
		possible := filterTableRows(searchInFile(indexPath, in.Plugin))
		if len(possible) > 0 {
			var b strings.Builder
			fmt.Fprintf(&b, "Plugin %q not found directly, but found possible matches:\n\n", in.Plugin)
			b.WriteString("| Component | Type | Name | Version | Path |\n|---|---|---|---|---|\n")
			for _, m := range possible {
				fmt.Fprintf(&b, "%s\n", m)
			}
			return textResult(false, b.String()), struct{}{}, nil
		}
		return textResult(true, fmt.Sprintf("❌ Plugin not found: %s", in.Plugin)), struct{}{}, nil
	}

	// Reported relative to the Moodle root: an absolute host path must never leak into tool output.
	relPath := relativeToMoodle(cfg.MoodlePath, pluginPath)

	if content, err := os.ReadFile(generators.PluginOutputPath(pluginPath, "PLUGIN_AI_CONTEXT.md")); err == nil {
		if in.Format == FormatJSON {
			return jsonResult(false, map[string]any{"path": relPath, "ai_context": string(content)}), struct{}{}, nil
		}
		return textResult(false, string(content)), struct{}{}, nil
	}

	info, err := extractors.DetectPlugin(pluginPath)
	if err != nil {
		return textResult(true, "❌ Failed to detect plugin: "+err.Error()), struct{}{}, nil
	}
	info.Path = relPath
	if in.Format == FormatJSON {
		return jsonResult(false, info), struct{}{}, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n| Field | Value |\n|---|---|\n| Type | %s |\n| Version | %s |\n| Requires | %s |\n"+
		"| Display name | %s |\n| Path | %s |\n\n_generate_plugin_context has not been run yet for this plugin._\n",
		info.Component, info.Type, info.Version, info.Requires, info.DisplayName, relPath)
	return textResult(false, b.String()), struct{}{}, nil
}

// --- 5d. list_dev_plugins ---------------------------------------------------------

type ListDevPluginsInput struct {
	Format Format `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

func handleListDevPlugins(ctx context.Context, req *mcp.CallToolRequest, in ListDevPluginsInput) (*mcp.CallToolResult, struct{}, error) {
	cfg, err := requireConfig()
	if err != nil {
		return textResult(true, "❌ Failed to resolve build82 configuration: "+err.Error()), struct{}{}, nil
	}
	if cfg == nil {
		return toolutil.NotInitialized(), struct{}{}, nil
	}

	dirs := generators.FindDevPlugins(cfg.MoodlePath)
	sort.Strings(dirs)

	if len(dirs) == 0 {
		return textResult(false, "No .indevelopment plugins found."), struct{}{}, nil
	}

	type row struct {
		Component, Path string
		HasAiContext    bool
	}
	rows := make([]row, len(dirs))
	for i, d := range dirs {
		component := "_(unknown)_"
		if info, err := extractors.DetectPlugin(d); err == nil {
			component = info.Component
		}
		_, statErr := os.Stat(generators.PluginOutputPath(d, "PLUGIN_AI_CONTEXT.md"))
		rel, _ := filepath.Rel(cfg.MoodlePath, d)
		rows[i] = row{Component: component, Path: filepath.ToSlash(rel), HasAiContext: statErr == nil}
	}

	if in.Format == FormatJSON {
		return jsonResult(false, rows), struct{}{}, nil
	}
	var b strings.Builder
	b.WriteString("# Dev Plugins\n\n| Component | Path | Has AI Context |\n|---|---|---|\n")
	for _, r := range rows {
		mark := ""
		if r.HasAiContext {
			mark = "✔"
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", r.Component, r.Path, mark)
	}
	return textResult(false, b.String()), struct{}{}, nil
}
