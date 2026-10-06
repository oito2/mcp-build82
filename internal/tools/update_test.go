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

package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/genutil"
	"github.com/oito2/mcp-build82/internal/watcher"
)

// TestPluginPanicLine_UsesMoodleRelativePath verifies the recovered-panic line names the plugin
// relative to the Moodle root and never contains the absolute host path.
func TestPluginPanicLine_UsesMoodleRelativePath(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "srv", "moodle")
	got := pluginPanicLine(root, filepath.Join(root, "local", "demo"), "boom")
	if got != "✖ local/demo: boom" {
		t.Errorf("unexpected line %q", got)
	}
	if strings.Contains(got, root) {
		t.Errorf("line leaks the absolute path: %q", got)
	}
}

// TestWatchStart_CallbackSeesChangesDuringStart verifies that the watch_plugins notification
// callback is registered before the watcher starts: a change made and regenerated while Start is
// still in progress must reach the connected session as a log notification.
func TestWatchStart_CallbackSeesChangesDuringStart(t *testing.T) {
	root := copyForceWatchFixture(t)
	setupForceWatchEnv(t, root)
	pluginDir := filepath.Join(root, "local", "demo")
	if err := os.MkdirAll(filepath.Join(pluginDir, genutil.ContextDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, genutil.ContextDir, ".indevelopment"), []byte("ts"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil)
	received := make(chan string, 8)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, &mcp.ClientOptions{
		LoggingMessageHandler: func(_ context.Context, r *mcp.LoggingMessageRequest) {
			if msg, ok := r.Params.Data.(string); ok {
				received <- msg
			}
		},
	})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	if err := session.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "info"}); err != nil {
		t.Fatalf("set logging level: %v", err)
	}

	// Start the watcher, then change version.php and wait for that regeneration to finish before
	// returning control to the handler. The probe callback is registered after any callback the
	// handler registered earlier, so it runs last.
	orig := startWatcher
	startWatcher = func(w *watcher.MoodleWatcher) int {
		done := make(chan struct{}, 1)
		w.OnChange(func(watcher.WatchEvent) {
			select {
			case done <- struct{}{}:
			default:
			}
		})
		count := orig(w)
		versionPhp := filepath.Join(pluginDir, "version.php")
		content, readErr := os.ReadFile(versionPhp)
		if readErr != nil {
			t.Errorf("read version.php: %v", readErr)
			return count
		}
		if writeErr := os.WriteFile(versionPhp, append(content, '\n'), 0o644); writeErr != nil {
			t.Errorf("touch version.php: %v", writeErr)
			return count
		}
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("timed out waiting for the regeneration triggered during Start")
		}
		return count
	}
	t.Cleanup(func() { startWatcher = orig })

	handle := makeHandleWatch(server)
	res, _, _ := handle(ctx, nil, WatchInput{Action: WatchStart})
	t.Cleanup(func() { _, _, _ = handle(context.Background(), nil, WatchInput{Action: WatchStop}) })
	if res.IsError {
		t.Fatalf("watch_plugins start failed: %+v", res.Content)
	}

	select {
	case msg := <-received:
		if !strings.Contains(msg, "local_demo") {
			t.Errorf("unexpected log message: %q", msg)
		}
	case <-time.After(5 * time.Second):
		t.Error("the change made during Start produced no log notification")
	}
}
