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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// copyFixtureMoodleTree copies the shared testdata/moodle fixture into a fresh temp directory and
// returns its path, because the tools under test write files (configuration, generated context).
func copyFixtureMoodleTree(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "moodle")
	dst := t.TempDir()

	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		defer in.Close()
		out, createErr := os.Create(target)
		if createErr != nil {
			return createErr
		}
		defer out.Close()
		_, copyErr := io.Copy(out, in)
		return copyErr
	})
	if err != nil {
		t.Fatalf("copying fixture tree: %v", err)
	}
	return dst
}

// withIsolatedHome points HOME at a fresh temp dir and clears BUILD82_MOODLE_PATH, so configuration
// reads and writes never touch the real user's ~/.build82.
func withIsolatedHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("BUILD82_MOODLE_PATH", "")
}

// connectInMemory connects a new in-memory client to a fresh NewServer instance and returns the
// client, its session and a cleanup function that closes the session and cancels the context.
func connectInMemory(t *testing.T) (*mcp.Client, *mcp.ClientSession, func()) {
	t.Helper()
	s := NewServer()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	serverDone := make(chan error, 1)
	go func() {
		_, err := s.Connect(ctx, serverTransport, nil)
		serverDone <- err
	}()

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()
		t.Fatalf("client connect: %v", err)
	}

	cleanup := func() {
		session.Close()
		cancel()
	}
	return client, session, cleanup
}

// TestServer_ReportsIconInServerInfo verifies that serverInfo carries one valid 64x64 PNG data URI
// icon identical to the published icon asset.
func TestServer_ReportsIconInServerInfo(t *testing.T) {
	_, session, cleanup := connectInMemory(t)
	defer cleanup()

	info := session.InitializeResult().ServerInfo
	if info == nil || len(info.Icons) != 1 {
		t.Fatalf("expected exactly 1 icon in serverInfo, got %+v", info)
	}
	icon := info.Icons[0]
	if icon.MIMEType != "image/png" || len(icon.Sizes) != 1 || icon.Sizes[0] != "64x64" {
		t.Errorf("unexpected icon metadata: mimeType=%q sizes=%v", icon.MIMEType, icon.Sizes)
	}

	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(icon.Source, prefix) {
		t.Fatalf("icon src is not a PNG data URI: %.40q", icon.Source)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(icon.Source, prefix))
	if err != nil {
		t.Fatalf("decoding icon data URI: %v", err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("icon is not a valid PNG: %v", err)
	}
	if cfg.Width != 64 || cfg.Height != 64 {
		t.Errorf("expected a 64x64 icon, got %dx%d", cfg.Width, cfg.Height)
	}

	// The embedded icon must be byte-identical to the published icon asset.
	asset, err := os.ReadFile(filepath.Join("..", "..", "docs", "img", "icons", "icon-build82-cropped-64.png"))
	if err != nil {
		t.Fatalf("reading icon asset: %v", err)
	}
	if !bytes.Equal(raw, asset) {
		t.Error("internal/server/icon.png differs from docs/img/icons/icon-build82-cropped-64.png")
	}
}

// TestServer_ListsAllToolsResourcesPrompts verifies the number of registered tools, resources,
// resource templates and prompts.
func TestServer_ListsAllToolsResourcesPrompts(t *testing.T) {
	_, session, cleanup := connectInMemory(t)
	defer cleanup()
	ctx := context.Background()

	toolsResult, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(toolsResult.Tools) != 13 {
		names := make([]string, len(toolsResult.Tools))
		for i, tl := range toolsResult.Tools {
			names[i] = tl.Name
		}
		t.Errorf("expected 13 tools, got %d: %v", len(toolsResult.Tools), names)
	}

	resourcesResult, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	// 13 global + 1 static plugin aggregate = 14.
	if len(resourcesResult.Resources) != 14 {
		t.Errorf("expected 14 resources, got %d", len(resourcesResult.Resources))
	}

	templatesResult, err := session.ListResourceTemplates(ctx, nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	if len(templatesResult.ResourceTemplates) != 12 {
		t.Errorf("expected 12 resource templates, got %d", len(templatesResult.ResourceTemplates))
	}

	promptsResult, err := session.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	if len(promptsResult.Prompts) != 3 {
		t.Errorf("expected 3 prompts, got %d", len(promptsResult.Prompts))
	}
}

// TestServer_CallInitMoodleContext_EndToEnd verifies that calling init_moodle_context on the fixture
// succeeds and generates AI_CONTEXT.md.
func TestServer_CallInitMoodleContext_EndToEnd(t *testing.T) {
	withIsolatedHome(t)
	moodlePath := copyFixtureMoodleTree(t)

	_, session, cleanup := connectInMemory(t)
	defer cleanup()
	ctx := context.Background()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "init_moodle_context",
		Arguments: map[string]any{"moodle_path": moodlePath},
	})
	if err != nil {
		t.Fatalf("CallTool init_moodle_context: %v", err)
	}
	if result.IsError {
		if tc, ok := result.Content[0].(*mcp.TextContent); ok {
			t.Fatalf("expected success, got IsError with text: %s", tc.Text)
		}
		t.Fatalf("expected success, got IsError with content: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected exactly one content block, got %d", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, "initialized successfully") {
		t.Errorf("unexpected response content: %+v", result.Content[0])
	}

	if _, statErr := os.Stat(filepath.Join(moodlePath, ".build82", "AI_CONTEXT.md")); statErr != nil {
		t.Errorf("expected AI_CONTEXT.md to be generated: %v", statErr)
	}
}

// TestServer_CallInitMoodleContext_JSONFormat verifies that format "json" returns a JSON document
// with success set to true.
func TestServer_CallInitMoodleContext_JSONFormat(t *testing.T) {
	withIsolatedHome(t)
	moodlePath := copyFixtureMoodleTree(t)

	_, session, cleanup := connectInMemory(t)
	defer cleanup()
	ctx := context.Background()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "init_moodle_context",
		Arguments: map[string]any{"moodle_path": moodlePath, "format": "json"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got IsError: %+v", result.Content)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	var parsed map[string]any
	if jsonErr := json.Unmarshal([]byte(text), &parsed); jsonErr != nil {
		t.Fatalf("expected valid JSON in the text block, got %q: %v", text, jsonErr)
	}
	if parsed["success"] != true {
		t.Errorf("expected success=true in JSON output, got %+v", parsed)
	}
}

// TestServer_ReadGlobalResource_NotInitializedPlaceholder verifies that reading a global resource
// before initialization returns a placeholder text.
func TestServer_ReadGlobalResource_NotInitializedPlaceholder(t *testing.T) {
	withIsolatedHome(t)

	_, session, cleanup := connectInMemory(t)
	defer cleanup()
	ctx := context.Background()

	result, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "moodle://context"})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(result.Contents) != 1 || !strings.Contains(result.Contents[0].Text, "not been initialized") {
		t.Errorf("expected a not-initialized placeholder, got %+v", result.Contents)
	}
}

// TestServer_GetScaffoldPrompt verifies that the scaffold_plugin prompt returns the expected
// user/assistant/user message sequence.
func TestServer_GetScaffoldPrompt(t *testing.T) {
	withIsolatedHome(t)

	_, session, cleanup := connectInMemory(t)
	defer cleanup()
	ctx := context.Background()

	result, err := session.GetPrompt(ctx, &mcp.GetPromptParams{
		Name: "scaffold_plugin",
		Arguments: map[string]string{
			"type": "local", "name": "demo", "description": "A demo plugin.",
		},
	})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if len(result.Messages) != 3 {
		t.Fatalf("expected 3 messages (few-shot pair + real request), got %d", len(result.Messages))
	}
	if result.Messages[0].Role != "user" || result.Messages[1].Role != "assistant" || result.Messages[2].Role != "user" {
		t.Errorf("expected user/assistant/user role sequence, got %v/%v/%v",
			result.Messages[0].Role, result.Messages[1].Role, result.Messages[2].Role)
	}
}
