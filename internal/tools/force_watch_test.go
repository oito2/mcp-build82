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
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/cache"
	"github.com/oito2/mcp-build82/internal/generators"
)

// copyForceWatchFixture copies testdata/moodle into a fresh temp directory and backdates every
// file by one hour, so generated outputs are always strictly newer than their sources and a pure
// mtime comparison would report them fresh.
func copyForceWatchFixture(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "moodle")
	dst := t.TempDir()
	past := time.Now().Add(-1 * time.Hour)
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
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
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if writeErr := os.WriteFile(target, content, 0o644); writeErr != nil {
			return writeErr
		}
		return os.Chtimes(target, past, past)
	})
	if err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return dst
}

// setupForceWatchEnv isolates HOME and points the configuration at moodlePath.
func setupForceWatchEnv(t *testing.T, moodlePath string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("BUILD82_MOODLE_PATH", moodlePath)
}

// simulateRestart replaces the process-wide cache with an empty one, as a new server process
// would have: the next EnsureLoaded reloads the marks persisted on disk by the previous run.
func simulateRestart(t *testing.T) {
	t.Helper()
	cache.Global = cache.NewMtimeCache()
}

// resultText returns the text of the single text block of `res`, failing the test when absent.
func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		t.Fatalf("empty tool result")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected content type %T", res.Content[0])
	}
	return text.Text
}

// TestPluginBatch_ForceRegeneratesEveryFileAfterRestart runs plugin_batch once to populate the
// persisted cache, restarts the cache as a new process would, and then runs it again with
// force: true. Every plugin context file must be regenerated even though every output is newer
// than its sources.
func TestPluginBatch_ForceRegeneratesEveryFileAfterRestart(t *testing.T) {
	freshCache(t)
	moodlePath := copyForceWatchFixture(t)
	setupForceWatchEnv(t, moodlePath)

	in := BatchInput{Mode: BatchModeList, Plugins: []string{"local/demo"}, Format: FormatJSON}
	res, _, err := handleBatch(context.Background(), nil, in)
	if err != nil || res.IsError {
		t.Fatalf("first plugin_batch failed: err=%v text=%s", err, resultText(t, res))
	}

	simulateRestart(t)

	// Without force the second run must be fully cached; this guards the premise of the test.
	res, _, err = handleBatch(context.Background(), nil, in)
	if err != nil || res.IsError {
		t.Fatalf("cached plugin_batch failed: err=%v text=%s", err, resultText(t, res))
	}
	var cachedOut BatchOutput
	if err := json.Unmarshal([]byte(resultText(t, res)), &cachedOut); err != nil {
		t.Fatalf("decode: %v", err)
	}
	cached := cachedOut.Plugins
	if len(cached) != 1 || cached[0].Generated != 0 {
		t.Fatalf("expected a fully cached run without force, got %+v", cached)
	}

	simulateRestart(t)

	in.Force = true
	res, _, err = handleBatch(context.Background(), nil, in)
	if err != nil || res.IsError {
		t.Fatalf("forced plugin_batch failed: err=%v text=%s", err, resultText(t, res))
	}
	var forcedOut BatchOutput
	if err := json.Unmarshal([]byte(resultText(t, res)), &forcedOut); err != nil {
		t.Fatalf("decode: %v", err)
	}
	forced := forcedOut.Plugins
	if len(forced) != 1 {
		t.Fatalf("expected one plugin result, got %+v", forced)
	}
	want := len(generators.PluginContextFiles)
	if forced[0].Generated != want || forced[0].Skipped != 0 || forced[0].Failed != 0 {
		t.Errorf("force: true must regenerate all %d files, got generated=%d cached=%d failed=%d",
			want, forced[0].Generated, forced[0].Skipped, forced[0].Failed)
	}
}

// TestUpdateIndexes_ForceRegeneratesEveryFileAfterRestart covers update_indexes in a fresh
// process: the forced invalidation must not be undone by the cache loading its persisted marks,
// and must cover both the global indexes and the include_plugins path.
func TestUpdateIndexes_ForceRegeneratesEveryFileAfterRestart(t *testing.T) {
	freshCache(t)
	moodlePath := copyForceWatchFixture(t)
	setupForceWatchEnv(t, moodlePath)

	markerDir := filepath.Join(moodlePath, "local", "demo", generators.ContextDir)
	mustMkdirAll(t, markerDir)
	mustWriteFile(t, filepath.Join(markerDir, ".indevelopment"), "1\n")

	in := UpdateIndexesInput{IncludePlugins: true, Format: FormatJSON}
	res, _, err := handleUpdateIndexes(context.Background(), nil, in)
	if err != nil || res.IsError {
		t.Fatalf("first update_indexes failed: err=%v text=%s", err, resultText(t, res))
	}

	simulateRestart(t)

	in.Force = true
	res, _, err = handleUpdateIndexes(context.Background(), nil, in)
	if err != nil || res.IsError {
		t.Fatalf("forced update_indexes failed: err=%v text=%s", err, resultText(t, res))
	}
	var out UpdateIndexesOutput
	if err := json.Unmarshal([]byte(resultText(t, res)), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for _, f := range generators.GlobalContextFilenames {
		rel := filepath.ToSlash(filepath.Join(generators.ContextDir, f))
		found := slices.ContainsFunc(out.Regenerated, func(r string) bool { return filepath.ToSlash(r) == rel })
		if !found {
			t.Errorf("force: true must regenerate %s, regenerated=%v skipped=%v", rel, out.Regenerated, out.Skipped)
		}
	}

	wantPlugin := "(12 regenerated, 0 cached, 0 failed)"
	if len(out.Plugins) != 1 || !strings.Contains(out.Plugins[0], wantPlugin) {
		t.Errorf("force: true must regenerate every plugin file, want %q, got %v", wantPlugin, out.Plugins)
	}
}

// TestWatchPlugins_StartWithoutDevPluginsDoesNotActivate starts the watcher on an installation
// with no .indevelopment plugin: the result must say nothing was started (as a non-error), and
// neither status nor stop may pretend a watcher exists.
func TestWatchPlugins_StartWithoutDevPluginsDoesNotActivate(t *testing.T) {
	freshCache(t)
	moodlePath := copyForceWatchFixture(t)
	setupForceWatchEnv(t, moodlePath)

	activeWatcherMu.Lock()
	previous := activeWatcher
	activeWatcher = nil
	activeWatcherMu.Unlock()
	t.Cleanup(func() {
		activeWatcherMu.Lock()
		if activeWatcher != nil {
			activeWatcher.Stop()
		}
		activeWatcher = previous
		activeWatcherMu.Unlock()
	})

	handle := makeHandleWatch(mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil))

	res, _, err := handle(context.Background(), nil, WatchInput{Action: WatchStart})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	text := resultText(t, res)
	if res.IsError {
		t.Fatalf("starting with nothing to watch must not be an error result, got: %s", text)
	}
	if strings.Contains(text, "Watcher started") || !strings.Contains(text, "not started") {
		t.Errorf("expected a clear not-started message, got: %s", text)
	}

	activeWatcherMu.Lock()
	registered := activeWatcher != nil
	activeWatcherMu.Unlock()
	if registered {
		t.Error("no watcher may be registered as active when nothing is being watched")
	}

	res, _, _ = handle(context.Background(), nil, WatchInput{Action: WatchStatus})
	if got := resultText(t, res); !strings.Contains(got, "not running") {
		t.Errorf("status must report the watcher as not running, got: %s", got)
	}

	res, _, _ = handle(context.Background(), nil, WatchInput{Action: WatchStop})
	if got := resultText(t, res); !strings.Contains(got, "No active watcher") {
		t.Errorf("stop must report there is no active watcher, got: %s", got)
	}
}
