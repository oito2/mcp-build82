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

package server

import (
	_ "embed"
	"encoding/base64"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/prompts"
	"github.com/oito2/mcp-build82/internal/resources"
	"github.com/oito2/mcp-build82/internal/tools"
	"github.com/oito2/mcp-build82/internal/version"
)

// iconPNG holds the embedded 64x64 PNG server icon reported in serverInfo.
//
//go:embed icon.png
var iconPNG []byte

// serverIcons returns the server icon as a single base64 PNG data URI entry, so clients can
// display it without a network fetch (the stdio transport has no URL to serve it from).
func serverIcons() []mcp.Icon {
	return []mcp.Icon{{
		Source:   "data:image/png;base64," + base64.StdEncoding.EncodeToString(iconPNG),
		MIMEType: "image/png",
		Sizes:    []string{"64x64"},
	}}
}

// NewServer builds a new build82 MCP server with every tool, global and plugin resource, and
// prompt registered. Each call returns an independent server instance, so HTTP sessions can each
// use their own.
func NewServer() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "build82",
		Version: version.Current,
		Icons:   serverIcons(),
	}, nil)

	tools.RegisterInitTool(s)
	tools.RegisterGenerateContextTool(s)
	tools.RegisterBatchTool(s)
	tools.RegisterUpdateTool(s)  // registers both update_indexes and watch_plugins
	tools.RegisterSearchTools(s) // registers search_plugins, search_api, get_plugin_info, list_dev_plugins
	tools.RegisterDoctorTool(s)
	tools.RegisterExplainTool(s)
	tools.RegisterReleaseTool(s)
	tools.RegisterCreatePluginSkeletonTool(s)

	resources.RegisterGlobalResources(s)
	resources.RegisterPluginResources(s)

	prompts.RegisterScaffoldPrompt(s)
	prompts.RegisterReviewPrompt(s)
	prompts.RegisterDebugPrompt(s)

	return s
}
