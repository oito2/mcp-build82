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

package genutil

import (
	"testing"

	"github.com/oito2/mcp-build82/internal/cache"
)

// TestContextDirStaysInSyncWithCache verifies that cache.ContextDirName equals this package's
// ContextDir (".build82"). cache duplicates the literal because genutil already imports cache.
func TestContextDirStaysInSyncWithCache(t *testing.T) {
	if cache.ContextDirName != ContextDir {
		t.Fatalf("cache.ContextDirName (%q) and genutil.ContextDir (%q) have drifted apart — update both to the same value", cache.ContextDirName, ContextDir)
	}
}
