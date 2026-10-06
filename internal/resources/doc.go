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

// Package resources implements the global and per-plugin MCP resources, which serve the Markdown
// context files generated under each `.build82` directory.
package resources

// notInitializedBody is the message body returned by every resource when no configuration exists
// yet (config.Load returns nil). It is used by readMoodleFile, readPluginFile, and
// handlePluginsWithContext, the last of which pairs it with its own heading.
const notInitializedBody = "build82 has not been initialized.\n\n" +
	"Run the `init_moodle_context` tool to generate context files."

// notInitializedText is the generic-heading placeholder returned by readMoodleFile and
// readPluginFile when no configuration exists yet.
const notInitializedText = "# Resource not available\n\n" + notInitializedBody
