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
	"context"
	_ "embed"
	"encoding/base64"
	"fmt"

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
// prompt registered. It supports resource subscriptions: a client may subscribe to any resource
// the server serves (see resources.KnownURI) and is then sent notifications/resources/updated
// when a tool or the watcher regenerates its file. It does not advertise the logging capability
// (deprecated by MCP, and never used: the server sends no log notifications). Each call returns an
// independent server instance.
func NewServer() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "build82",
		Version: version.Current,
		Icons:   serverIcons(),
	}, &mcp.ServerOptions{
		// A non-nil value replaces the SDK's default {"logging":{}}; tools, prompts and resources
		// are still inferred from what is registered below.
		Capabilities:       &mcp.ServerCapabilities{},
		SubscribeHandler:   subscribe,
		UnsubscribeHandler: unsubscribe,
	})

	tools.RegisterInitTool(s)
	tools.RegisterGenerateContextTool(s)
	tools.RegisterBatchTool(s)
	tools.RegisterUpdateTool(s)  // registers both update_indexes and watch_plugins
	tools.RegisterSearchTools(s) // registers search_plugins, search_api, get_plugin_info, list_dev_plugins
	tools.RegisterDoctorTool(s)
	tools.RegisterExplainTool(s)
	tools.RegisterReleaseTool(s)
	tools.RegisterCreatePluginSkeletonTool(s)
	tools.RegisterBumpVersionTool(s)
	tools.RegisterCodingStyleTool(s)

	resources.RegisterGlobalResources(s)
	resources.RegisterPluginResources(s)

	prompts.RegisterScaffoldPrompt(s)
	prompts.RegisterReviewPrompt(s)
	prompts.RegisterDebugPrompt(s)
	prompts.RegisterUpgradePrompt(s)

	return s
}

// subscribe accepts a resource subscription when the URI names a resource this server serves; the
// SDK then records it for the session. Any other URI is refused with an error.
func subscribe(_ context.Context, req *mcp.SubscribeRequest) error {
	if !resources.KnownURI(req.Params.URI) {
		return fmt.Errorf("unknown resource %q: build82 serves moodle://... resources only", req.Params.URI)
	}
	return nil
}

// unsubscribe accepts every unsubscription; the SDK removes the session from the URI's
// subscribers.
func unsubscribe(context.Context, *mcp.UnsubscribeRequest) error {
	return nil
}
