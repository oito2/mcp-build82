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
	"testing"
	"time"

	"github.com/oito2/mcp-build82/internal/resources"
)

// TestWatchPlugins_NotifiesAllConnectedSessions verifies that a watcher regeneration is announced
// with notifications/resources/updated to every session subscribed to the plugin's resources, not
// only the one that started the watcher. Two sessions subscribe, one starts the watcher, a real
// file change is made, and both must be notified.
func TestWatchPlugins_NotifiesAllConnectedSessions(t *testing.T) {
	withIsolatedHome(t)
	root := copyFixtureMoodleTree(t)
	markDev(t, filepath.Join(root, "local", "demo"))
	t.Setenv("BUILD82_MOODLE_PATH", root)

	s := NewServer()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	contextURI := resources.PluginURIs("local_demo")[0]
	sessionA, updatesA := connectSubscriber(t, ctx, s)
	sessionB, updatesB := connectSubscriber(t, ctx, s) // never calls action=start itself
	subscribeAll(t, sessionA, contextURI)
	subscribeAll(t, sessionB, contextURI)

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
		"session A (started the watcher)": updatesA,
		"session B (never called start)":  updatesB,
	} {
		select {
		case uri := <-ch:
			if uri != contextURI {
				t.Errorf("%s: unexpected resource update %q", name, uri)
			}
		case <-time.After(10 * time.Second):
			t.Errorf("%s: timed out waiting for a watcher change notification", name)
		}
	}
}
