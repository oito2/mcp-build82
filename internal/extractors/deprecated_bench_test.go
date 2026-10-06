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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// findDeprecatedApiUsageUncached behaves like FindDeprecatedApiUsage but compiles the whole
// alternation pattern on every call; it is the baseline for the cached variant's benchmark.
func findDeprecatedApiUsageUncached(pluginPath string, deprecated map[string]struct{}) []DeprecatedCall {
	if len(deprecated) == 0 {
		return nil
	}

	names := make([]string, 0, len(deprecated))
	for name := range deprecated {
		names = append(names, regexp.QuoteMeta(name))
	}
	sort.Strings(names)
	pattern := regexp.MustCompile(`\b(` + strings.Join(names, "|") + `)\s*\(`)

	var calls []DeprecatedCall
	walkPhpFiles(pluginPath, func(absPath, relSlash string) {
		calls = append(calls, scanFileForDeprecatedCalls(absPath, relSlash, pattern)...)
	})
	return calls
}

// benchDeprecatedNames returns a set of about 300 deprecated function names, the size at which
// recompiling the alternation pattern is most costly.
func benchDeprecatedNames() map[string]struct{} {
	deprecated := make(map[string]struct{}, 300)
	for i := 0; i < 300; i++ {
		deprecated[fmt.Sprintf("deprecated_core_function_%d", i)] = struct{}{}
	}
	deprecated["get_context_instance"] = struct{}{}
	return deprecated
}

// benchDeprecatedPluginDir creates a temporary plugin directory containing one lib.php with a bare
// and a method call of a deprecated function, and returns its path.
func benchDeprecatedPluginDir(b *testing.B) string {
	b.Helper()
	dir := b.TempDir()
	content := `<?php
function local_bench_helper() {
    get_context_instance(CONTEXT_COURSE, 1);
    $x = $this->get_context_instance(1);
}
`
	if err := os.WriteFile(filepath.Join(dir, "lib.php"), []byte(content), 0o644); err != nil {
		b.Fatalf("write lib.php: %v", err)
	}
	return dir
}

// BenchmarkFindDeprecatedApiUsage_Uncached measures the scan when the pattern is recompiled on
// every call.
func BenchmarkFindDeprecatedApiUsage_Uncached(b *testing.B) {
	dir := benchDeprecatedPluginDir(b)
	deprecated := benchDeprecatedNames()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		findDeprecatedApiUsageUncached(dir, deprecated)
	}
}

// BenchmarkFindDeprecatedApiUsage_Cached measures FindDeprecatedApiUsage, which reuses the cached
// compiled pattern across calls with the same deprecated set.
func BenchmarkFindDeprecatedApiUsage_Cached(b *testing.B) {
	dir := benchDeprecatedPluginDir(b)
	deprecated := benchDeprecatedNames()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		FindDeprecatedApiUsage(dir, deprecated)
	}
}
