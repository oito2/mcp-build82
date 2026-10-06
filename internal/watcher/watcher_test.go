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

package watcher

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/oito2/mcp-build82/internal/cache"
	"github.com/oito2/mcp-build82/internal/genutil"
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

// markPluginDev creates the plugin directory `pluginRel` under `moodlePath` with a version.php
// declaring `component` and the .build82/.indevelopment marker, and returns the plugin directory.
func markPluginDev(t *testing.T, moodlePath, pluginRel, component string) string {
	t.Helper()
	pluginDir := filepath.Join(moodlePath, pluginRel)
	mustMkdirAll(t, filepath.Join(pluginDir, genutil.ContextDir))
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"),
		"<?php\n$plugin->component = '"+component+"';\n$plugin->version = 2024010100;\n")
	mustWriteFile(t, filepath.Join(pluginDir, genutil.ContextDir, ".indevelopment"), "2026-07-27T00:00:00Z")
	return pluginDir
}

// freshCache replaces cache.Global with an empty cache for the duration of the test.
func freshCache(t *testing.T) {
	t.Helper()
	old := cache.Global
	cache.Global = cache.NewMtimeCache()
	t.Cleanup(func() { cache.Global = old })
}

// TestStart_NoMarkers verifies that Start returns 0 and does not run when no plugin is dev-marked.
func TestStart_NoMarkers(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()

	w := NewMoodleWatcher(moodlePath, "4.3")
	if count := w.Start(); count != 0 {
		t.Errorf("expected 0 files watched, got %d", count)
	}
	if w.Running() {
		t.Error("expected Running()=false with no markers")
	}
}

// TestStart_MarkersButNothingWatchable verifies that Start returns 0 and does not run when a marked
// plugin has none of the watched source files.
func TestStart_MarkersButNothingWatchable(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	pluginDir := filepath.Join(moodlePath, "local", "bare")
	mustMkdirAll(t, filepath.Join(pluginDir, genutil.ContextDir))
	mustWriteFile(t, filepath.Join(pluginDir, genutil.ContextDir, ".indevelopment"), "ts")
	// No version.php, lib.php or db/*.php: none of the watched source files exist.

	w := NewMoodleWatcher(moodlePath, "4.3")
	if count := w.Start(); count != 0 {
		t.Errorf("expected 0 files watched (marker exists but nothing watchable), got %d", count)
	}
	if w.Running() {
		t.Error("expected Running()=false when nothing is watchable, even though a dev marker exists")
	}
	w.Stop()
}

// TestStart_NothingWatchableClosesWatcher verifies that when dev markers exist but no source file
// is watchable, Start closes the fsnotify watcher it created and keeps no reference to it.
func TestStart_NothingWatchableClosesWatcher(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	pluginDir := filepath.Join(moodlePath, "local", "bare")
	mustMkdirAll(t, filepath.Join(pluginDir, genutil.ContextDir))
	mustWriteFile(t, filepath.Join(pluginDir, genutil.ContextDir, ".indevelopment"), "ts")

	var created *fsnotify.Watcher
	orig := newFSWatcher
	newFSWatcher = func() (*fsnotify.Watcher, error) {
		fw, err := orig()
		created = fw
		return fw, err
	}
	t.Cleanup(func() { newFSWatcher = orig })

	w := NewMoodleWatcher(moodlePath, "4.3")
	if count := w.Start(); count != 0 {
		t.Fatalf("expected 0 files watched, got %d", count)
	}
	if created == nil {
		t.Fatal("expected Start to create an fsnotify watcher")
	}
	if err := created.Add(moodlePath); !errors.Is(err, fsnotify.ErrClosed) {
		t.Errorf("expected the unused fsnotify watcher to be closed (Add -> ErrClosed), got %v", err)
	}
	w.mu.Lock()
	kept := w.fsWatcher
	w.mu.Unlock()
	if kept != nil {
		t.Error("expected no fsnotify watcher to be kept when nothing is watchable")
	}
}

// TestStart_WatchesExistingFiles verifies that Start watches exactly the source files that exist.
func TestStart_WatchesExistingFiles(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	pluginDir := markPluginDev(t, moodlePath, filepath.Join("local", "demo"), "local_demo")
	mustWriteFile(t, filepath.Join(pluginDir, "lib.php"), "<?php\n")
	mustMkdirAll(t, filepath.Join(pluginDir, "db"))
	mustWriteFile(t, filepath.Join(pluginDir, "db", "events.php"), "<?php\n")

	w := NewMoodleWatcher(moodlePath, "4.3")
	defer w.Stop()

	// The watchable files present are version.php, lib.php and db/events.php.
	if count := w.Start(); count != 3 {
		t.Errorf("expected 3 files watched, got %d", count)
	}
	if !w.Running() {
		t.Error("expected Running()=true")
	}
}

// TestStart_SecondCallWithoutStopIsNoop verifies that calling Start again before Stop returns the
// count of the active run and keeps the same fsnotify watcher instead of creating a new one.
func TestStart_SecondCallWithoutStopIsNoop(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	pluginDir := markPluginDev(t, moodlePath, filepath.Join("local", "demo"), "local_demo")
	mustWriteFile(t, filepath.Join(pluginDir, "lib.php"), "<?php\n")

	w := NewMoodleWatcher(moodlePath, "4.3")
	defer w.Stop()

	firstCount := w.Start()
	if firstCount == 0 {
		t.Fatal("expected at least one watched file on the first Start()")
	}
	firstWatcher := w.fsWatcher
	if firstWatcher == nil {
		t.Fatal("expected fsWatcher to be set after the first Start()")
	}

	// A plugin added after the first Start would be picked up if the second call rescanned, so
	// the unchanged count and the identical watcher instance show that it did nothing.
	markPluginDev(t, moodlePath, filepath.Join("local", "other"), "local_other")

	secondCount := w.Start()
	if secondCount != firstCount {
		t.Errorf("expected a no-op second Start() to return the same count %d, got %d", firstCount, secondCount)
	}
	if w.fsWatcher != firstWatcher {
		t.Error("expected a no-op second Start() to keep the original *fsnotify.Watcher instance, not create a new one")
	}
	if !w.Running() {
		t.Error("expected Running()=true to still hold after the no-op second Start()")
	}
}

// TestStart_DoesNotBlockRunningDuringWalk verifies that Start does not hold the mutex during the
// marker walk, so a concurrent Running call returns quickly. findMarkersDelayHook makes the walk
// take a fixed time while Running is called.
func TestStart_DoesNotBlockRunningDuringWalk(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	markPluginDev(t, moodlePath, filepath.Join("local", "demo"), "local_demo")

	const walkDelay = 800 * time.Millisecond
	findMarkersDelayHook = func() { time.Sleep(walkDelay) }
	t.Cleanup(func() { findMarkersDelayHook = nil })

	w := NewMoodleWatcher(moodlePath, "4.3")
	defer w.Stop()

	startDone := make(chan int, 1)
	go func() { startDone <- w.Start() }()

	// Let Start reach the delayed walk, but return before the delay has elapsed.
	time.Sleep(150 * time.Millisecond)

	runningStart := time.Now()
	w.Running()
	elapsed := time.Since(runningStart)

	// An uncontended lock takes microseconds. If Start held the mutex during the walk, this call
	// would take about as long as the remaining delay.
	if elapsed >= walkDelay/2 {
		t.Errorf("Running() took %v while Start() was mid-walk (walk delay %v) — w.mu appears to be held during findMarkers", elapsed, walkDelay)
	}

	<-startDone // let Start finish before Stop and the test cleanup run
}

// TestStart_MaxWatchedPluginsCap verifies that only the first maxWatchedPlugins plugins are watched.
func TestStart_MaxWatchedPluginsCap(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()

	for i := 0; i < maxWatchedPlugins+3; i++ {
		name := "plugin" + string(rune('a'+i))
		markPluginDev(t, moodlePath, filepath.Join("local", name), "local_"+name)
	}

	w := NewMoodleWatcher(moodlePath, "4.3")
	defer w.Stop()

	count := w.Start()
	// Each plugin has only version.php, so one watched file per plugin.
	if count != maxWatchedPlugins {
		t.Errorf("expected exactly %d files watched (cap), got %d", maxWatchedPlugins, count)
	}
}

// waitForCallback returns the next event from `ch`, or nil when none arrives within `timeout`.
func waitForCallback(t *testing.T, ch <-chan WatchEvent, timeout time.Duration) *WatchEvent {
	t.Helper()
	select {
	case evt := <-ch:
		return &evt
	case <-time.After(timeout):
		return nil
	}
}

// TestDebounce_CoalescesRapidTouches verifies that rapid successive writes to one plugin file
// produce a single regeneration.
func TestDebounce_CoalescesRapidTouches(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	pluginDir := markPluginDev(t, moodlePath, filepath.Join("local", "demo"), "local_demo")

	w := NewMoodleWatcher(moodlePath, "4.3")
	defer w.Stop()
	if count := w.Start(); count == 0 {
		t.Fatal("expected at least one watched file")
	}

	events := make(chan WatchEvent, 10)
	w.OnChange(func(e WatchEvent) { events <- e })

	versionPhp := filepath.Join(pluginDir, "version.php")
	// Rapid writes, well within the debounce window.
	for i := 0; i < 5; i++ {
		mustWriteFile(t, versionPhp, "<?php\n$plugin->component = 'local_demo';\n$plugin->version = 2024010100;\n")
		time.Sleep(50 * time.Millisecond)
	}

	first := waitForCallback(t, events, 2*time.Second)
	if first == nil {
		t.Fatal("expected exactly one coalesced regeneration, got none")
	}
	if first.Component != "local_demo" {
		t.Errorf("expected component local_demo, got %q", first.Component)
	}

	// No second regeneration may follow.
	select {
	case extra := <-events:
		t.Errorf("expected only one coalesced regeneration, got an extra one: %+v", extra)
	case <-time.After(700 * time.Millisecond):
		// no extra event, as expected
	}
}

// TestDebounce_SecondPluginUnaffectedByFirst verifies that each plugin has its own debounce timer, so
// changes to two plugins regenerate both.
func TestDebounce_SecondPluginUnaffectedByFirst(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	pluginA := markPluginDev(t, moodlePath, filepath.Join("local", "a"), "local_a")
	pluginB := markPluginDev(t, moodlePath, filepath.Join("local", "b"), "local_b")

	w := NewMoodleWatcher(moodlePath, "4.3")
	defer w.Stop()
	if count := w.Start(); count == 0 {
		t.Fatal("expected watched files")
	}

	events := make(chan WatchEvent, 10)
	w.OnChange(func(e WatchEvent) { events <- e })

	mustWriteFile(t, filepath.Join(pluginA, "version.php"), "<?php\n$plugin->component = 'local_a';\n$plugin->version = 2024010100;\n")
	time.Sleep(200 * time.Millisecond) // within plugin A's debounce window
	mustWriteFile(t, filepath.Join(pluginB, "version.php"), "<?php\n$plugin->component = 'local_b';\n$plugin->version = 2024010100;\n")

	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		evt := waitForCallback(t, events, 2*time.Second)
		if evt == nil {
			t.Fatalf("expected 2 independent regenerations, got %d", len(seen))
		}
		seen[evt.Component] = true
	}
	if !seen["local_a"] || !seen["local_b"] {
		t.Errorf("expected both plugins to regenerate independently, got %+v", seen)
	}
}

// TestStop_IsIdempotentAndClearsState verifies that Stop marks the watcher as not running and can be
// called twice.
func TestStop_IsIdempotentAndClearsState(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	markPluginDev(t, moodlePath, filepath.Join("local", "demo"), "local_demo")

	w := NewMoodleWatcher(moodlePath, "4.3")
	w.Start()
	w.Stop()
	if w.Running() {
		t.Error("expected Running()=false after Stop()")
	}
	// A second Stop must not panic.
	w.Stop()
}

// TestRegeneratePlugin_AfterStopDoesNotPanic verifies that regeneratePlugin is a harmless no-op
// when it runs after Stop. A debounce timer that already fired can still call it once Stop has
// cleared the watcher state.
func TestRegeneratePlugin_AfterStopDoesNotPanic(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	pluginDir := markPluginDev(t, moodlePath, filepath.Join("local", "demo"), "local_demo")

	w := NewMoodleWatcher(moodlePath, "4.3")
	w.Start()
	w.Stop() // clears the watcher state

	// This call must not panic on the cleared state.
	w.regeneratePlugin(pluginDir, filepath.Join(pluginDir, "version.php"))
}

// TestLogging_NeverWritesToStdout verifies that starting and stopping the watcher writes nothing to
// stdout, which carries the MCP protocol in stdio mode.
func TestLogging_NeverWritesToStdout(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	markPluginDev(t, moodlePath, filepath.Join("local", "demo"), "local_demo")

	r, wPipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = wPipe
	defer func() { os.Stdout = origStdout }()

	w := NewMoodleWatcher(moodlePath, "4.3")
	w.Start()
	w.Stop()

	wPipe.Close()
	os.Stdout = origStdout

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	if n != 0 {
		t.Errorf("expected zero bytes written to stdout, got %q", buf[:n])
	}
}

// TestWatch_SurvivesAtomicRenameSaves verifies that repeated saves done through a temporary file
// and a rename each trigger a regeneration.
func TestWatch_SurvivesAtomicRenameSaves(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	pluginDir := markPluginDev(t, moodlePath, filepath.Join("local", "demo"), "local_demo")

	w := NewMoodleWatcher(moodlePath, "4.3")
	defer w.Stop()
	if count := w.Start(); count == 0 {
		t.Fatal("expected at least one watched file")
	}
	events := make(chan WatchEvent, 10)
	w.OnChange(func(e WatchEvent) { events <- e })

	versionPhp := filepath.Join(pluginDir, "version.php")
	for i := 0; i < 3; i++ {
		tmp := filepath.Join(pluginDir, "version.php.tmp")
		mustWriteFile(t, tmp, "<?php\n$plugin->component = 'local_demo';\n$plugin->version = 2024010100;\n")
		if err := os.Rename(tmp, versionPhp); err != nil {
			t.Fatalf("rename: %v", err)
		}
		if waitForCallback(t, events, 3*time.Second) == nil {
			t.Fatalf("save %d via rename did not trigger a regeneration", i+1)
		}
	}
}

// TestRegeneratePlugin_ChangeDuringRunIsReplayed verifies that a change arriving while a
// regeneration of the same plugin is in flight causes exactly one follow-up regeneration.
func TestRegeneratePlugin_ChangeDuringRunIsReplayed(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	pluginDir := markPluginDev(t, moodlePath, filepath.Join("local", "demo"), "local_demo")

	w := NewMoodleWatcher(moodlePath, "4.3")
	defer w.Stop()
	if count := w.Start(); count == 0 {
		t.Fatal("expected at least one watched file")
	}
	events := make(chan WatchEvent, 10)
	release := make(chan struct{})
	w.OnChange(func(e WatchEvent) {
		events <- e
		if e.File == "first" {
			<-release
		}
	})

	done := make(chan struct{})
	go func() {
		w.regeneratePlugin(pluginDir, "first")
		close(done)
	}()
	if waitForCallback(t, events, 3*time.Second) == nil {
		t.Fatal("first regeneration did not complete")
	}
	w.regeneratePlugin(pluginDir, "second") // arrives while the first is still in its callback
	close(release)

	second := waitForCallback(t, events, 3*time.Second)
	if second == nil || second.File != "second" {
		t.Fatalf("expected a follow-up regeneration for the change made during the run, got %+v", second)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("regeneratePlugin did not return")
	}
	select {
	case extra := <-events:
		t.Errorf("unexpected extra regeneration: %+v", extra)
	case <-time.After(300 * time.Millisecond):
	}
}
