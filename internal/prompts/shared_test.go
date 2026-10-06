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

package prompts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mustMkdirAll creates `path` and any missing parents, failing the test on error.
func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

// mustWriteFile writes `content` to `path`, failing the test on error.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestResolvePluginForPrompt_PathTraversalIsRejected verifies that an absolute plugin path outside
// the Moodle root, even with a real version.php there, leaks neither its metadata nor its file
// content through resolvePluginForPrompt.
func TestResolvePluginForPrompt_PathTraversalIsRejected(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", root)

	outside := t.TempDir()
	mustWriteFile(t, filepath.Join(outside, "version.php"),
		"<?php\n$plugin->component = 'local_secret_outside';\n$plugin->version = 2024010100;\n")
	mustMkdirAll(t, filepath.Join(outside, ".build82"))
	mustWriteFile(t, filepath.Join(outside, ".build82", "PLUGIN_CONTEXT.md"), "SECRET EXTERNAL CONTENT")

	rc := resolvePluginForPrompt(outside)
	if rc.Component == "local_secret_outside" {
		t.Fatalf("path traversal leaked external plugin metadata: %+v", rc)
	}
	if text := rc.readPluginFileTruncated("PLUGIN_CONTEXT.md", 1000); strings.Contains(text, "SECRET EXTERNAL CONTENT") {
		t.Fatalf("path traversal leaked external file content: %s", text)
	}
}

// TestResolvePluginForPrompt_WithinRootResolvesNormally verifies that a plugin identifier resolving
// inside the Moodle root has its metadata detected and its files readable.
func TestResolvePluginForPrompt_WithinRootResolvesNormally(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BUILD82_MOODLE_PATH", root)

	pluginDir := filepath.Join(root, "local", "demo")
	mustMkdirAll(t, pluginDir)
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"),
		"<?php\n$plugin->component = 'local_demo';\n$plugin->version = 2024010100;\n")
	mustMkdirAll(t, filepath.Join(pluginDir, ".build82"))
	mustWriteFile(t, filepath.Join(pluginDir, ".build82", "PLUGIN_CONTEXT.md"), "real content")

	rc := resolvePluginForPrompt("local/demo")
	if rc.Component != "local_demo" {
		t.Errorf("expected component 'local_demo', got %+v", rc)
	}
	if text := rc.readPluginFileTruncated("PLUGIN_CONTEXT.md", 1000); text != "real content" {
		t.Errorf("expected 'real content', got %q", text)
	}
}

// promptReq builds a GetPromptRequest carrying the given prompt arguments.
func promptReq(args map[string]string) *mcp.GetPromptRequest {
	return &mcp.GetPromptRequest{Params: &mcp.GetPromptParams{Arguments: args}}
}

// TestRequireArgs verifies that requireArgs errors on a missing or blank required argument and
// accepts a complete set.
func TestRequireArgs(t *testing.T) {
	if err := requireArgs(map[string]string{"a": "x", "b": "y"}, "a", "b"); err != nil {
		t.Errorf("expected no error when all required args are present, got: %v", err)
	}
	if err := requireArgs(map[string]string{"a": "x"}, "a", "b"); err == nil {
		t.Error("expected an error when a required arg is missing")
	}
	if err := requireArgs(map[string]string{"a": "x", "b": "  "}, "a", "b"); err == nil {
		t.Error("expected an error when a required arg is present but blank/whitespace-only")
	}
}

// TestHandleScaffoldPrompt_MissingRequiredArgsIsError verifies scaffold_plugin rejects a request
// missing required arguments.
func TestHandleScaffoldPrompt_MissingRequiredArgsIsError(t *testing.T) {
	if _, err := handleScaffoldPrompt(context.Background(), promptReq(map[string]string{"name": "demo"})); err == nil {
		t.Error("expected an error when 'type'/'description' are missing")
	}
}

// TestHandleReviewPrompt_MissingRequiredArgIsError verifies review_plugin rejects a request
// without "plugin".
func TestHandleReviewPrompt_MissingRequiredArgIsError(t *testing.T) {
	if _, err := handleReviewPrompt(context.Background(), promptReq(map[string]string{})); err == nil {
		t.Error("expected an error when 'plugin' is missing")
	}
}

// TestHandleDebugPrompt_MissingRequiredArgsIsError verifies debug_plugin rejects a request
// without "error".
func TestHandleDebugPrompt_MissingRequiredArgsIsError(t *testing.T) {
	if _, err := handleDebugPrompt(context.Background(), promptReq(map[string]string{"plugin": "local_demo"})); err == nil {
		t.Error("expected an error when 'error' is missing")
	}
}

// TestWithRecoverPrompt_ConvertsPanicToErrorResult verifies that a panic inside a wrapped handler
// is returned as an error with a nil result and the panic message preserved.
func TestWithRecoverPrompt_ConvertsPanicToErrorResult(t *testing.T) {
	panicky := func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		panic("boom: simulated prompt handler failure")
	}
	wrapped := withRecoverPrompt(panicky)

	result, err := wrapped(context.Background(), promptReq(nil))
	if err == nil {
		t.Fatal("expected the panic to be converted into a returned error")
	}
	if result != nil {
		t.Errorf("expected a nil result alongside the error, got: %+v", result)
	}
	if !strings.Contains(err.Error(), "boom: simulated prompt handler failure") {
		t.Errorf("expected the panic message in the error, got: %v", err)
	}
}

// TestWithRecoverPrompt_PassesThroughNormalResults verifies the wrapper returns a non-panicking
// handler's result unchanged.
func TestWithRecoverPrompt_PassesThroughNormalResults(t *testing.T) {
	normal := func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: "all good"}}},
		}, nil
	}
	wrapped := withRecoverPrompt(normal)

	result, err := wrapped(context.Background(), promptReq(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected exactly 1 message, got %d", len(result.Messages))
	}
	if tc, ok := result.Messages[0].Content.(*mcp.TextContent); !ok || tc.Text != "all good" {
		t.Errorf("expected the handler's own result to pass through unchanged, got: %+v", result.Messages[0].Content)
	}
}

// connectReviewPrompt registers only review_plugin on a fresh server and connects a real client to
// it over the SDK's in-memory transport.
func connectReviewPrompt(t *testing.T) (context.Context, *mcp.ClientSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	server := mcp.NewServer(&mcp.Implementation{Name: "prompts-test", Version: "0.0.0"}, nil)
	RegisterReviewPrompt(server)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "prompts-test-client", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return ctx, session
}

// TestReviewPrompt_UnknownFocusIsInvalidParamsError verifies an unknown focus yields a jsonrpc
// invalid-params error listing the accepted values.
func TestReviewPrompt_UnknownFocusIsInvalidParamsError(t *testing.T) {
	_, pluginPath := setupPromptPlugin(t)
	ctx, session := connectReviewPrompt(t)

	_, err := session.GetPrompt(ctx, &mcp.GetPromptParams{
		Name:      "review_plugin",
		Arguments: map[string]string{"plugin": pluginPath, "focus": "securty"},
	})
	if err == nil {
		t.Fatal("expected an error for an unknown focus value")
	}
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Errorf("expected a jsonrpc invalid-params error, got %T: %v", err, err)
	}
	for _, want := range []string{"securty", "all", "security", "performance", "standards", "database", "apis"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected the error to mention %q, got: %v", want, err)
		}
	}
}

// TestReviewPrompt_EveryAcceptedFocusRendersCriteria verifies every accepted focus value renders
// a non-empty criteria section.
func TestReviewPrompt_EveryAcceptedFocusRendersCriteria(t *testing.T) {
	_, pluginPath := setupPromptPlugin(t)
	ctx, session := connectReviewPrompt(t)

	for _, focus := range []string{"", "all", "security", "performance", "standards", "database", "apis"} {
		result, err := session.GetPrompt(ctx, &mcp.GetPromptParams{
			Name:      "review_plugin",
			Arguments: map[string]string{"plugin": pluginPath, "focus": focus},
		})
		if err != nil {
			t.Errorf("focus %q: unexpected error: %v", focus, err)
			continue
		}
		text := textContentOf(t, result.Messages[len(result.Messages)-1])
		if !strings.Contains(text, "### Review Criteria\n\n## ") {
			t.Errorf("focus %q: expected a non-empty review criteria section, got:\n%s", focus, text)
		}
	}
}

func TestTruncateArg_RuneSafe(t *testing.T) {
	in := strings.Repeat("a", maxPromptArgLen-1) + "é" + "tail"
	got := truncateArg(in)
	if !utf8.ValidString(got) {
		t.Fatalf("truncateArg produced invalid UTF-8")
	}
	if want := strings.Repeat("a", maxPromptArgLen-1) + "\n...(truncated)"; got != want {
		t.Errorf("truncateArg tail = %q", got[len(got)-30:])
	}
}
