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

package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oito2/mcp-build82/internal/cache"
	"github.com/oito2/mcp-build82/internal/genutil"
)

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// markPluginDev creates a plugin dir with a version.php and the .build82/.indevelopment marker.
func markPluginDev(t *testing.T, moodlePath, pluginRel, component string) string {
	t.Helper()
	pluginDir := filepath.Join(moodlePath, pluginRel)
	mustMkdirAll(t, filepath.Join(pluginDir, genutil.ContextDir))
	mustWriteFile(t, filepath.Join(pluginDir, "version.php"),
		"<?php\n$plugin->component = '"+component+"';\n$plugin->version = 2024010100;\n")
	mustWriteFile(t, filepath.Join(pluginDir, genutil.ContextDir, ".indevelopment"), "2026-07-27T00:00:00Z")
	return pluginDir
}

// freshCache swaps in a fresh MtimeCache for the duration of the test.
func freshCache(t *testing.T) {
	t.Helper()
	old := cache.Global
	cache.Global = cache.NewMtimeCache()
	t.Cleanup(func() { cache.Global = old })
}

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

func TestStart_MarkersButNothingWatchable(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	pluginDir := filepath.Join(moodlePath, "local", "bare")
	mustMkdirAll(t, filepath.Join(pluginDir, genutil.ContextDir))
	mustWriteFile(t, filepath.Join(pluginDir, genutil.ContextDir, ".indevelopment"), "ts")
	// Deliberately no version.php, lib.php, db/*.php — nothing in watchedFiles exists.

	w := NewMoodleWatcher(moodlePath, "4.3")
	if count := w.Start(); count != 0 {
		t.Errorf("expected 0 files watched (marker exists but nothing watchable), got %d", count)
	}
	if w.Running() {
		t.Error("expected Running()=false when nothing is watchable, even though a dev marker exists")
	}
	w.Stop()
}

func TestStart_WatchesExistingFiles(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	pluginDir := markPluginDev(t, moodlePath, filepath.Join("local", "demo"), "local_demo")
	mustWriteFile(t, filepath.Join(pluginDir, "lib.php"), "<?php\n")
	mustMkdirAll(t, filepath.Join(pluginDir, "db"))
	mustWriteFile(t, filepath.Join(pluginDir, "db", "events.php"), "<?php\n")

	w := NewMoodleWatcher(moodlePath, "4.3")
	defer w.Stop()

	// version.php + lib.php + db/events.php = 3 watchable files present.
	if count := w.Start(); count != 3 {
		t.Errorf("expected 3 files watched, got %d", count)
	}
	if !w.Running() {
		t.Error("expected Running()=true")
	}
}

// TestStart_SecondCallWithoutStopIsNoop verifies that a second Start() call before Stop() is a
// no-op that returns the count already in effect and does not recreate the fsnotify.Watcher or
// w.watchedPaths/w.timers/w.regenerating.
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

	// Add another watchable plugin after the first Start() — if a second Start() recreated the
	// watcher, this new file would end up being (re-)discovered; since it must be a no-op instead,
	// the count must stay identical to the first call and the underlying *fsnotify.Watcher must be
	// the exact same instance (not merely an equal one).
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

// TestStart_DoesNotBlockRunningDuringWalk verifies that Start() does not hold w.mu during the
// findMarkers walk, so a concurrent Running() call returns quickly. It uses findMarkersDelayHook to
// make findMarkers pause for a known, fixed duration, and asserts Running() returns quickly while
// that pause is in progress.
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

	// Give the Start() goroutine time to pass its initial running-check and enter the (delayed)
	// walk, without waiting so long that the delay itself has already elapsed.
	time.Sleep(150 * time.Millisecond)

	runningStart := time.Now()
	w.Running()
	elapsed := time.Since(runningStart)

	// Comfortably below the remaining walk delay (~650ms): an uncontended lock acquisition should
	// take microseconds. If Start() were still holding w.mu for the whole walk, this call would
	// instead take roughly as long as what's left of walkDelay.
	if elapsed >= walkDelay/2 {
		t.Errorf("Running() took %v while Start() was mid-walk (walk delay %v) — w.mu appears to be held during findMarkers", elapsed, walkDelay)
	}

	<-startDone // let Start() finish committing its state before Stop()/test cleanup runs
}

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
	// Each marked plugin has exactly version.php watchable -> 1 file each.
	if count != maxWatchedPlugins {
		t.Errorf("expected exactly %d files watched (cap), got %d", maxWatchedPlugins, count)
	}
}

func waitForCallback(t *testing.T, ch <-chan WatchEvent, timeout time.Duration) *WatchEvent {
	t.Helper()
	select {
	case evt := <-ch:
		return &evt
	case <-time.After(timeout):
		return nil
	}
}

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
	// Rapid repeated touches well within the 500ms debounce window.
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

	// Confirm no second regeneration follows shortly after (i.e. it really coalesced to one).
	select {
	case extra := <-events:
		t.Errorf("expected only one coalesced regeneration, got an extra one: %+v", extra)
	case <-time.After(700 * time.Millisecond):
		// good — no extra event
	}
}

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
	time.Sleep(200 * time.Millisecond) // partway into plugin A's debounce window
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
	// Must not panic on a second Stop().
	w.Stop()
}

// TestRegeneratePlugin_AfterStopDoesNotPanic reproduces the race between Stop() (which nils out
// w.regenerating) and a debounce timer's AfterFunc firing after Stop() already ran — time.Timer.Stop
// does not guarantee an already-fired function stops running, so regeneratePlugin can observe a nil
// w.regenerating map. Writing to it must not panic and must simply become a no-op.
func TestRegeneratePlugin_AfterStopDoesNotPanic(t *testing.T) {
	freshCache(t)
	moodlePath := t.TempDir()
	pluginDir := markPluginDev(t, moodlePath, filepath.Join("local", "demo"), "local_demo")

	w := NewMoodleWatcher(moodlePath, "4.3")
	w.Start()
	w.Stop() // nils w.regenerating/w.timers/w.watchedPaths

	// Simulates the debounce timer's callback firing after Stop() already cleared state — must not
	// panic on a nil-map write, and the deferred cleanup inside regeneratePlugin must also tolerate it.
	w.regeneratePlugin(pluginDir, filepath.Join(pluginDir, "version.php"))
}

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
