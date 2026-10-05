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

package toolutil

import "github.com/modelcontextprotocol/go-sdk/mcp"

// NotInitialized returns the canned response returned by every tool that requires config to be
// loaded but found none. A constructor function rather than a shared *mcp.CallToolResult var:
// a package-level var would expose one *mcp.CallToolResult pointer, shared
// by every tool and every concurrent call, with a mutable Content slice — a future call site doing
// something as innocuous-looking as `res := toolutil.NotInitialized; res.Content =
// append(res.Content, x)` would corrupt this canonical response for the entire process, for every
// client, until restart. Nothing mutates it today, but a constructor removes the risk entirely
// instead of relying on every future caller remembering not to.
func NotInitialized() *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{
			Text: "❌ build82 is not initialized. Run `init_moodle_context` first.",
		}},
		IsError: true,
	}
}
