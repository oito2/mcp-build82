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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectLoggingClient connects a new client session to `s` with the "info" logging level and
// returns it with a channel that receives the string Data of every logging notification.
func connectLoggingClient(t *testing.T, ctx context.Context, s *mcp.Server) (*mcp.ClientSession, <-chan string) {
	t.Helper()
	received := make(chan string, 8)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, &mcp.ClientOptions{
		LoggingMessageHandler: func(_ context.Context, r *mcp.LoggingMessageRequest) { //nolint:staticcheck // receives the watcher's log notifications
			if msg, ok := r.Params.Data.(string); ok {
				received <- msg
			}
		},
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = s.Connect(ctx, serverTransport, nil) }()

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	if err := session.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "info"}); err != nil { //nolint:staticcheck // enables the watcher's log notifications
		t.Fatalf("set logging level: %v", err)
	}
	return session, received
}

// TestWatchPlugins_NotifiesAllConnectedSessions verifies that a watcher change notification
// reaches every session connected to the server, not only the one that started the watcher. Two
// sessions connect, one starts the watcher, a real file change is made, and both must be notified.
func TestWatchPlugins_NotifiesAllConnectedSessions(t *testing.T) {
	withIsolatedHome(t)
	root := copyFixtureMoodleTree(t)
	markDev(t, filepath.Join(root, "local", "demo"))
	t.Setenv("BUILD82_MOODLE_PATH", root)

	s := NewServer()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sessionA, msgsA := connectLoggingClient(t, ctx, s)
	_, msgsB := connectLoggingClient(t, ctx, s) // never calls action=start itself

	startResult := callTool(t, sessionA, "watch_plugins", map[string]any{"action": "start"})
	if startResult.IsError {
		t.Fatalf("watch_plugins start failed: %s", toolText(t, startResult))
	}
	t.Cleanup(func() { callTool(t, sessionA, "watch_plugins", map[string]any{"action": "stop"}) })

	// Modify a watched file (version.php) to trigger a real filesystem event.
	versionPhp := filepath.Join(root, "local", "demo", "version.php")
	content, err := os.ReadFile(versionPhp)
	if err != nil {
		t.Fatalf("read version.php: %v", err)
	}
	if err := os.WriteFile(versionPhp, append(content, '\n'), 0o644); err != nil {
		t.Fatalf("touch version.php: %v", err)
	}

	for name, ch := range map[string]<-chan string{
		"session A (started the watcher)": msgsA,
		"session B (never called start)":  msgsB,
	} {
		select {
		case msg := <-ch:
			if !strings.Contains(msg, "local_demo") {
				t.Errorf("%s: unexpected log message: %q", name, msg)
			}
		case <-time.After(10 * time.Second):
			t.Errorf("%s: timed out waiting for a watcher change notification", name)
		}
	}
}
