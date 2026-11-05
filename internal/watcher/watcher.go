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
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/oito2/mcp-build82/internal/cache"
	"github.com/oito2/mcp-build82/internal/generators"
	"github.com/oito2/mcp-build82/internal/genutil"
)

const (
	// defaultDebounce is the quiet period after a file event before regeneration starts; further
	// events for the same plugin restart it.
	defaultDebounce = 500 * time.Millisecond
	// maxWatchedPlugins caps how many dev-marked plugins are watched at once.
	maxWatchedPlugins = 20
)

// WatchEvent describes one completed regeneration triggered by a file change. PluginPath is the
// plugin root directory, Component its Moodle component name, File the changed source file that
// triggered the run, and Timestamp the completion time.
type WatchEvent struct {
	PluginPath, Component, File, Timestamp string
}

// WatchCallback is invoked with the details of each completed regeneration.
type WatchCallback func(WatchEvent)

// MoodleWatcher watches every source file of each plugin carrying an .indevelopment marker (all
// files below the plugin, as generators.PluginSourceFiles lists them) and regenerates that plugin's
// context when one changes, debounced per plugin directory. It is safe for concurrent use.
type MoodleWatcher struct {
	moodlePath, moodleVersion string
	debounce                  time.Duration // quiet period before a regeneration; set at construction

	mu           sync.Mutex
	fsWatcher    *fsnotify.Watcher
	watchedDirs  map[string]string // watched directory -> owning plugin dir
	timers       map[string]*time.Timer
	regenerating map[string]bool
	pending      map[string]string // plugin dir -> last changed file that arrived during an in-flight regeneration
	callbacks    []WatchCallback
	running      bool
	watchedCount int // file count returned by the Start call in effect; reused when Start is called again while running
}

// NewMoodleWatcher returns a MoodleWatcher for the Moodle installation at `moodlePath`, whose
// version string `moodleVersion` is passed to the AI index generation. It does not watch anything
// until Start is called.
func NewMoodleWatcher(moodlePath, moodleVersion string) *MoodleWatcher {
	return &MoodleWatcher{moodlePath: moodlePath, moodleVersion: moodleVersion, debounce: defaultDebounce}
}

// logf writes a line to stderr prefixed with the watcher tag, using fmt.Fprintf formatting.
func (w *MoodleWatcher) logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[build82 watcher] "+format+"\n", args...)
}

// OnChange registers `cb` to be called, synchronously on the regeneration goroutine, after every
// completed regeneration. A panic in a callback is recovered and logged.
func (w *MoodleWatcher) OnChange(cb WatchCallback) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callbacks = append(w.callbacks, cb)
}

// Running reports whether the watcher was started and is watching at least one file.
func (w *MoodleWatcher) Running() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

// findMarkersDelayHook, when non-nil, is called once at the start of findMarkers. Tests use it to
// simulate a slow directory walk; production code leaves it nil.
var findMarkersDelayHook func()

// newFSWatcher creates the fsnotify watcher used by Start. Tests replace it to observe the watcher
// Start creates.
var newFSWatcher = fsnotify.NewWatcher

// findDevPluginDirs returns, sorted, the root directories of the plugins under `moodlePath` marked
// .indevelopment, as generators.FindDevPlugins finds them (the same walk every other tool uses).
func findDevPluginDirs(moodlePath string) []string {
	if findMarkersDelayHook != nil {
		findMarkersDelayHook()
	}
	dirs := generators.FindDevPlugins(moodlePath)
	sort.Strings(dirs)
	return dirs
}

// Start finds the plugins marked .indevelopment under the Moodle path and begins watching all of
// their source files (at most the first maxWatchedPlugins plugins in path order). Every directory
// of a plugin is registered with fsnotify, except the ones generators.IsSkippedPluginDir leaves
// out (vendor/, node_modules/, .git/, .build82/) at any depth, and a directory created later is
// added when its creation is seen, so a file in any of them — including one saved through a
// temporary file and a rename — triggers a regeneration. It returns the number of source files
// found when it started (generators.PluginSourceFiles). Finding no markers, or no source files, is a
// normal state and returns 0 without an error; failures to create the watcher or to watch a
// directory are logged. The watcher counts as running only when the result is greater than zero.
//
// Start does nothing when the watcher is already running and returns the count of the active
// run, so a second call never leaks a second fsnotify watcher.
//
// The directory walk and watcher setup run without holding the mutex, so Running, OnChange and
// Stop are not blocked by a slow walk. Two concurrent Start calls may both walk. The first to
// commit its state wins, and the other discards its own fsnotify watcher and returns the
// winner's count.
func (w *MoodleWatcher) Start() int {
	w.mu.Lock()
	if w.running {
		count := w.watchedCount
		w.mu.Unlock()
		return count
	}
	w.mu.Unlock()

	pluginDirs := findDevPluginDirs(w.moodlePath)
	if len(pluginDirs) == 0 {
		w.logf("no dev plugins found (no .indevelopment markers) — nothing to watch")
		return 0
	}

	if len(pluginDirs) > maxWatchedPlugins {
		w.logf("warning: %d dev plugins found, only watching the first %d (cap) — remove .indevelopment from inactive plugins to watch different ones", len(pluginDirs), maxWatchedPlugins)
	}
	limited := pluginDirs
	if len(limited) > maxWatchedPlugins {
		limited = limited[:maxWatchedPlugins]
	}

	fsWatcher, err := newFSWatcher()
	if err != nil {
		w.logf("failed to create watcher: %v", err)
		return 0
	}

	watchedDirs := map[string]string{}
	count := 0
	for _, pluginDir := range limited {
		files := len(generators.PluginSourceFiles(pluginDir))
		if files == 0 {
			continue
		}
		w.addTree(fsWatcher, watchedDirs, pluginDir, pluginDir)
		count += files
	}

	w.mu.Lock()
	if w.running {
		// Another concurrent Start() already committed a live watcher while this call was busy
		// walking the tree. Drop our own (unused) watcher instead of clobbering the running one.
		w.mu.Unlock()
		_ = fsWatcher.Close()
		return w.watchedCount
	}

	if count == 0 {
		w.mu.Unlock()
		_ = fsWatcher.Close()
		w.logf("dev plugins found but nothing watchable (no source files)")
		return 0
	}

	w.fsWatcher = fsWatcher
	w.watchedDirs = watchedDirs
	w.timers = map[string]*time.Timer{}
	w.regenerating = map[string]bool{}
	w.pending = map[string]string{}
	w.running = true
	w.watchedCount = count
	w.mu.Unlock()

	w.logf("watching %d file(s) in %d director(ies) across %d plugin(s)", count, len(watchedDirs), len(limited))
	go w.consumeEvents(fsWatcher)
	return count
}

// consumeEvents forwards events from `fsWatcher` to onFileChange and logs its errors. It returns
// when the watcher's channels are closed, which happens when the watcher is closed.
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

// addTree registers with `fsWatcher` the directory `root` and every directory below it that
// generators.IsSkippedPluginDir does not leave out, recording each in `dirs` as owned by the
// plugin `pluginDir`. Symbolic links are not followed. Directories that cannot be read or watched
// are logged and skipped.
func (w *MoodleWatcher) addTree(fsWatcher *fsnotify.Watcher, dirs map[string]string, root, pluginDir string) {
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if path != pluginDir && generators.IsSkippedPluginDir(d.Name()) {
			return filepath.SkipDir
		}
		if _, seen := dirs[path]; seen {
			return nil
		}
		if addErr := fsWatcher.Add(path); addErr != nil {
			w.logf("failed to watch %s: %v", path, addErr)
			return filepath.SkipDir
		}
		dirs[path] = pluginDir
		return nil
	})
}

// isBuildOutput reports whether `path`, inside the plugin `pluginDir`, is build82's own output: a
// skipped directory (.build82 being created, for instance) or a legacy context file at the plugin
// root, which generation moves into .build82/. Events for them are not source changes, so they
// must not trigger another regeneration.
func isBuildOutput(path, pluginDir string) bool {
	name := filepath.Base(path)
	if generators.IsSkippedPluginDir(name) {
		return true
	}
	if filepath.Dir(path) != pluginDir {
		return false
	}
	return name == ".indevelopment" || slices.Contains(generators.PluginContextFiles, name)
}

// onFileChange handles one filesystem event. Events outside the watched directories, events for
// build82's own outputs (isBuildOutput), and events other than write, create, rename or remove,
// are ignored. A created directory is watched too, with its subdirectories; a removed or renamed
// watched directory is forgotten. Otherwise it restarts the plugin's debounce timer, so
// regeneratePlugin runs only after the plugin's files have been quiet for the debounce period. A
// removed file is handled like an edit, so context derived from it is regenerated.
func (w *MoodleWatcher) onFileChange(event fsnotify.Event) {
	if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) && !event.Has(fsnotify.Rename) && !event.Has(fsnotify.Remove) {
		return
	}
	path := filepath.Clean(event.Name)
	w.mu.Lock()
	pluginDir, ok := w.watchedDirs[filepath.Dir(path)]
	if !ok || w.fsWatcher == nil || isBuildOutput(path, pluginDir) {
		w.mu.Unlock()
		return
	}
	switch {
	case event.Has(fsnotify.Create):
		if info, err := os.Lstat(path); err == nil && info.IsDir() {
			w.addTree(w.fsWatcher, w.watchedDirs, path, pluginDir)
		}
	case event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename):
		for dir := range w.watchedDirs {
			if dir == path || strings.HasPrefix(dir, path+string(filepath.Separator)) {
				delete(w.watchedDirs, dir)
			}
		}
	}
	if timer, exists := w.timers[pluginDir]; exists {
		timer.Stop()
	}
	changedFile := event.Name
	w.timers[pluginDir] = time.AfterFunc(w.debounce, func() {
		w.regeneratePlugin(pluginDir, changedFile)
	})
	w.mu.Unlock()
}

// regeneratePlugin invalidates the cached context files of the plugin at `pluginDir`, regenerates
// them and the AI index, then notifies the registered callbacks with an event naming
// `changedFile`. If a regeneration is already running for the plugin, the change is recorded and
// the plugin is regenerated once more after the running one finishes. If the watcher was stopped,
// the call is dropped.
func (w *MoodleWatcher) regeneratePlugin(pluginDir, changedFile string) {
	w.mu.Lock()
	if w.regenerating == nil { // Stop() ran between this timer firing and acquiring the lock
		w.mu.Unlock()
		return
	}
	if w.regenerating[pluginDir] {
		w.pending[pluginDir] = changedFile
		w.mu.Unlock()
		w.logf("regeneration already in progress for %s — queued one more run", pluginDir)
		return
	}
	w.regenerating[pluginDir] = true
	w.mu.Unlock()

	for {
		w.runRegeneration(pluginDir, changedFile)

		w.mu.Lock()
		next, again := w.pending[pluginDir]
		if w.regenerating == nil || !again { // Stop() may have run while this regeneration was in flight
			if w.regenerating != nil {
				w.regenerating[pluginDir] = false
			}
			w.mu.Unlock()
			return
		}
		delete(w.pending, pluginDir)
		w.mu.Unlock()
		changedFile = next
	}
}

// runRegeneration performs one regeneration of the plugin at `pluginDir` and notifies the
// callbacks with an event naming `changedFile`.
func (w *MoodleWatcher) runRegeneration(pluginDir, changedFile string) {
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

// invokeCallback calls `cb` with `evt`, recovering and logging any panic so one faulty callback
// cannot crash the watcher or block the others.
func (w *MoodleWatcher) invokeCallback(cb WatchCallback, evt WatchEvent) {
	defer func() {
		if r := recover(); r != nil {
			w.logf("a watch callback panicked (ignored): %v", r)
		}
	}()
	cb(evt)
}

// Stop closes the fsnotify watcher, cancels all pending debounce timers, clears the watch state
// and marks the watcher as not running. It is safe to call when the watcher is not running, and
// Start can be called again afterwards. A regeneration already in progress is not interrupted.
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
	w.watchedDirs = nil
	w.timers = nil
	w.regenerating = nil
	w.pending = nil
	w.running = false
	w.watchedCount = 0
	w.logf("stopped")
}
