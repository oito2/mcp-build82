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

// Package resources implements the global and per-plugin MCP resource definitions.
package resources

// notInitializedBody is the shared message body for every resource's "config.Load() is nil"
// placeholder, used by readMoodleFile, readPluginFile, and handlePluginsWithContext.
// handlePluginsWithContext keeps its own specific heading (it names the resource itself, "Plugins
// With AI Context") but shares this same body text.
const notInitializedBody = "build82 has not been initialized.\n\n" +
	"Run the `init_moodle_context` tool to generate context files."

// notInitializedText is the full placeholder (generic heading + body) for the two resources with
// no more specific heading of their own.
const notInitializedText = "# Resource not available\n\n" + notInitializedBody
