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

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/config"
	"github.com/oito2/mcp-build82/internal/fsutil"
	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/genutil"
)

// globalResourceDef describes one global resource: its URI, the generated file it serves, and its
// display name and description.
type globalResourceDef struct {
	URI, Filename, Name, Description string
}

// globalResources is the table mapping each global resource URI to the generated file it serves.
var globalResources = []globalResourceDef{
	{"moodle://context", "AI_CONTEXT.md", "Moodle AI Context", "High-level overview: version, directory structure, key APIs, coding guidelines"},
	{"moodle://index", "MOODLE_AI_INDEX.md", "Moodle AI Index", "Master index linking all generated files and plugin AI contexts"},
	{"moodle://workspace", "MOODLE_AI_WORKSPACE.md", "Moodle AI Workspace", "Workspace guide: version, all plugins, dev plugins, plugins with AI context"},
	{"moodle://api-index", "MOODLE_API_INDEX.md", "Moodle API Index", "Public functions from lib/, grouped by source file"},
	{"moodle://events-index", "MOODLE_EVENTS_INDEX.md", "Moodle Events Index", "Event observers across all plugins"},
	{"moodle://tasks-index", "MOODLE_TASKS_INDEX.md", "Moodle Tasks Index", "Scheduled tasks across all plugins"},
	{"moodle://services-index", "MOODLE_SERVICES_INDEX.md", "Moodle Services Index", "Web service functions across all plugins"},
	{"moodle://db-tables", "MOODLE_DB_TABLES_INDEX.md", "Moodle DB Tables Index", "Database tables across all plugins"},
	{"moodle://classes-index", "MOODLE_CLASSES_INDEX.md", "Moodle Classes Index", "PHP classes/interfaces/traits/enums, FQNs"},
	{"moodle://capabilities-index", "MOODLE_CAPABILITIES_INDEX.md", "Moodle Capabilities Index", "Capabilities across all plugins"},
	{"moodle://plugin-index", "MOODLE_PLUGIN_INDEX.md", "Moodle Plugin Index", "Component/type/name/version/path map"},
	{"moodle://dev-rules", "MOODLE_DEV_RULES.md", "Moodle Dev Rules", "Coding standards, security rules, DB conventions"},
	{"moodle://plugin-guide", "MOODLE_PLUGIN_GUIDE.md", "Moodle Plugin Guide", "Component naming, required files, version.php template, hooks"},
}

// RegisterGlobalResources registers every resource listed in globalResources on `server`, each
// served as text/markdown.
func RegisterGlobalResources(server *mcp.Server) {
	for _, def := range globalResources {
		def := def
		server.AddResource(&mcp.Resource{
			URI:         def.URI,
			Name:        def.Name,
			Description: def.Description,
			MIMEType:    "text/markdown",
		}, withRecoverResource(handleGlobalResource(def.Filename)))
	}
}

// handleGlobalResource returns a resource handler that serves the global generated file named
// `filename`, echoing the requested URI in the result. It fails only when readMoodleFile fails.
func handleGlobalResource(filename string) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		text, err := readMoodleFile(filename)
		if err != nil {
			return nil, err
		}
		uri := req.Params.URI
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "text/markdown", Text: text}},
		}, nil
	}
}

// readMoodleFile returns the content of the global generated file `filename` under the configured
// Moodle root. When no configuration exists, or the file has not been generated yet, it returns
// placeholder text (with the path relative to the Moodle root) and a nil error. An error is
// returned only when config.Load itself fails.
func readMoodleFile(filename string) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	if cfg == nil {
		return notInitializedText, nil
	}
	path := generators.GlobalOutputPath(cfg.MoodlePath, filename)
	content, readErr := fsutil.ReadRegular(path, 0)
	if readErr != nil {
		return fmt.Sprintf("# %s — Not found\n\nExpected at: %s\n\nThis file has not been generated yet.\n"+
			"Run `init_moodle_context` or `update_indexes` to generate it.",
			filename, genutil.RelativeOrOriginal(cfg.MoodlePath, path)), nil
	}
	return string(content), nil
}
