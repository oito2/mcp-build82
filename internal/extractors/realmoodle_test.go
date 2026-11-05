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
)

// defaultRealMoodleDir is the directory holding the local Moodle installations (mdle-401,
// mdle-500, ...) read by the tests and benchmarks that use real core files.
const defaultRealMoodleDir = "/srv/workspace/localhost/html"

// realMoodleDir returns the directory holding the local Moodle installations:
// BUILD82_REAL_MOODLE_DIR when set, defaultRealMoodleDir otherwise. The tests that use it skip
// themselves when an installation is absent.
func realMoodleDir() string {
	if v := os.Getenv("BUILD82_REAL_MOODLE_DIR"); v != "" {
		return v
	}
	return defaultRealMoodleDir
}

// realMoodle returns the root of the local Moodle installation `name` (such as "mdle-500").
func realMoodle(name string) string {
	return filepath.Join(realMoodleDir(), name)
}
