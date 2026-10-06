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
	"regexp"
	"testing"
)

// functionExistsInContentUncached mirrors functionExistsInContent but compiles the regex on every
// call, serving as the benchmark baseline.
func functionExistsInContentUncached(content []byte, name string) bool {
	if content == nil {
		return false
	}
	re := regexp.MustCompile(`(?im)^function\s+` + regexp.QuoteMeta(name) + `\s*\(`)
	return re.Match(content)
}

// benchLibPhpContent is a small lib.php sample used as benchmark input.
var benchLibPhpContent = []byte(`<?php
defined('MOODLE_INTERNAL') || die();

function local_test_cron() {
    return true;
}

function local_test_other_thing() {
    return false;
}
`)

// BenchmarkFunctionExistsInContent_Uncached is the baseline that recompiles the regex on every call.
func BenchmarkFunctionExistsInContent_Uncached(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		functionExistsInContentUncached(benchLibPhpContent, "local_test_cron")
	}
}

// BenchmarkFunctionExistsInContent_Cached measures functionExistsInContent, which reuses the
// regex cached by phparray.CachedPattern for repeated calls with the same name.
func BenchmarkFunctionExistsInContent_Cached(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		functionExistsInContent(benchLibPhpContent, "local_test_cron")
	}
}
