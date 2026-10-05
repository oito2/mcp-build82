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

package extractors

import (
	"os"
	"testing"
)

// benchRealLibPHP is a large real file (lib/moodlelib.php, ~10.1k lines) read directly from a
// Moodle installation rather than vendored into testdata; both benchmarks skip if it isn't present
// in the current environment.
const benchRealLibPHP = "/srv/workspace/www/html/mdle/dev-500/lib/moodlelib.php"

func BenchmarkExtractFunctionsFromPhpFile_Regex(b *testing.B) {
	if _, err := os.Stat(benchRealLibPHP); err != nil {
		b.Skipf("real moodlelib.php not available at %s: %v", benchRealLibPHP, err)
	}
	b.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ExtractFunctionsFromPhpFile(benchRealLibPHP)
	}
}

func BenchmarkExtractFunctionsFromPhpFile_Treesitter(b *testing.B) {
	if _, err := os.Stat(benchRealLibPHP); err != nil {
		b.Skipf("real moodlelib.php not available at %s: %v", benchRealLibPHP, err)
	}
	b.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ExtractFunctionsFromPhpFile(benchRealLibPHP)
	}
}
