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
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/fsutil"
	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/moodletype"
	"github.com/oito2/mcp-build82/internal/toolutil"
)

// maxQueryLen is the maximum accepted length of the search_plugins and search_api queries. It
// bounds the CPU cost of fuzzySearchInFile, which compares the query by edit distance against
// every word of every line of an index file.
const maxQueryLen = 200

// --- shared index cache --------------------------------------------------------

// indexCacheEntry is a cached index file: its modification time (UnixNano) and its lines.
type indexCacheEntry struct {
	mtime int64
	lines []string
}

// indexCache holds the lines of recently searched index files keyed by path, guarded by
// indexCacheMu.
var (
	indexCache   = map[string]indexCacheEntry{}
	indexCacheMu sync.Mutex
)

// indexCacheMax is the number of files indexCache holds before it is emptied.
const indexCacheMax = 50

// cachedLines returns the content of `filePath` split into lines, served from a cache that is
// refreshed when the file's modification time changes. It returns nil when the file cannot be
// stat'ed or read. The cache is mutex-protected because handlers may run concurrently. Callers
// must not modify the returned slice.
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
	content, readErr := fsutil.ReadRegular(filePath, 0)
	if readErr != nil {
		return nil
	}
	entry = indexCacheEntry{mtime: mtime, lines: strings.Split(string(content), "\n")}
	indexCache[filePath] = entry
	return entry.lines
}

// searchInFile returns the lines of the file `filePath` that contain `query`, compared
// case-insensitively. It returns nil when the file cannot be read.
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

// fuzzySearchInFile is the typo-tolerant fallback for searchInFile, used only when the exact match
// finds nothing. It returns the lines of `filePath` that contain a word whose edit distance to
// `query` (case-insensitive) is within the limit given by maxEditDistance. It returns nil when the
// file cannot be read.
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

// maxEditDistance returns the edit distance tolerated for a query of `queryLen` bytes: none for
// very short queries, growing with length.
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

// wordSplit splits `s` into words made of the characters a-z, 0-9 and underscore.
func wordSplit(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_'
	})
}

// levenshtein returns the edit distance (insertions, deletions, substitutions) between the
// strings `a` and `b`, measured in runes.
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

// min3 returns the smallest of `a`, `b` and `c`.
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

// SearchPluginsOutput is the structured output of search_plugins: the query, whether the matches
// come from the fuzzy fallback, and the matching rows of the plugin index (Markdown table rows).
type SearchPluginsOutput struct {
	Query   string   `json:"query"`
	Fuzzy   bool     `json:"fuzzy"`
	Matches []string `json:"matches"`
}

// SearchPluginsInput is the input of the search_plugins tool.
type SearchPluginsInput struct {
	Query  string `json:"query" jsonschema:"Search term (component name, plugin type, etc.)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Max results, 1-100 (default 20)"`
	Format Format `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

// RegisterSearchTools registers the search_plugins, search_api, get_plugin_info and
// list_dev_plugins tools on `server`.
func RegisterSearchTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_plugins",
		Annotations: toolAnnotations("Search Plugins", true, false, true, false),
		Description: "Searches the plugin index (component, type, name, version, path) for a query string.",
	}, withRecover(handleSearchPlugins))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_api",
		Annotations: toolAnnotations("Search Moodle API", true, false, true, false),
		Description: "Searches the public Moodle API index (lib/ functions) for a query string, filterable by visibility.",
	}, withRecover(handleSearchApi))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_plugin_info",
		Annotations: toolAnnotations("Get Plugin Info", true, false, true, false),
		Description: "Returns the generated AI context for a plugin (or live-detected metadata if not yet generated).",
	}, withRecover(handleGetPluginInfo))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_dev_plugins",
		Annotations: toolAnnotations("List Dev Plugins", true, false, true, false),
		Description: "Lists every plugin currently marked .indevelopment.",
	}, withRecover(handleListDevPlugins))
}

// normalizeLimit returns `def` when `limit` is not positive, and `limit` capped at 100 otherwise.
func normalizeLimit(limit, def int) int {
	if limit <= 0 {
		return def
	}
	if limit > 100 {
		return 100
	}
	return limit
}

// handleSearchPlugins searches the generated plugin index for `in.Query`, falling back to fuzzy
// matching when there is no exact match, and returns at most `in.Limit` rows (default 20, max
// 100). The structured output is a SearchPluginsOutput in either format; no match is a success
// with an empty list. A missing or invalid configuration, a query longer than maxQueryLen, or a
// missing index is returned as an error, so that result has no structured output.
func handleSearchPlugins(ctx context.Context, req *mcp.CallToolRequest, in SearchPluginsInput) (*mcp.CallToolResult, SearchPluginsOutput, error) {
	cfg, err := requireConfig()
	if err != nil {
		return nil, SearchPluginsOutput{}, configError(err)
	}
	if cfg == nil {
		return nil, SearchPluginsOutput{}, resultError(toolutil.NotInitialized())
	}
	if len(in.Query) > maxQueryLen {
		return nil, SearchPluginsOutput{}, toolError(fmt.Sprintf("❌ query too long (max %d characters).", maxQueryLen))
	}
	limit := normalizeLimit(in.Limit, 20)

	indexPath := generators.GlobalOutputPath(cfg.MoodlePath, "MOODLE_PLUGIN_INDEX.md")
	if _, err := os.Stat(indexPath); err != nil {
		return nil, SearchPluginsOutput{}, toolError("❌ Plugin index not found. Run `init_moodle_context` or `update_indexes` first.")
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

	if matches == nil {
		matches = []string{}
	}
	out := SearchPluginsOutput{Query: in.Query, Fuzzy: fuzzy, Matches: matches}
	if len(matches) == 0 {
		return structuredResult(in.Format, false, fmt.Sprintf("No plugins matched %q.", in.Query), out)
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
	return structuredResult(in.Format, false, b.String(), out)
}

// filterTableRows returns the Markdown table rows among `lines`: those starting with "|",
// excluding the header row (whose first cell is "Component") and the separator row.
func filterTableRows(lines []string) []string {
	var out []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if !strings.HasPrefix(t, "|") || isTableHeaderOrSeparator(t) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// isTableHeaderOrSeparator reports whether the trimmed table row `row` is the plugin index header
// (first cell "Component") or a Markdown separator row (cells made only of '-', ':' and spaces).
func isTableHeaderOrSeparator(row string) bool {
	cells := strings.Split(strings.Trim(row, "|"), "|")
	if strings.TrimSpace(cells[0]) == "Component" {
		return true
	}
	for _, c := range cells {
		if strings.Trim(strings.TrimSpace(c), "-:") != "" {
			return false
		}
	}
	return true
}

// --- 5b. search_api ---------------------------------------------------------------

// ApiVisibilityFilter selects which entries of the API index search_api returns.
type ApiVisibilityFilter string

// Supported values of ApiVisibilityFilter.
const (
	ApiVisPublic     ApiVisibilityFilter = "public"
	ApiVisDeprecated ApiVisibilityFilter = "deprecated"
	ApiVisAll        ApiVisibilityFilter = "all"
)

// SearchApiOutput is the structured output of search_api: the query, the visibility filter
// applied, whether the matches come from the fuzzy fallback, and the matching function entries of
// the API index (Markdown list items).
type SearchApiOutput struct {
	Query      string              `json:"query"`
	Visibility ApiVisibilityFilter `json:"visibility"`
	Fuzzy      bool                `json:"fuzzy"`
	Matches    []string            `json:"matches"`
}

// SearchApiInput is the input of the search_api tool.
type SearchApiInput struct {
	Query      string              `json:"query" jsonschema:"Search term (function name, keyword in summary, etc.)"`
	Visibility ApiVisibilityFilter `json:"visibility,omitempty" jsonschema:"'public' (default), 'deprecated', or 'all'"`
	Limit      int                 `json:"limit,omitempty" jsonschema:"Max results, 1-100 (default 30)"`
	Format     Format              `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

// handleSearchApi searches the generated API index for `in.Query`, falling back to fuzzy matching
// when there is no exact match. `in.Visibility` (default "public") keeps non-deprecated,
// deprecated or all functions, and at most `in.Limit` entries (default 30, max 100) are returned.
// The structured output is a SearchApiOutput in either format; no match is a success with an empty
// list. A missing or invalid configuration, a query longer than maxQueryLen, or a missing index is
// returned as an error, so that result has no structured output.
func handleSearchApi(ctx context.Context, req *mcp.CallToolRequest, in SearchApiInput) (*mcp.CallToolResult, SearchApiOutput, error) {
	cfg, err := requireConfig()
	if err != nil {
		return nil, SearchApiOutput{}, configError(err)
	}
	if cfg == nil {
		return nil, SearchApiOutput{}, resultError(toolutil.NotInitialized())
	}
	if len(in.Query) > maxQueryLen {
		return nil, SearchApiOutput{}, toolError(fmt.Sprintf("❌ query too long (max %d characters).", maxQueryLen))
	}
	if in.Visibility == "" {
		in.Visibility = ApiVisPublic
	}
	limit := normalizeLimit(in.Limit, 30)

	indexPath := generators.GlobalOutputPath(cfg.MoodlePath, "MOODLE_API_INDEX.md")
	if _, err := os.Stat(indexPath); err != nil {
		return nil, SearchApiOutput{}, toolError("❌ API index not found. Run `init_moodle_context` or `update_indexes` first.")
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

	if matches == nil {
		matches = []string{}
	}
	out := SearchApiOutput{Query: in.Query, Visibility: in.Visibility, Fuzzy: fuzzy, Matches: matches}
	if len(matches) == 0 {
		return structuredResult(in.Format, false, fmt.Sprintf("No functions matched %q.", in.Query), out)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# API Search — %q (visibility: %s)\n\n", in.Query, in.Visibility)
	if fuzzy {
		b.WriteString("_(no exact matches — showing fuzzy matches)_\n\n")
	}
	for _, m := range matches {
		fmt.Fprintf(&b, "%s\n", m)
	}
	return structuredResult(in.Format, false, b.String(), out)
}

// filterFunctionLines returns the function entries among `lines`: those that are Markdown list
// items starting with a code span.
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

// GetPluginInfoInput is the input of the get_plugin_info tool.
type GetPluginInfoInput struct {
	Plugin string `json:"plugin" jsonschema:"Component, relative path, or absolute path"`
	Format Format `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

// GetPluginInfoOutput is the structured output of get_plugin_info: the plugin's path relative to
// the Moodle root, its detected metadata, and its generated PLUGIN_AI_CONTEXT.md when it exists.
type GetPluginInfoOutput struct {
	Path         string `json:"path"`
	Component    string `json:"component,omitempty"`
	Type         string `json:"type,omitempty"`
	Name         string `json:"name,omitempty"`
	Version      string `json:"version,omitempty"`
	Requires     string `json:"requires,omitempty"`
	DisplayName  string `json:"display_name,omitempty"`
	Maturity     string `json:"maturity,omitempty"`
	HasAIContext bool   `json:"has_ai_context"`
	AIContext    string `json:"ai_context,omitempty"`
}

// handleGetPluginInfo returns the generated PLUGIN_AI_CONTEXT.md of the plugin named by
// `in.Plugin` as text, or its live-detected metadata when that file does not exist. The structured
// output is a GetPluginInfoOutput in either format, holding the metadata and, when generated, the
// AI context. A plugin that cannot be found is returned as an error — listing the index entries
// matching the identifier when there are any — so that result has no structured output.
func handleGetPluginInfo(ctx context.Context, req *mcp.CallToolRequest, in GetPluginInfoInput) (*mcp.CallToolResult, GetPluginInfoOutput, error) {
	cfg, err := requireConfig()
	if err != nil {
		return nil, GetPluginInfoOutput{}, configError(err)
	}
	if cfg == nil {
		return nil, GetPluginInfoOutput{}, resultError(toolutil.NotInitialized())
	}

	pluginPath, ok := resolvePluginPathWithinMoodle(in.Plugin, cfg.MoodlePath)
	if !ok {
		// An unresolvable identifier and one resolving outside the Moodle root are treated the
		// same: both fall through to the index lookup and not-found handling below, because this
		// tool is a forgiving, best-effort lookup.
		pluginPath = filepath.Join(cfg.MoodlePath, in.Plugin)
	}

	if _, err := os.Stat(pluginPath); err != nil || !moodletype.IsWithinMoodle(pluginPath, cfg.MoodlePath) {
		indexPath := generators.GlobalOutputPath(cfg.MoodlePath, "MOODLE_PLUGIN_INDEX.md")
		possible := filterTableRows(searchInFile(indexPath, in.Plugin))
		if len(possible) > 0 {
			var b strings.Builder
			fmt.Fprintf(&b, "❌ Plugin %q not found directly, but found possible matches:\n\n", in.Plugin)
			b.WriteString("| Component | Type | Name | Version | Path |\n|---|---|---|---|---|\n")
			for _, m := range possible {
				fmt.Fprintf(&b, "%s\n", m)
			}
			return nil, GetPluginInfoOutput{}, toolError(b.String())
		}
		return nil, GetPluginInfoOutput{}, toolError(fmt.Sprintf("❌ Plugin not found: %s", in.Plugin))
	}

	// Reported relative to the Moodle root so no absolute host path appears in the output.
	relPath := relativeToMoodle(cfg.MoodlePath, pluginPath)
	info, detectErr := extractors.DetectPlugin(pluginPath)
	out := GetPluginInfoOutput{Path: relPath}
	if detectErr == nil {
		out.Component, out.Type, out.Name, out.Version = info.Component, info.Type, info.Name, info.Version
		out.Requires, out.DisplayName, out.Maturity = info.Requires, info.DisplayName, info.Maturity
	}

	if content, err := fsutil.ReadRegular(generators.PluginOutputPath(pluginPath, "PLUGIN_AI_CONTEXT.md"), 0); err == nil {
		out.HasAIContext, out.AIContext = true, string(content)
		return structuredResult(in.Format, false, string(content), out)
	}

	if detectErr != nil {
		return nil, GetPluginInfoOutput{}, toolError("❌ Failed to detect plugin: " + detectErr.Error())
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n| Field | Value |\n|---|---|\n| Type | %s |\n| Version | %s |\n| Requires | %s |\n"+
		"| Display name | %s |\n| Path | %s |\n\n_generate_plugin_context has not been run yet for this plugin._\n",
		info.Component, info.Type, info.Version, info.Requires, info.DisplayName, relPath)
	return structuredResult(in.Format, false, b.String(), out)
}

// --- 5d. list_dev_plugins ---------------------------------------------------------

// ListDevPluginsInput is the input of the list_dev_plugins tool.
type ListDevPluginsInput struct {
	Format Format `json:"format,omitempty" jsonschema:"'text' (default) for Markdown, 'json' for a structured response"`
}

// ListDevPluginsOutput is the structured output of list_dev_plugins: one entry per plugin marked
// .indevelopment, sorted by path.
type ListDevPluginsOutput struct {
	Plugins []DevPlugin `json:"plugins"`
}

// DevPlugin is one .indevelopment plugin of a ListDevPluginsOutput: its component ("_(unknown)_"
// when it cannot be detected), its path relative to the Moodle root, and whether its
// PLUGIN_AI_CONTEXT.md has been generated.
type DevPlugin struct {
	Component    string `json:"component"`
	Path         string `json:"path"`
	HasAIContext bool   `json:"has_ai_context"`
}

// handleListDevPlugins lists the plugins marked .indevelopment with their path relative to the
// Moodle root and whether a generated PLUGIN_AI_CONTEXT.md exists. The structured output is a
// ListDevPluginsOutput in either format; no dev plugin is a success with an empty list. A missing
// or invalid configuration is returned as an error, so that result has no structured output.
func handleListDevPlugins(ctx context.Context, req *mcp.CallToolRequest, in ListDevPluginsInput) (*mcp.CallToolResult, ListDevPluginsOutput, error) {
	cfg, err := requireConfig()
	if err != nil {
		return nil, ListDevPluginsOutput{}, configError(err)
	}
	if cfg == nil {
		return nil, ListDevPluginsOutput{}, resultError(toolutil.NotInitialized())
	}

	dirs := generators.FindDevPlugins(cfg.MoodlePath)
	sort.Strings(dirs)

	out := ListDevPluginsOutput{Plugins: make([]DevPlugin, 0, len(dirs))}
	if len(dirs) == 0 {
		return structuredResult(in.Format, false, "No .indevelopment plugins found.", out)
	}

	for _, d := range dirs {
		component := "_(unknown)_"
		if info, err := extractors.DetectPlugin(d); err == nil {
			component = info.Component
		}
		_, statErr := os.Stat(generators.PluginOutputPath(d, "PLUGIN_AI_CONTEXT.md"))
		rel, _ := filepath.Rel(cfg.MoodlePath, d)
		out.Plugins = append(out.Plugins, DevPlugin{Component: component, Path: filepath.ToSlash(rel), HasAIContext: statErr == nil})
	}

	var b strings.Builder
	b.WriteString("# Dev Plugins\n\n| Component | Path | Has AI Context |\n|---|---|---|\n")
	for _, r := range out.Plugins {
		mark := ""
		if r.HasAIContext {
			mark = "✔"
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", r.Component, r.Path, mark)
	}
	return structuredResult(in.Format, false, b.String(), out)
}
