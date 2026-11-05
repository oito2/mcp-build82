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

package tsbackend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// largeLibFunctions is the number of top-level functions in the synthetic lib.php, about 1.1 MB.
const largeLibFunctions = 21000

// writeLargeLibPhp writes a lib.php of about 1.1 MB into a new plugin directory: one legacy
// callback (local_demo_before_footer) followed by largeLibFunctions small functions. It
// returns the plugin directory.
func writeLargeLibPhp(tb testing.TB) string {
	tb.Helper()
	var b strings.Builder
	b.WriteString("<?php\nfunction local_demo_before_footer() {}\n")
	for i := 0; i < largeLibFunctions; i++ {
		fmt.Fprintf(&b, "function local_demo_fn%05d($a) { return $a + 1; }\n", i)
	}
	dir := tb.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lib.php"), []byte(b.String()), 0o644); err != nil {
		tb.Fatal(err)
	}
	return dir
}

// TestLargeLibPhp_ScansInLinearTime verifies that a lib.php with tens of thousands of top-level
// functions is fully extracted and scanned for legacy callbacks within a bound that a scan
// quadratic in the number of root children (tens of seconds) cannot meet.
func TestLargeLibPhp_ScansInLinearTime(t *testing.T) {
	if testing.Short() {
		t.Skip("parses a 1.1 MB file")
	}
	dir := writeLargeLibPhp(t)
	start := time.Now()
	fns := ExtractFunctionsFromPhpFile(filepath.Join(dir, "lib.php"))
	warnings := DetectLegacyCallbacks(dir, "local_demo")
	elapsed := time.Since(start)

	if len(fns) != largeLibFunctions+1 {
		t.Errorf("got %d functions, want %d", len(fns), largeLibFunctions+1)
	}
	if len(warnings) != 1 || warnings[0].LegacyFunction != "local_demo_before_footer" {
		t.Errorf("got legacy warnings %+v, want only local_demo_before_footer", warnings)
	}
	if limit := 10 * time.Second * raceSlowdown; elapsed > limit {
		t.Errorf("scanning took %s, over the %s bound", elapsed, limit)
	}
}

// BenchmarkLargeLibPhp measures parsing a 1.1 MB lib.php and running both root-level scans.
func BenchmarkLargeLibPhp(b *testing.B) {
	dir := writeLargeLibPhp(b)
	path := filepath.Join(dir, "lib.php")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ExtractFunctionsFromPhpFile(path)
		DetectLegacyCallbacks(dir, "local_demo")
	}
}

// TestParseFile_EarlyStopIsAnError verifies that a parse stopped by the timeout is reported as an
// error instead of returning a partial tree whose missing declarations would go unnoticed.
func TestParseFile_EarlyStopIsAnError(t *testing.T) {
	dir := writeLargeLibPhp(t)
	old := parseTimeout
	parseTimeout = time.Microsecond
	t.Cleanup(func() { parseTimeout = old })

	tree, _, err := ParseFile(filepath.Join(dir, "lib.php"))
	if err == nil {
		tree.Release()
		t.Fatal("expected an error for a parse that stopped early")
	}
	if !strings.Contains(err.Error(), "stopped early") {
		t.Errorf("unexpected error: %v", err)
	}
}
