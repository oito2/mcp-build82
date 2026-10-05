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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/oito2/mcp-build82/internal/cache"
	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/genutil"
)

const (
	debounce          = 500 * time.Millisecond
	maxWatchedPlugins = 20
)

// WatchEvent describes one completed regeneration triggered by a file change.
type WatchEvent struct {
	PluginPath, Component, File, Timestamp string
}

// WatchCallback is invoked with the details of each completed regeneration.
type WatchCallback func(WatchEvent)

// MoodleWatcher watches every .indevelopment-marked plugin's source files and regenerates that
// plugin's context on change, debounced per plugin directory.
type MoodleWatcher struct {
	moodlePath, moodleVersion string

	mu           sync.Mutex
	fsWatcher    *fsnotify.Watcher
	watchedPaths map[string]string // absolute file path -> owning plugin dir
	timers       map[string]*time.Timer
	regenerating map[string]bool
	callbacks    []WatchCallback
	running      bool
	watchedCount int // count returned by the Start() call currently in effect; reused by a no-op re-Start()
}

// NewMoodleWatcher creates a MoodleWatcher for the given Moodle installation path and version.
// Call Start to begin watching.
func NewMoodleWatcher(moodlePath, moodleVersion string) *MoodleWatcher {
	return &MoodleWatcher{moodlePath: moodlePath, moodleVersion: moodleVersion}
}

func (w *MoodleWatcher) logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[build82 watcher] "+format+"\n", args...)
}

// OnChange registers a callback fired after every completed regeneration.
func (w *MoodleWatcher) OnChange(cb WatchCallback) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callbacks = append(w.callbacks, cb)
}

// Running reports whether the watcher is actively watching at least one file.
func (w *MoodleWatcher) Running() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

// findMarkersDelayHook, when non-nil, is invoked once at the very start of findMarkers. It exists
// solely so tests can simulate a slow filepath.WalkDir — the kind a real Moodle installation's tens
// of thousands of files would cause — without actually creating that many files on disk. Production
// code never sets it.
var findMarkersDelayHook func()

func findMarkers(moodlePath string) []string {
	if findMarkersDelayHook != nil {
		findMarkersDelayHook()
	}
	var matches []string
	_ = filepath.WalkDir(moodlePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != moodlePath && (name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == ".indevelopment" && filepath.Base(filepath.Dir(path)) == genutil.ContextDir {
			matches = append(matches, path)
		}
		return nil
	})
	return matches
}

// Start begins watching every dev-marked plugin's source files and returns the count of files
// actually watched. Zero markers found is a valid, common state (returns 0, not an error). Note
// running ends up count > 0, not len(markers) > 0 — a plugin can be marked .indevelopment yet have
// none of cache.PluginSourceFileNames present.
//
// Start is a no-op if the watcher is already running: calling it twice without an intervening
// Stop() never creates a second fsnotify.Watcher, so the first one's goroutine, file descriptors,
// and inotify watches are never orphaned. A no-op call returns the count from the Start() currently
// in effect.
//
// findMarkers (a filepath.WalkDir over the whole Moodle tree) and the fsnotify.Watcher setup that
// follows it run WITHOUT holding w.mu: on a real Moodle installation (tens of thousands of files)
// the walk alone can take seconds, and holding the lock for that whole time would block any
// concurrent Running()/OnChange()/Stop() call on this same instance for just as long. w.mu
// is only held (a) briefly at the top, to read w.running, and (b) again at the very end, to commit
// the freshly-built state.
//
// Because two Start() calls can race between (a) and (b) — both observing w.running == false and
// both performing their own walk concurrently — the commit at the end re-checks w.running before
// writing anything: if another Start() already won the race and is running, this call discards its
// own redundant fsnotify.Watcher instead of replacing a live watcher. A second Start() call that
// arrives once the first has already committed is caught by the first w.running check and never
// performs a walk at all.
func (w *MoodleWatcher) Start() int {
	w.mu.Lock()
	if w.running {
		count := w.watchedCount
		w.mu.Unlock()
		return count
	}
	w.mu.Unlock()

	markers := findMarkers(w.moodlePath)
	if len(markers) == 0 {
		w.logf("no dev plugins found (no .indevelopment markers) — nothing to watch")
		return 0
	}
	sort.Strings(markers)

	if len(markers) > maxWatchedPlugins {
		w.logf("warning: %d dev plugins found, only watching the first %d (cap) — remove .indevelopment from inactive plugins to watch different ones", len(markers), maxWatchedPlugins)
	}
	limited := markers
	if len(limited) > maxWatchedPlugins {
		limited = limited[:maxWatchedPlugins]
	}

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		w.logf("failed to create watcher: %v", err)
		return 0
	}

	watchedPaths := map[string]string{}
	count := 0
	for _, marker := range limited {
		pluginDir := filepath.Dir(filepath.Dir(marker)) // grandparent: out of .build82/, to the plugin root
		for _, f := range cache.PluginSourceFileNames {
			path := filepath.Join(pluginDir, f)
			if _, statErr := os.Stat(path); statErr != nil {
				continue
			}
			if addErr := fsWatcher.Add(path); addErr != nil {
				w.logf("failed to watch %s: %v", path, addErr)
				continue
			}
			watchedPaths[path] = pluginDir
			count++
		}
	}

	w.mu.Lock()
	if w.running {
		// Another concurrent Start() already committed a live watcher while this call was busy
		// walking the tree. Drop our own (unused) watcher instead of clobbering the running one.
		w.mu.Unlock()
		_ = fsWatcher.Close()
		return w.watchedCount
	}

	w.fsWatcher = fsWatcher
	w.watchedPaths = watchedPaths
	w.timers = map[string]*time.Timer{}
	w.regenerating = map[string]bool{}
	w.running = count > 0
	w.watchedCount = count
	w.mu.Unlock()

	if count == 0 {
		w.logf("dev plugins found but nothing watchable (none of the watched files are present)")
		return 0
	}

	w.logf("watching %d file(s) across %d plugin(s)", count, len(limited))
	go w.consumeEvents(fsWatcher)
	return count
}

func (w *MoodleWatcher) consumeEvents(fsWatcher *fsnotify.Watcher) {
	for {
		select {
		case event, ok := <-fsWatcher.Events:
			if !ok {
				return
			}
			w.onFileChange(event)
		case err, ok := <-fsWatcher.Errors:
			if !ok {
				return
			}
			w.logf("watcher error: %v", err)
		}
	}
}

func (w *MoodleWatcher) onFileChange(event fsnotify.Event) {
	w.mu.Lock()
	pluginDir, ok := w.watchedPaths[event.Name]
	if !ok {
		w.mu.Unlock()
		return
	}
	if timer, exists := w.timers[pluginDir]; exists {
		timer.Stop()
	}
	changedFile := event.Name
	w.timers[pluginDir] = time.AfterFunc(debounce, func() {
		w.regeneratePlugin(pluginDir, changedFile)
	})
	w.mu.Unlock()
}

// regeneratePlugin invalidates the plugin's cached context and regenerates it. A regeneration
// already in progress for this plugin dir causes this call to be dropped, not queued — the next
// file-change event after the in-flight one completes triggers a fresh regeneration reflecting
// everything changed in the meantime, so no information is permanently lost, just coalesced.
func (w *MoodleWatcher) regeneratePlugin(pluginDir, changedFile string) {
	w.mu.Lock()
	if w.regenerating == nil { // Stop() ran between this timer firing and acquiring the lock
		w.mu.Unlock()
		return
	}
	if w.regenerating[pluginDir] {
		w.mu.Unlock()
		w.logf("regeneration already in progress for %s — skipping", pluginDir)
		return
	}
	w.regenerating[pluginDir] = true
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		if w.regenerating != nil { // Stop() may have run while this regeneration was in flight
			w.regenerating[pluginDir] = false
		}
		w.mu.Unlock()
	}()

	for _, f := range generators.PluginContextFiles {
		cache.Global.Invalidate(generators.PluginOutputPath(pluginDir, f))
	}

	result := generators.GenerateAllForPlugin(pluginDir, w.moodlePath, true, nil)
	generators.GenerateAiIndex(w.moodlePath, w.moodleVersion)

	ok, fail := 0, 0
	for _, r := range result.Files {
		if r.Success {
			ok++
		} else {
			fail++
		}
	}
	w.logf("regenerated %s: %d ok, %d failed (triggered by %s)", result.Plugin, ok, fail, changedFile)

	evt := WatchEvent{
		PluginPath: pluginDir,
		Component:  result.Plugin,
		File:       changedFile,
		Timestamp:  genutil.Timestamp(),
	}

	w.mu.Lock()
	callbacks := append([]WatchCallback{}, w.callbacks...)
	w.mu.Unlock()
	for _, cb := range callbacks {
		w.invokeCallback(cb, evt)
	}
}

func (w *MoodleWatcher) invokeCallback(cb WatchCallback, evt WatchEvent) {
	defer func() {
		if r := recover(); r != nil {
			w.logf("a watch callback panicked (ignored): %v", r)
		}
	}()
	cb(evt)
}

// Stop closes the shared fsnotify.Watcher, cancels all pending debounce timers, clears internal
// state, and sets running to false.
func (w *MoodleWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.fsWatcher != nil {
		_ = w.fsWatcher.Close()
		w.fsWatcher = nil
	}
	for _, t := range w.timers {
		t.Stop()
	}
	w.watchedPaths = nil
	w.timers = nil
	w.regenerating = nil
	w.running = false
	w.watchedCount = 0
	w.logf("stopped")
}
