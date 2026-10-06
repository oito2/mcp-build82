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

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestNotInitialized verifies the response is a single text error block carrying the build82
// message and none of the legacy branding.
func TestNotInitialized(t *testing.T) {
	result := NotInitialized()
	if !result.IsError {
		t.Error("expected IsError=true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected exactly one content block, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(tc.Text, "build82") {
		t.Errorf("expected the build82 branded message, got %q", tc.Text)
	}
	if strings.Contains(tc.Text, "moodle-mcp") {
		t.Errorf("expected the legacy moodle-mcp string to be gone, got %q", tc.Text)
	}
}

// TestNotInitialized_ReturnsFreshInstanceEachCall verifies that each call returns its own
// independent *mcp.CallToolResult, so mutating one result (e.g. appending to Content) never affects
// another.
func TestNotInitialized_ReturnsFreshInstanceEachCall(t *testing.T) {
	a := NotInitialized()
	b := NotInitialized()
	if a == b {
		t.Fatal("expected two distinct *mcp.CallToolResult instances, got the same pointer")
	}
	a.Content = append(a.Content, &mcp.TextContent{Text: "mutated"})
	if len(b.Content) != 1 {
		t.Errorf("mutating one call's result must not affect another call's result, got len(b.Content)=%d", len(b.Content))
	}
}
