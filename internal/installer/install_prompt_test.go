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

package installer

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// TestPromptMoodlePath_EOFReturnsError verifies that closed or exhausted stdin yields an error
// instead of looping, both when nothing is read and when the last line is not a Moodle root.
func TestPromptMoodlePath_EOFReturnsError(t *testing.T) {
	t.Chdir(t.TempDir())

	for _, input := range []string{"", "/definitely/not/moodle", "/definitely/not/moodle\n"} {
		in := bufio.NewReader(strings.NewReader(input))
		captureStdout(t, func() {
			if path, err := promptMoodlePath(in); err == nil {
				t.Errorf("input %q: expected an error, got path %q", input, path)
			}
		})
	}
}

// TestPromptMoodlePath_AcceptsValidRoot verifies that a valid Moodle root typed at the prompt is
// returned.
func TestPromptMoodlePath_AcceptsValidRoot(t *testing.T) {
	t.Chdir(t.TempDir())
	root := t.TempDir()
	mustMkdir(t, root+"/lib")
	for _, f := range []string{"version.php", "config.php"} {
		if err := os.WriteFile(root+"/"+f, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	in := bufio.NewReader(strings.NewReader(root + "\n"))
	captureStdout(t, func() {
		if path, err := promptMoodlePath(in); err != nil || path != root {
			t.Errorf("got %q, %v; want %q", path, err, root)
		}
	})
}
