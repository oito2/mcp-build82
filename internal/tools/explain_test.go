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

package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestHandleExplainPlugin_UnknownSectionIsError confirms that an unrecognized `section` value
// (typically a typo, e.g. "overiew") is rejected up front with a structured IsError result,
// instead of falling through every `if in.Section == SectionXxx` check and returning a response
// truncated to just "Overview" + "Key Files Present".
func TestHandleExplainPlugin_UnknownSectionIsError(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)

	pluginDir := filepath.Join(moodlePath, "local", "demo")
	mustMkdirAll(t, pluginDir)
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"),
		"<?php\n$plugin->component = 'local_demo';\n$plugin->version = 2024010100;\n")

	res, _, err := handleExplainPlugin(context.Background(), nil, ExplainPluginInput{
		Plugin:  "local/demo",
		Section: ExplainSection("overiew"), // typo, not a valid section
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected IsError=true for an unrecognized section value")
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "overiew") {
		t.Errorf("expected the error message to name the bad value, got: %s", text)
	}
}

// TestHandleExplainPlugin_KnownSectionsAreAccepted confirms the section validation doesn't reject
// any of the legitimate section values (including the empty-string default, which
// handleExplainPlugin maps to SectionAll before validating).
func TestHandleExplainPlugin_KnownSectionsAreAccepted(t *testing.T) {
	moodlePath := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)

	pluginDir := filepath.Join(moodlePath, "local", "demo")
	mustMkdirAll(t, pluginDir)
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"),
		"<?php\n$plugin->component = 'local_demo';\n$plugin->version = 2024010100;\n")

	for _, section := range []ExplainSection{
		"", SectionAll, SectionOverview, SectionDatabase, SectionEvents,
		SectionClasses, SectionServices, SectionFlow,
	} {
		res, _, err := handleExplainPlugin(context.Background(), nil, ExplainPluginInput{
			Plugin: "local/demo", Section: section,
		})
		if err != nil {
			t.Fatalf("section %q: unexpected error: %v", section, err)
		}
		if res.IsError {
			text := res.Content[0].(*mcp.TextContent).Text
			t.Errorf("section %q: unexpected IsError result: %s", section, text)
		}
	}
}
