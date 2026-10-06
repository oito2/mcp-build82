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

package resources

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/config"
	"github.com/oito2/mcp-build82/internal/extractors"
	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/genutil"
	"github.com/oito2/mcp-build82/internal/moodletype"
)

// pluginFileResource describes one per-plugin resource template: the URI suffix after
// moodle://plugin/{component}, the generated file it serves, and its label and description.
type pluginFileResource struct {
	Suffix, Filename, Label, Description string
}

// pluginFileResources lists every per-plugin file resource template.
var pluginFileResources = []pluginFileResource{
	{"", "PLUGIN_AI_CONTEXT.md", "AI Context", "Complete AI context — start here."},
	{"/context", "PLUGIN_CONTEXT.md", "Context", "Plugin metadata summary."},
	{"/structure", "PLUGIN_STRUCTURE.md", "Structure", "Directory tree and file inventory."},
	{"/database", "PLUGIN_DB_TABLES.md", "Database", "Full DB schema."},
	{"/events", "PLUGIN_EVENTS.md", "Events", "Event observers."},
	{"/dependencies", "PLUGIN_DEPENDENCIES.md", "Dependencies", "Tasks, services, capabilities, upgrade history."},
	{"/architecture", "PLUGIN_ARCHITECTURE.md", "Architecture", "Architecture overview and class structure."},
	{"/settings", "PLUGIN_SETTINGS.md", "Settings", "Admin settings declared in settings.php."},
	{"/functions", "PLUGIN_FUNCTION_INDEX.md", "Functions", "All PHP functions declared."},
	{"/callbacks", "PLUGIN_CALLBACK_INDEX.md", "Callbacks", "Moodle hook callbacks."},
	{"/endpoints", "PLUGIN_ENDPOINT_INDEX.md", "Endpoints", "Web services, AJAX, AMD modules."},
	{"/flow", "PLUGIN_RUNTIME_FLOW.md", "Runtime Flow", "Entry points and execution flow."},
}

// RegisterPluginResources registers on `server` the static moodle://plugins/with-context aggregate
// and one resource template per entry of pluginFileResources.
//
// The SDK's mcp.ResourceTemplate cannot enumerate its concrete URIs, so moodle://plugins/with-context
// is the only listing, and it covers just the AI-context file kind.
func RegisterPluginResources(server *mcp.Server) {
	server.AddResource(&mcp.Resource{
		URI:         "moodle://plugins/with-context",
		Name:        "Plugins With AI Context",
		Description: "Lists all Moodle plugins that have AI context files generated.",
		MIMEType:    "text/markdown",
	}, withRecoverResource(handlePluginsWithContext))

	for _, def := range pluginFileResources {
		def := def
		server.AddResourceTemplate(&mcp.ResourceTemplate{
			URITemplate: fmt.Sprintf("moodle://plugin/{component}%s", def.Suffix),
			Name:        fmt.Sprintf("Plugin %s", def.Label),
			Description: def.Description,
			MIMEType:    "text/markdown",
		}, withRecoverResource(makePluginFileHandler(def)))
	}
}

// handlePluginsWithContext serves moodle://plugins/with-context: a Markdown table of the plugins
// under the Moodle root that have a generated PLUGIN_AI_CONTEXT.md, followed by usage notes. It
// returns a placeholder when no configuration exists, and an error only when config.Load fails.
func handlePluginsWithContext(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI: uri, MIMEType: "text/markdown",
			Text: "# Plugins With AI Context\n\n" + notInitializedBody,
		}}}, nil
	}

	rows := pluginsWithContextRows(cfg.MoodlePath)

	var b strings.Builder
	b.WriteString("# Plugins With AI Context\n\n| Component | Type | Path |\n|---|---|---|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", r.component, r.typ, r.path)
	}
	b.WriteString("\n## Usage\n\n```\nmoodle://plugin/{component}           - AI context (default)\n" +
		"moodle://plugin/{component}/context   - Metadata summary\n" +
		"moodle://plugin/{component}/database  - DB schema\n" +
		"... (see the plugin resource templates for the full list)\n```\n")

	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "text/markdown", Text: b.String()}}}, nil
}

// pluginContextRow is one row of the plugins-with-context listing: component, plugin type, and
// the plugin directory relative to the Moodle root (slash-separated).
type pluginContextRow struct{ component, typ, path string }

// pluginsWithContextCacheEntry is a cached listing together with the time it stops being valid.
type pluginsWithContextCacheEntry struct {
	rows      []pluginContextRow
	expiresAt time.Time
}

// pluginsWithContextTTL is how long a computed plugins-with-context listing is reused, bounding
// the cost of repeated reads at the price of a listing that may lag newly generated context by up
// to this duration.
const pluginsWithContextTTL = 5 * time.Second

// pluginsWithContextCache holds the listing for the current Moodle root, guarded by pluginsWithContextCacheMu.
var (
	pluginsWithContextCacheMu sync.Mutex
	pluginsWithContextCache   = map[string]pluginsWithContextCacheEntry{}
)

// pluginsWithContextRows returns, sorted by component, the plugins under `moodlePath` that have a
// generated PLUGIN_AI_CONTEXT.md. Plugins whose metadata cannot be detected are skipped. The
// result, which involves a full directory walk and a version.php parse per match, is cached for
// pluginsWithContextTTL, keeping only the entry for the most recent `moodlePath`.
func pluginsWithContextRows(moodlePath string) []pluginContextRow {
	pluginsWithContextCacheMu.Lock()
	if entry, ok := pluginsWithContextCache[moodlePath]; ok && time.Now().Before(entry.expiresAt) {
		pluginsWithContextCacheMu.Unlock()
		return entry.rows
	}
	pluginsWithContextCacheMu.Unlock()

	matches := globContextFiles(moodlePath, "PLUGIN_AI_CONTEXT.md")
	var rows []pluginContextRow
	for _, m := range matches {
		pluginDir := filepath.Dir(filepath.Dir(m)) // grandparent: out of .build82/, to the plugin root
		info, err := extractors.DetectPlugin(pluginDir)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(moodlePath, pluginDir)
		rows = append(rows, pluginContextRow{info.Component, info.Type, filepath.ToSlash(rel)})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].component < rows[j].component })

	pluginsWithContextCacheMu.Lock()
	clear(pluginsWithContextCache)
	pluginsWithContextCache[moodlePath] = pluginsWithContextCacheEntry{rows: rows, expiresAt: time.Now().Add(pluginsWithContextTTL)}
	pluginsWithContextCacheMu.Unlock()

	return rows
}

// globContextFiles returns the paths of every `.build82/<filename>` file found under `moodlePath`,
// skipping vendor and node_modules directories below the root and ignoring unreadable entries.
func globContextFiles(moodlePath, filename string) []string {
	var matches []string
	_ = filepath.WalkDir(moodlePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != moodlePath && (name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == filename && filepath.Base(filepath.Dir(path)) == generators.ContextDir {
			matches = append(matches, path)
		}
		return nil
	})
	return matches
}

// makePluginFileHandler returns a resource-template handler for `def`: it extracts the component
// from the requested URI by stripping the moodle://plugin/ prefix and def.Suffix, then serves the
// matching generated file via readPluginFile, echoing the requested URI in the result.
func makePluginFileHandler(def pluginFileResource) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		uri := req.Params.URI
		component := strings.TrimSuffix(strings.TrimPrefix(uri, "moodle://plugin/"), def.Suffix)

		text, err := readPluginFile(component, def.Filename)
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "text/markdown", Text: text}}}, nil
	}
}

// readPluginFile returns the generated file `filename` of the plugin identified by `component`
// (a component name or a path). When no configuration exists, the plugin cannot be resolved inside
// the Moodle root, or the file has not been generated, it returns placeholder text and a nil error.
// An error is returned only when config.Load fails.
func readPluginFile(component, filename string) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	if cfg == nil {
		return notInitializedText, nil
	}
	// The component comes from the client-controlled resource URI, so a resolved path outside the
	// Moodle root is treated as not found and its files are never read.
	pluginPath, ok := moodletype.ResolvePluginPath(component, cfg.MoodlePath)
	if ok && !moodletype.IsWithinMoodle(pluginPath, cfg.MoodlePath) {
		ok = false
	}
	if !ok {
		return fmt.Sprintf("# Plugin not found\n\nCould not resolve %q to a plugin directory under the configured Moodle root.", component), nil
	}
	path := generators.PluginOutputPath(pluginPath, filename)
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		return fmt.Sprintf("# %s — Not found\n\nExpected at: %s\n\nThis file has not been generated yet.\n"+
			"Run `generate_plugin_context` on this plugin to generate it.",
			filename, genutil.RelativeOrOriginal(cfg.MoodlePath, path)), nil
	}
	return string(content), nil
}
