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

package generators

import (
	"os"
	"testing"

	"github.com/oito2/mcp-build82/internal/extractors"
)

// benchRealMoodleRoot is the path of a full Moodle install used by the benchmarks, which skip
// when it is absent.
const benchRealMoodleRoot = "/srv/workspace/www/html/mdle/dev-500"

// BenchmarkClassesIndex_FullTreeWalk measures parsing every *.php file of the installation and
// discarding those outside classes/ directories.
func BenchmarkClassesIndex_FullTreeWalk(b *testing.B) {
	if _, err := os.Stat(benchRealMoodleRoot); err != nil {
		b.Skipf("real Moodle install not available at %s: %v", benchRealMoodleRoot, err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		extractors.ExtractClasses(benchRealMoodleRoot, benchRealMoodleRoot, "**/*.php")
	}
}

// BenchmarkClassesIndex_RestrictedGlob measures GenerateClassesIndex's approach: globbing the
// classes/ files first and parsing only those.
func BenchmarkClassesIndex_RestrictedGlob(b *testing.B) {
	if _, err := os.Stat(benchRealMoodleRoot); err != nil {
		b.Skipf("real Moodle install not available at %s: %v", benchRealMoodleRoot, err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		files := globMoodleClassesPhp(benchRealMoodleRoot)
		extractors.ExtractClassesFromFiles(files, benchRealMoodleRoot)
	}
}
