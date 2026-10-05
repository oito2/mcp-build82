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
	"reflect"
	"testing"

	"github.com/oito2/mcp-build82/internal/extractors/tsbackend"
	"github.com/oito2/mcp-build82/internal/phptypes"
)

func TestUseTreesitter_DefaultFalse(t *testing.T) {
	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
	if useTreesitter() {
		t.Error("expected false when unset")
	}
}

func TestUseTreesitter_ExactMatchOnly(t *testing.T) {
	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "TreeSitter")
	if useTreesitter() {
		t.Error("expected false for a near-match value — must be exactly \"treesitter\"")
	}
}

func TestUseTreesitter_True(t *testing.T) {
	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
	if !useTreesitter() {
		t.Error("expected true when set to \"treesitter\"")
	}
}

// TestBackendsShareExactSameType verifies that both backends return the exact same Go type
// (internal/phptypes.EventsExtraction), not two structurally identical but distinct types
// reconciled by a conversion function. reflect.TypeOf on a value returned by each backend must
// report the identical type; a structural check (e.g. comparing field names) would be weaker.
func TestBackendsShareExactSameType(t *testing.T) {
	regexResult := ParseEventsPhp("/does/not/exist/events.php")
	tsResult := tsbackend.ParseEventsPhp("/does/not/exist/events.php")

	regexType := reflect.TypeOf(regexResult)
	tsType := reflect.TypeOf(tsResult)
	if regexType != tsType {
		t.Fatalf("expected the regex and tree-sitter backends to return the identical type, got %v (regex) vs %v (tsbackend)", regexType, tsType)
	}

	wantType := reflect.TypeOf(&phptypes.EventsExtraction{})
	if regexType != wantType {
		t.Fatalf("expected *phptypes.EventsExtraction, got %v", regexType)
	}
}
