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

package toolutil

import "github.com/modelcontextprotocol/go-sdk/mcp"

// NotInitialized returns the error response used by every tool that requires a loaded
// configuration when none exists. It tells the caller to run `init_moodle_context` first.
//
// It is a constructor rather than a shared package-level result so that each call gets its own
// *mcp.CallToolResult and Content slice; a caller that mutates the result cannot affect other
// tools or concurrent calls.
func NotInitialized() *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{
			Text: "❌ build82 is not initialized. Run `init_moodle_context` first.",
		}},
		IsError: true,
	}
}
