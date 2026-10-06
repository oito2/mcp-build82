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

import "os"

// useTreesitter reports whether the opt-in tree-sitter-php backend (internal/extractors/tsbackend)
// should be used instead of the default regex/bracket-depth backend. The environment variable
// BUILD82_EXTRACTOR_BACKEND is read on every call rather than cached, so it can change at runtime.
// The regex backend is used for every value other than exactly "treesitter".
func useTreesitter() bool {
	return os.Getenv("BUILD82_EXTRACTOR_BACKEND") == "treesitter"
}
