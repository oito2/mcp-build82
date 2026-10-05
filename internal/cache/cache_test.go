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

package cache

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func touch(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func TestIsStale_MissingOutput(t *testing.T) {
	c := NewMtimeCache()
	dir := t.TempDir()
	output := filepath.Join(dir, "out.md")

	if !c.IsStale(output, nil) {
		t.Error("expected stale=true for a missing output file")
	}
	if c.Stats().Misses != 1 {
		t.Errorf("expected 1 miss, got %+v", c.Stats())
	}
}

func TestIsStale_SkipsWhenSourcesOlder(t *testing.T) {
	c := NewMtimeCache()
	dir := t.TempDir()
	now := time.Now()

	src := filepath.Join(dir, "src.php")
	touch(t, src, now.Add(-1*time.Hour))
	output := filepath.Join(dir, "out.md")
	touch(t, output, now)

	if c.IsStale(output, []string{src}) {
		t.Error("expected stale=false when all sources are older than the output")
	}
	if c.Stats().Skips != 1 {
		t.Errorf("expected 1 skip, got %+v", c.Stats())
	}
}

func TestIsStale_MissWhenSourceNewer(t *testing.T) {
	c := NewMtimeCache()
	dir := t.TempDir()
	now := time.Now()

	output := filepath.Join(dir, "out.md")
	touch(t, output, now)
	src := filepath.Join(dir, "src.php")
	touch(t, src, now.Add(1*time.Hour))

	if !c.IsStale(output, []string{src}) {
		t.Error("expected stale=true when a source is newer than the output")
	}
	if c.Stats().Misses != 1 {
		t.Errorf("expected 1 miss, got %+v", c.Stats())
	}
}

func TestIsStale_HitAfterMark(t *testing.T) {
	c := NewMtimeCache()
	dir := t.TempDir()
	now := time.Now()

	output := filepath.Join(dir, "out.md")
	touch(t, output, now)
	src := filepath.Join(dir, "src.php")
	touch(t, src, now.Add(-1*time.Hour)) // older than the mark we're about to set

	c.Mark(output)

	if c.IsStale(output, []string{src}) {
		t.Error("expected stale=false (hit) when no source changed after the mark")
	}
	if c.Stats().Hits != 1 {
		t.Errorf("expected 1 hit, got %+v", c.Stats())
	}
}

func TestIsStale_MarkInvalidatedBySourceChangeAfterMark(t *testing.T) {
	c := NewMtimeCache()
	dir := t.TempDir()
	now := time.Now()

	output := filepath.Join(dir, "out.md")
	touch(t, output, now)
	src := filepath.Join(dir, "src.php")
	touch(t, src, now.Add(-1*time.Hour))

	c.Mark(output)

	// Source changes *after* the mark — must invalidate the mark and fall through
	// to comparing against the output's own mtime (branch 3), not just report stale
	// directly off the mark check.
	touch(t, src, now.Add(1*time.Hour))

	if !c.IsStale(output, []string{src}) {
		t.Error("expected stale=true after a source changed following the mark")
	}
}

func TestMarkInvalidateAndStats(t *testing.T) {
	c := NewMtimeCache()
	dir := t.TempDir()
	output := filepath.Join(dir, "out.md")
	touch(t, output, time.Now())

	c.Mark(output)
	c.Invalidate(output)
	// Invalidated: stale even though the output exists and there are no newer sources.
	if !c.IsStale(output, nil) {
		t.Error("expected stale=true after Invalidate, regardless of mtimes")
	}
	c.Mark(output)
	if c.IsStale(output, nil) {
		t.Error("expected Mark to clear the pending invalidation")
	}

	c.InvalidateAll()
	if !c.IsStale(output, nil) {
		t.Error("expected InvalidateAll to force every recorded output stale (behavior should match Invalidate)")
	}
	c.Mark(output)
	if c.IsStale(output, nil) {
		t.Error("expected Mark to clear the invalidation left by InvalidateAll")
	}

	stats := c.Stats()
	if stats.Misses != 2 || stats.Hits != 2 {
		t.Errorf("expected 2 misses and 2 hits, got %+v", stats)
	}
}

// TestInvalidate_ForcesStaleDespiteOlderSources uses an output strictly newer than its source
// and no recorded mark, the exact state a plain mtime comparison reports as fresh: an explicit
// invalidation must still win until the output is regenerated.
func TestInvalidate_ForcesStaleDespiteOlderSources(t *testing.T) {
	c := NewMtimeCache()
	dir := t.TempDir()
	src := filepath.Join(dir, "version.php")
	output := filepath.Join(dir, "out.md")
	now := time.Now()
	touch(t, src, now.Add(-2*time.Hour))
	touch(t, output, now.Add(-1*time.Hour))

	if c.IsStale(output, []string{src}) {
		t.Fatal("premise: an output newer than its source must be fresh before any invalidation")
	}

	c.Invalidate(output)
	for i := 0; i < 2; i++ {
		if !c.IsStale(output, []string{src}) {
			t.Fatalf("check %d: expected an invalidated output to stay stale until regenerated", i+1)
		}
	}

	c.Mark(output)
	if c.IsStale(output, []string{src}) {
		t.Error("expected the output to be fresh again once regenerated and marked")
	}
}

// TestInvalidate_SurvivesEnsureLoaded invalidates an output before the cache is bound to its
// root, then loads a persisted cache file that records the output as fresh. The load must not
// cancel the invalidation.
func TestInvalidate_SurvivesEnsureLoaded(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, ContextDirName, "out.md")
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		t.Fatal(err)
	}
	touch(t, output, time.Now())

	writer := NewMtimeCache()
	writer.EnsureLoaded(dir)
	writer.Mark(output)
	if err := writer.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	c := NewMtimeCache()
	c.Invalidate(output)
	c.EnsureLoaded(dir)
	if !c.IsStale(output, nil) {
		t.Error("expected an invalidation issued before EnsureLoaded to survive loading persisted marks")
	}

	// And the opposite order: invalidating after loading must not be undone by a reload either.
	other := NewMtimeCache()
	other.EnsureLoaded(dir)
	other.Invalidate(output)
	other.EnsureLoaded(t.TempDir())
	other.EnsureLoaded(dir)
	if !other.IsStale(output, nil) {
		t.Error("expected an invalidation to survive switching roots and reloading")
	}
}

func TestEnsureLoaded_IdempotentOnSameRoot(t *testing.T) {
	c := NewMtimeCache()
	dir := t.TempDir()
	output := filepath.Join(dir, "out.md")
	touch(t, output, time.Now())

	c.EnsureLoaded(dir)
	c.Mark(output)      // in-memory only, not yet saved
	c.EnsureLoaded(dir) // same root again — must be a no-op, must not discard the unsaved mark

	if c.IsStale(output, nil) {
		t.Error("EnsureLoaded on the same root discarded an unsaved in-memory mark")
	}
}

func TestEnsureLoaded_DifferentRootResetsStats(t *testing.T) {
	c := NewMtimeCache()
	dirA := t.TempDir()
	dirB := t.TempDir()

	c.EnsureLoaded(dirA)
	c.IsStale(filepath.Join(dirA, "missing.md"), nil) // generates a Miss

	if c.Stats().Misses == 0 {
		t.Fatal("expected at least one miss before switching roots")
	}

	c.EnsureLoaded(dirB)
	if c.Stats() != (CacheStats{}) {
		t.Errorf("expected stats reset to zero after binding a different root, got %+v", c.Stats())
	}
}

func TestEnsureLoaded_MissingFileDegradesToEmpty(t *testing.T) {
	c := NewMtimeCache()
	dir := t.TempDir() // no .build82/.cache.json exists here at all

	c.EnsureLoaded(dir)
	output := filepath.Join(dir, "out.md")
	touch(t, output, time.Now())

	if c.IsStale(output, nil) {
		t.Error("expected no pre-existing mark from a missing cache file")
	}
}

func TestEnsureLoaded_CorruptFileDegradesToEmpty(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, ContextDirName)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, ".cache.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewMtimeCache()
	c.EnsureLoaded(dir) // must not panic or error

	output := filepath.Join(dir, "out.md")
	touch(t, output, time.Now())
	if c.IsStale(output, nil) {
		t.Error("expected a corrupt cache file to degrade to an empty cache, not a stale hit")
	}
}

func TestEnsureLoaded_VersionMismatchDegradesToEmpty(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, ContextDirName)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	future := cacheFile{Version: cacheFileVersion + 1, Entries: map[string]time.Time{
		filepath.Join(dir, "out.md"): time.Now(),
	}}
	b, _ := json.Marshal(future)
	if err := os.WriteFile(filepath.Join(cacheDir, ".cache.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewMtimeCache()
	c.EnsureLoaded(dir)

	output := filepath.Join(dir, "out.md")
	touch(t, output, time.Now())
	if c.IsStale(output, nil) {
		t.Error("expected a version-mismatched cache file to degrade to an empty cache")
	}
}

// TestEnsureLoaded_SavesPreviousRootBeforeSwitching verifies that EnsureLoaded persists unsaved
// marks to the previous root's own .cache.json before switching to a different moodlePath, so an
// unsaved Mark() on root A is not lost when another goroutine calls EnsureLoaded(B).
func TestEnsureLoaded_SavesPreviousRootBeforeSwitching(t *testing.T) {
	c := NewMtimeCache()
	dirA := t.TempDir()
	dirB := t.TempDir()
	outputA := filepath.Join(dirA, "out.md")
	touch(t, outputA, time.Now())

	c.EnsureLoaded(dirA)
	c.Mark(outputA) // dirty, unsaved — never called c.Save() for root A

	if _, err := os.Stat(cachePath(dirA)); err == nil {
		t.Fatal("test setup invariant violated: root A's cache file must not exist before the switch")
	}

	// Switching roots must flush root A's pending mark to disk first.
	c.EnsureLoaded(dirB)

	if _, err := os.Stat(cachePath(dirA)); err != nil {
		t.Fatalf("expected root A's mark to be saved to %s before switching roots, got: %v", cachePath(dirA), err)
	}

	// Simulate a fresh process reloading root A afterwards — the mark must have survived.
	c2 := NewMtimeCache()
	c2.EnsureLoaded(dirA)
	if c2.IsStale(outputA, nil) {
		t.Error("expected root A's mark, saved during the switch, to survive a reload as a hit")
	}
}

// TestEnsureLoaded_ReadErrorWarnsOnStderr verifies that a genuine read failure on the cache file
// (as opposed to it not existing) makes EnsureLoaded print a warning to stderr, while still
// degrading to an empty, working cache instead of failing.
//
// A directory is created at the cache file's path (instead of a regular file) to force os.ReadFile
// to fail with a real error distinct from "not exist" (errors.Is(err, os.ErrNotExist) is false for
// "is a directory"), without relying on permission bits that behave inconsistently when tests run as
// root.
func TestEnsureLoaded_ReadErrorWarnsOnStderr(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, ContextDirName)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The cache file's own path is a directory, not a file — os.ReadFile must fail with EISDIR, not
	// ENOENT.
	if err := os.MkdirAll(filepath.Join(cacheDir, ".cache.json"), 0o755); err != nil {
		t.Fatal(err)
	}

	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("failed to create pipe: %v", pipeErr)
	}
	origStderr := os.Stderr
	os.Stderr = w

	c := NewMtimeCache()
	c.EnsureLoaded(dir) // must not panic despite the real read error

	os.Stderr = origStderr
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}

	captured := buf.String()
	if !strings.Contains(captured, "could not read cache file") {
		t.Errorf("expected a stderr warning about the unreadable cache file, got: %q", captured)
	}

	// Confirm the degrade-gracefully contract still holds: no fatal error, cache usable as empty.
	output := filepath.Join(dir, "out.md")
	touch(t, output, time.Now())
	if c.IsStale(output, nil) {
		t.Error("expected the cache to still degrade to an empty, working cache after the read error")
	}
}

func TestSave_NoopWhenNotDirty(t *testing.T) {
	c := NewMtimeCache()
	dir := t.TempDir()
	c.EnsureLoaded(dir)

	if err := c.Save(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(cachePath(dir)); err == nil {
		t.Error("expected no cache file to be written when nothing was marked dirty")
	}
}

func TestSave_WritesAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "out.md")
	touch(t, output, time.Now())

	c1 := NewMtimeCache()
	c1.EnsureLoaded(dir)
	c1.Mark(output)
	if err := c1.Save(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(cachePath(dir)); err != nil {
		t.Fatalf("expected cache file to exist after Save: %v", err)
	}

	// Simulate a process restart: a fresh MtimeCache, same root.
	src := filepath.Join(dir, "src.php")
	touch(t, src, time.Now().Add(-1*time.Hour)) // older than the persisted mark

	c2 := NewMtimeCache()
	c2.EnsureLoaded(dir)
	if c2.IsStale(output, []string{src}) {
		t.Error("expected the persisted mark to survive a simulated restart as a hit")
	}
}
