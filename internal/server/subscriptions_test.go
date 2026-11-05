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

// connectSubscriber connects a new client session to `s` and returns it with a channel that
// receives the URI of every notifications/resources/updated it gets.
func connectSubscriber(t *testing.T, ctx context.Context, s *mcp.Server) (*mcp.ClientSession, <-chan string) {
	t.Helper()
	updated := make(chan string, 64)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, &mcp.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, r *mcp.ResourceUpdatedNotificationRequest) {
			updated <- r.Params.URI
		},
	})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = s.Connect(ctx, serverTransport, nil) }()
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session, updated
}

// subscribeAll subscribes `session` to every URI in `uris`, failing the test on an error.
func subscribeAll(t *testing.T, session *mcp.ClientSession, uris ...string) {
	t.Helper()
	for _, uri := range uris {
		if err := session.Subscribe(context.Background(), &mcp.SubscribeParams{URI: uri}); err != nil {
			t.Fatalf("subscribe %s: %v", uri, err)
		}
	}
}

// collectUpdates returns the set of URIs received on `ch` until no notification arrives for
// `quiet`.
func collectUpdates(ch <-chan string, quiet time.Duration) map[string]bool {
	got := map[string]bool{}
	for {
		select {
		case uri := <-ch:
			got[uri] = true
		case <-time.After(quiet):
			return got
		}
	}
}

// TestSubscriptions_AdvertisedAndValidated verifies that the server advertises resource
// subscriptions, accepts every URI it serves (including a plugin whose files are not generated
// yet) and refuses an unknown one.
func TestSubscriptions_AdvertisedAndValidated(t *testing.T) {
	withIsolatedHome(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, _ := connectSubscriber(t, ctx, NewServer())

	caps := session.InitializeResult().Capabilities
	if caps.Resources == nil || !caps.Resources.Subscribe {
		t.Fatalf("expected resources.subscribe to be advertised, got %+v", caps.Resources)
	}
	subscribeAll(t, session,
		"moodle://index", "moodle://plugins/with-context",
		"moodle://plugin/local_notyet", "moodle://plugin/local_notyet/flow")
}

// TestSubscribe_RefusesUnknownURIs verifies the subscription handler: every URI the server serves
// is accepted, including a plugin whose files are not generated yet, and anything else is refused.
// (A client on the newer protocol subscribes through subscriptions/listen, which does not return
// this error to the caller, so the handler is tested directly; the stdio e2e test covers the
// error a resources/subscribe client receives.)
func TestSubscribe_RefusesUnknownURIs(t *testing.T) {
	for _, uri := range []string{"moodle://context", "moodle://index", "moodle://plugins/with-context", "moodle://plugin/local_notyet", "moodle://plugin/mod_forum/flow"} {
		if err := subscribe(context.Background(), &mcp.SubscribeRequest{Params: &mcp.SubscribeParams{URI: uri}}); err != nil {
			t.Errorf("subscribe %s: unexpected error %v", uri, err)
		}
	}
	for _, uri := range []string{"", "https://example.com/x", "moodle://nope", "moodle://plugin/", "moodle://plugin/Local_Bad/context", "moodle://plugin/local_demo/unknown", "moodle://plugin/../etc/context", "moodle://plugin/local_demo/context/extra"} {
		if err := subscribe(context.Background(), &mcp.SubscribeRequest{Params: &mcp.SubscribeParams{URI: uri}}); err == nil {
			t.Errorf("subscribe %q: expected an error", uri)
		}
	}
}

// TestSubscriptions_ToolsNotifySubscribedResources verifies that generate_plugin_context announces
// the plugin's resources and the plugin indexes to their subscribers only, that unsubscribing
// stops the notifications, that init_moodle_context announces the global resources, and that a
// failed call announces nothing.
func TestSubscriptions_ToolsNotifySubscribedResources(t *testing.T) {
	withIsolatedHome(t)
	root := copyFixtureMoodleTree(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	session, updates := connectSubscriber(t, ctx, NewServer())

	subscribeAll(t, session, "moodle://context", "moodle://plugin-index")
	if res := callTool(t, session, "init_moodle_context", map[string]any{"moodle_path": root}); res.IsError {
		t.Fatalf("init: %s", toolText(t, res))
	}
	if got := collectUpdates(updates, 300*time.Millisecond); !got["moodle://context"] || !got["moodle://plugin-index"] || len(got) != 2 {
		t.Errorf("init: expected exactly the 2 subscribed global resources, got %v", got)
	}

	subscribeAll(t, session, "moodle://plugin/local_demo", "moodle://plugin/local_demo/context", "moodle://index")
	if res := callTool(t, session, "generate_plugin_context", map[string]any{"plugin_path": "local_demo"}); res.IsError {
		t.Fatalf("generate: %s", toolText(t, res))
	}
	got := collectUpdates(updates, 300*time.Millisecond)
	for _, uri := range []string{"moodle://plugin/local_demo", "moodle://plugin/local_demo/context", "moodle://index"} {
		if !got[uri] {
			t.Errorf("generate: expected an update for %s, got %v", uri, got)
		}
	}
	if got["moodle://plugin/local_demo/flow"] || got["moodle://context"] {
		t.Errorf("generate: unsubscribed or unchanged resources must not be announced, got %v", got)
	}

	if err := session.Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: "moodle://plugin/local_demo/context"}); err != nil {
		t.Fatal(err)
	}
	callTool(t, session, "generate_plugin_context", map[string]any{"plugin_path": "local_demo"})
	if got := collectUpdates(updates, 300*time.Millisecond); got["moodle://plugin/local_demo/context"] || !got["moodle://plugin/local_demo"] {
		t.Errorf("after unsubscribe: expected only the still-subscribed resources, got %v", got)
	}

	if res := callTool(t, session, "generate_plugin_context", map[string]any{"plugin_path": "local_nosuchplugin"}); !res.IsError {
		t.Fatal("expected an error for an unknown plugin")
	}
	if got := collectUpdates(updates, 300*time.Millisecond); len(got) != 0 {
		t.Errorf("a failed call must announce nothing, got %v", got)
	}
}

// TestSubscriptions_WatcherNotifiesSubscribers verifies that a regeneration triggered by the
// watcher announces the plugin's resources to a subscribed session that never started the watcher.
func TestSubscriptions_WatcherNotifiesSubscribers(t *testing.T) {
	withIsolatedHome(t)
	root := copyFixtureMoodleTree(t)
	markDev(t, filepath.Join(root, "local", "demo"))
	t.Setenv("BUILD82_MOODLE_PATH", root)

	s := NewServer()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	starter, _ := connectSubscriber(t, ctx, s)
	subscriber, updates := connectSubscriber(t, ctx, s)
	subscribeAll(t, subscriber, "moodle://plugin/local_demo/context")

	if res := callTool(t, starter, "watch_plugins", map[string]any{"action": "start"}); res.IsError {
		t.Fatalf("watch_plugins start: %s", toolText(t, res))
	}
	t.Cleanup(func() { callTool(t, starter, "watch_plugins", map[string]any{"action": "stop"}) })

	versionPhp := filepath.Join(root, "local", "demo", "version.php")
	content, err := os.ReadFile(versionPhp)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(versionPhp, append(content, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case uri := <-updates:
		if uri != "moodle://plugin/local_demo/context" {
			t.Errorf("unexpected update for %s", uri)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the watcher's resource update")
	}
}

// TestSubscriptions_WithContextFreshAfterUpdate verifies that moodle://plugins/with-context, read
// right after a tool announced it as updated, lists the plugin that was just generated instead of
// the listing cached before the generation.
func TestSubscriptions_WithContextFreshAfterUpdate(t *testing.T) {
	withIsolatedHome(t)
	root := copyFixtureMoodleTree(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	session, _ := connectSubscriber(t, ctx, NewServer())

	if res := callTool(t, session, "init_moodle_context", map[string]any{"moodle_path": root}); res.IsError {
		t.Fatalf("init: %s", toolText(t, res))
	}
	read := func() string {
		t.Helper()
		res, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "moodle://plugins/with-context"})
		if err != nil || len(res.Contents) != 1 {
			t.Fatalf("read with-context: %v %+v", err, res)
		}
		return res.Contents[0].Text
	}
	if strings.Contains(read(), "local_demo") {
		t.Fatal("test setup: local_demo must not have context before it is generated")
	}
	if res := callTool(t, session, "generate_plugin_context", map[string]any{"plugin_path": "local_demo"}); res.IsError {
		t.Fatalf("generate: %s", toolText(t, res))
	}
	if !strings.Contains(read(), "local_demo") {
		t.Error("the listing read right after the update announcement is stale")
	}
}
