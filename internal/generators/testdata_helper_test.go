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

package generators

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/oito2/mcp-build82/internal/cache"
)

// swapGlobalCache replaces the shared cache.Global instance and returns the previous one, so a
// test can simulate a fresh process (a cold cache) or restore the original afterward.
func swapGlobalCache(c *cache.MtimeCache) *cache.MtimeCache {
	old := cache.Global
	cache.Global = c
	return old
}

// copyFixtureMoodleTree copies the shared testdata/moodle fixture tree into a fresh temp
// directory and returns its path — generators write files, so tests must never run directly
// against the checked-in fixture.
func copyFixtureMoodleTree(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "moodle")
	dst := t.TempDir()

	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
	if err != nil {
		t.Fatalf("copying fixture tree: %v", err)
	}
	return dst
}
