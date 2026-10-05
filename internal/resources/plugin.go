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

type pluginFileResource struct {
	Suffix, Filename, Label, Description string
}

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

// RegisterPluginResources registers the static "plugins with context" aggregate and the 12
// dynamic per-plugin-file resource templates.
//
// SDK gap: the Go SDK's mcp.ResourceTemplate has no List field/callback, so template resources
// cannot advertise a per-file-kind enumeration of their concrete URIs. This is an SDK-imposed
// limitation, not a design choice: moodle://plugins/with-context (below) is the only enumeration
// surface, covering just the AI-context file kind, not all 12.
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

type pluginContextRow struct{ component, typ, path string }

type pluginsWithContextCacheEntry struct {
	rows      []pluginContextRow
	expiresAt time.Time
}

// pluginsWithContextTTL bounds how stale moodle://plugins/with-context's listing can be between a
// plugin's context actually being (re)generated and this resource reflecting it — chosen as a
// pragmatic ceiling on repeated-read cost for large installations, not because the underlying data
// changes on any particular schedule.
const pluginsWithContextTTL = 5 * time.Second

var (
	pluginsWithContextCacheMu sync.Mutex
	pluginsWithContextCache   = map[string]pluginsWithContextCacheEntry{}
)

// pluginsWithContextRows computes (or returns the cached) sorted list of plugins that have
// PLUGIN_AI_CONTEXT.md generated, memoizing the full filepath.WalkDir + per-match
// extractors.DetectPlugin (which reparses version.php) for pluginsWithContextTTL, instead of
// redoing both on every read of the moodle://plugins/with-context resource.
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
	pluginsWithContextCache[moodlePath] = pluginsWithContextCacheEntry{rows: rows, expiresAt: time.Now().Add(pluginsWithContextTTL)}
	pluginsWithContextCacheMu.Unlock()

	return rows
}

// globContextFiles globs **/.build82/{filename} under moodlePath, ignoring vendor/node_modules.
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

// readPluginFile's error return is non-nil only for a genuine failure resolving config (see
// config.Load); every other "can't produce real content" case (no config yet, plugin not
// found, file not generated yet) still comes back as (placeholder text, nil).
func readPluginFile(component, filename string) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	if cfg == nil {
		return notInitializedText, nil
	}
	// IsWithinMoodle is required here, not just ResolvePluginPath — component comes straight from
	// the client-controlled resource URI (moodle://plugin/{component}...), and nothing else in this
	// handler checks containment. A component that resolves outside the Moodle root is rejected so
	// its files are never read into the resource response.
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
