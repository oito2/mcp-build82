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
	"testing"
)

// TestReadSupported verifies the $plugin->supported value read from version.php.
func TestReadSupported(t *testing.T) {
	cases := map[string]string{
		"$plugin->supported = [401, 405];\n":                        "[401, 405]",
		"$plugin->supported = [\n    401,\n    502,\n]; // range\n": "[401, 502,]",
		"$plugin->supported  =  array(400, 404);\n":                 "array(400, 404)",
		"// $plugin->supported = [401, 405];\n":                     "",
		"/* $plugin->supported = [401, 405]; */\n":                  "",
		"$plugin->version = 2024010100;\n":                          "",
	}
	for body, want := range cases {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "version.php"), []byte("<?php\n"+body), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := ReadSupported(dir); got != want {
			t.Errorf("ReadSupported(%q) = %q, want %q", body, got, want)
		}
	}
	if got := ReadSupported(t.TempDir()); got != "" {
		t.Errorf("missing version.php: got %q", got)
	}
}
