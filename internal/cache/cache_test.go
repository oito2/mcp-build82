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

// touch writes a small file at `path` and sets its modification time to `mtime`, failing the test on
// error.
func touch(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

// TestIsStale_MissingOutput verifies that a missing output file is stale and counted as a miss.
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

// TestIsStale_SkipsWhenSourcesOlder verifies that an output newer than all sources is fresh and
// counted as a skip.
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

// TestIsStale_MissWhenSourceNewer verifies that a source newer than the output makes it stale.
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

// TestIsStale_HitAfterMark verifies that a marked output with no later source change is a hit.
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

// TestIsStale_MarkInvalidatedBySourceChangeAfterMark verifies that a source modified after the mark
// drops the mark and falls back to the output mtime comparison.
func TestIsStale_MarkInvalidatedBySourceChangeAfterMark(t *testing.T) {
	c := NewMtimeCache()
	dir := t.TempDir()
	now := time.Now()

	output := filepath.Join(dir, "out.md")
	touch(t, output, now)
	src := filepath.Join(dir, "src.php")
	touch(t, src, now.Add(-1*time.Hour))

	c.Mark(output)

	// A source changed after the mark drops the mark, and the output mtime comparison then
	// decides.
	touch(t, src, now.Add(1*time.Hour))

	if !c.IsStale(output, []string{src}) {
		t.Error("expected stale=true after a source changed following the mark")
	}
}

// TestMarkInvalidateAndStats verifies Invalidate, InvalidateAll and Mark interplay, and the resulting
// hit and miss counters.
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

// TestInvalidate_ForcesStaleDespiteOlderSources verifies that an explicit invalidation keeps an
// output stale, although it is newer than its source and has no mark (which a plain mtime
// comparison reports as fresh), until the output is marked again.
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

// TestInvalidate_SurvivesEnsureLoaded verifies that an invalidation made before the cache is bound
// to a root is not cancelled by loading a persisted file that marks the output as fresh, and that
// an invalidation also survives switching roots and reloading.
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

	// Invalidating after loading must not be undone by a reload either.
	other := NewMtimeCache()
	other.EnsureLoaded(dir)
	other.Invalidate(output)
	other.EnsureLoaded(t.TempDir())
	other.EnsureLoaded(dir)
	if !other.IsStale(output, nil) {
		t.Error("expected an invalidation to survive switching roots and reloading")
	}
}

// TestEnsureLoaded_IdempotentOnSameRoot verifies that EnsureLoaded on the bound root keeps unsaved marks.
func TestEnsureLoaded_IdempotentOnSameRoot(t *testing.T) {
	c := NewMtimeCache()
	dir := t.TempDir()
	output := filepath.Join(dir, "out.md")
	touch(t, output, time.Now())

	c.EnsureLoaded(dir)
	c.Mark(output)      // not saved yet
	c.EnsureLoaded(dir) // same root: must keep the unsaved mark

	if c.IsStale(output, nil) {
		t.Error("EnsureLoaded on the same root discarded an unsaved in-memory mark")
	}
}

// TestEnsureLoaded_DifferentRootResetsStats verifies that binding a different root resets the statistics.
func TestEnsureLoaded_DifferentRootResetsStats(t *testing.T) {
	c := NewMtimeCache()
	dirA := t.TempDir()
	dirB := t.TempDir()

	c.EnsureLoaded(dirA)
	c.IsStale(filepath.Join(dirA, "missing.md"), nil) // records a miss

	if c.Stats().Misses == 0 {
		t.Fatal("expected at least one miss before switching roots")
	}

	c.EnsureLoaded(dirB)
	if c.Stats() != (CacheStats{}) {
		t.Errorf("expected stats reset to zero after binding a different root, got %+v", c.Stats())
	}
}

// TestEnsureLoaded_MissingFileDegradesToEmpty verifies that a missing cache file yields an empty cache.
func TestEnsureLoaded_MissingFileDegradesToEmpty(t *testing.T) {
	c := NewMtimeCache()
	dir := t.TempDir() // contains no .build82/.cache.json

	c.EnsureLoaded(dir)
	output := filepath.Join(dir, "out.md")
	touch(t, output, time.Now())

	if c.IsStale(output, nil) {
		t.Error("expected no pre-existing mark from a missing cache file")
	}
}

// TestEnsureLoaded_CorruptFileDegradesToEmpty verifies that an invalid JSON cache file yields an
// empty cache without failing.
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
	c.EnsureLoaded(dir) // must not panic

	output := filepath.Join(dir, "out.md")
	touch(t, output, time.Now())
	if c.IsStale(output, nil) {
		t.Error("expected a corrupt cache file to degrade to an empty cache, not a stale hit")
	}
}

// TestEnsureLoaded_VersionMismatchDegradesToEmpty verifies that a cache file with another format
// version is ignored.
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

// TestEnsureLoaded_SavesPreviousRootBeforeSwitching verifies that EnsureLoaded writes unsaved marks
// to the previous root's own cache file before switching to another root, so a mark on root A is
// not lost when EnsureLoaded is called for root B.
func TestEnsureLoaded_SavesPreviousRootBeforeSwitching(t *testing.T) {
	c := NewMtimeCache()
	dirA := t.TempDir()
	dirB := t.TempDir()
	outputA := filepath.Join(dirA, "out.md")
	touch(t, outputA, time.Now())

	c.EnsureLoaded(dirA)
	c.Mark(outputA) // unsaved: Save is never called for root A

	if _, err := os.Stat(cachePath(dirA)); err == nil {
		t.Fatal("test setup invariant violated: root A's cache file must not exist before the switch")
	}

	// Switching roots must write root A's pending mark to disk first.
	c.EnsureLoaded(dirB)

	if _, err := os.Stat(cachePath(dirA)); err != nil {
		t.Fatalf("expected root A's mark to be saved to %s before switching roots, got: %v", cachePath(dirA), err)
	}

	// A new cache loading root A must see the saved mark.
	c2 := NewMtimeCache()
	c2.EnsureLoaded(dirA)
	if c2.IsStale(outputA, nil) {
		t.Error("expected root A's mark, saved during the switch, to survive a reload as a hit")
	}
}

// TestEnsureLoaded_ReadErrorWarnsOnStderr verifies that a read failure other than a missing file
// makes EnsureLoaded print a warning to stderr while still yielding a usable empty cache.
//
// The failure is produced by creating a directory at the cache file's path, which avoids relying
// on permission bits that behave differently when tests run as root.
func TestEnsureLoaded_ReadErrorWarnsOnStderr(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, ContextDirName)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Reading a directory fails with an error other than "not exist".
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
	c.EnsureLoaded(dir) // must not panic on the read error

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

	// The cache must remain usable and empty.
	output := filepath.Join(dir, "out.md")
	touch(t, output, time.Now())
	if c.IsStale(output, nil) {
		t.Error("expected the cache to still degrade to an empty, working cache after the read error")
	}
}

// TestSave_NoopWhenNotDirty verifies that Save writes no file when nothing was marked.
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

// TestSave_WritesAndRoundTrips verifies that a saved mark is loaded by a new cache for the same root
// and counts as a hit.
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

	// A new cache for the same root simulates a restart.
	src := filepath.Join(dir, "src.php")
	touch(t, src, time.Now().Add(-1*time.Hour)) // older than the persisted mark

	c2 := NewMtimeCache()
	c2.EnsureLoaded(dir)
	if c2.IsStale(output, []string{src}) {
		t.Error("expected the persisted mark to survive a simulated restart as a hit")
	}
}

// TestEnsureLoaded_NullOrMissingEntriesIsEmpty verifies that a cache file with a valid version but
// a null or absent "entries" value loads as an empty cache that accepts Mark without panicking.
func TestEnsureLoaded_NullOrMissingEntriesIsEmpty(t *testing.T) {
	for name, body := range map[string]string{
		"null":    `{"version": 1, "entries": null}`,
		"missing": `{"version": 1}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := cachePath(root)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			c := NewMtimeCache()
			c.EnsureLoaded(root)
			out := filepath.Join(root, "out.md")
			touch(t, out, time.Now())
			c.Mark(out)
		})
	}
}
