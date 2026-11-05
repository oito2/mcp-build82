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

package extractors

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestReadFileCapped_OversizedFileReturnsErrorWithoutReading verifies that a file larger than
// MaxReadableFileSize is rejected from its stat size, before its content is read.
func TestReadFileCapped_OversizedFileReturnsErrorWithoutReading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.php")
	// A sparse file avoids writing more than 8 MiB; os.Stat still reports its truncated size.
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxReadableFileSize + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := readFileCapped(path); err == nil {
		t.Error("expected an error for a file larger than MaxReadableFileSize")
	}
}

// TestReadFileCapped_NormalFileReadsSuccessfully verifies that a file within the limit is read.
func TestReadFileCapped_NormalFileReadsSuccessfully(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "normal.php")
	want := []byte("<?php\n$a = 1;\n")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := readFileCapped(path)
	if err != nil {
		t.Fatalf("readFileCapped: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestReadFileCapped_AtSizeLimitStillReads verifies that a file of exactly the limit is read.
func TestReadFileCapped_AtSizeLimitStillReads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "atlimit.php")
	content := make([]byte, MaxReadableFileSize)
	for i := range content {
		content[i] = ' '
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := readFileCapped(path); err != nil {
		t.Errorf("expected a file at exactly MaxReadableFileSize to still read, got error: %v", err)
	}
}

// TestReadFileCapped_NonexistentReturnsError verifies that a missing file yields an error.
func TestReadFileCapped_NonexistentReturnsError(t *testing.T) {
	if _, err := readFileCapped(filepath.Join(t.TempDir(), "nonexistent.php")); err == nil {
		t.Error("expected an error for a nonexistent file")
	}
}

// TestLineIndex_Line verifies the 1-based line numbers of offsets at the start of the text, on a
// line break, right after one, and past the last one, in any query order.
func TestLineIndex_Line(t *testing.T) {
	s := "a\nbc\n\nd"
	idx := newLineIndex(s)
	for _, tc := range []struct{ pos, want int }{
		{6, 4}, {0, 1}, {1, 1}, {2, 2}, {4, 2}, {5, 3}, {3, 2},
	} {
		if got := idx.line(tc.pos); got != tc.want {
			t.Errorf("line(%d) = %d, want %d", tc.pos, got, tc.want)
		}
	}
}

// TestScanFileForOwnGetStringCalls_ManyCallsInLargeFile verifies that the line of every match in a
// multi-megabyte file with a hundred thousand matches is found quickly; counting line breaks from
// the start of the file for each match takes seconds.
func TestScanFileForOwnGetStringCalls_ManyCallsInLargeFile(t *testing.T) {
	const calls = 100000
	var b strings.Builder
	b.WriteString("<?php\n")
	for i := 0; i < calls; i++ {
		b.WriteString("$s = get_string('k', 'local_demo'); // padding padding padding\n")
	}
	path := filepath.Join(t.TempDir(), "big.php")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	got := scanFileForOwnGetStringCalls(path, "big.php", []string{"local_demo"})
	elapsed := time.Since(start)
	if len(got) != calls || got[calls-1].Line != calls+1 {
		t.Fatalf("got %d calls (last line %d), want %d (last line %d)", len(got), got[len(got)-1].Line, calls, calls+1)
	}
	if limit := 2 * time.Second * raceSlowdown; elapsed > limit {
		t.Errorf("scanning took %s, over the %s bound", elapsed, limit)
	}
}

// TestLoadPhpFiles_ScansShareOneReadPerFile verifies that the deprecated-API, capability and
// lang-string scans run over files loaded with LoadPhpFiles read each file once in total, and
// find the same calls as over unloaded files.
func TestLoadPhpFiles_ScansShareOneReadPerFile(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "lib.php"), "<?php\nold_fn();\nhas_capability('local/demo:view', $ctx);\nget_string('k', 'local_demo');\n")
	mustMkdirAll(t, filepath.Join(dir, "classes"))
	mustWriteFile(t, filepath.Join(dir, "classes", "x.php"), "<?php\nget_string('other', 'local_demo');\n")

	reads := map[string]int{}
	orig := readSource
	readSource = func(path string) ([]byte, error) {
		reads[path]++
		return orig(path)
	}
	t.Cleanup(func() { readSource = orig })

	deprecated := map[string]struct{}{"old_fn": {}}
	scan := func(files []PhpFile) (int, int, int) {
		return len(FindDeprecatedApiUsageIn(files, deprecated)),
			len(FindOwnCapabilityChecksIn(files, "local/demo")),
			len(FindOwnGetStringCallsIn(files, "local_demo"))
	}
	listed := ListPhpFiles(dir)
	files := LoadPhpFiles(listed)
	d, c, g := scan(files)
	if d != 1 || c != 1 || g != 2 {
		t.Errorf("calls found = %d deprecated, %d capability, %d get_string; want 1, 1, 2", d, c, g)
	}
	for path, n := range reads {
		if n != 1 {
			t.Errorf("%s read %d times, want 1", path, n)
		}
	}
	if len(reads) != 2 {
		t.Errorf("read %d files, want 2", len(reads))
	}
	if d2, c2, g2 := scan(listed); d2 != d || c2 != c || g2 != g {
		t.Errorf("unloaded files give %d, %d, %d calls; want %d, %d, %d", d2, c2, g2, d, c, g)
	}
}
