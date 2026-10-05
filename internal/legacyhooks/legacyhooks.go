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

// Package legacyhooks holds the legacy-callback-suffix → Hook API replacement map shared by both
// extraction backends (internal/extractors' regex backend and internal/extractors/tsbackend's
// tree-sitter backend). It lives in its own package so neither backend needs to import the other
// for this data.
package legacyhooks

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed legacy_hooks.json
var data []byte

// Map is the legacy lib.php callback suffix (e.g. "before_footer") to its Hook API replacement FQN
// (e.g. \core\hook\output\before_footer).
var Map = func() map[string]string {
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		panic(fmt.Sprintf("legacyhooks: malformed embedded legacy_hooks.json: %v", err))
	}
	return m
}()
