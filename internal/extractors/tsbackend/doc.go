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

// Package tsbackend implements the opt-in tree-sitter-php extraction backend
// (BUILD82_EXTRACTOR_BACKEND=treesitter) — an alternative to the default regex/bracket-depth
// backend in internal/extractors, built on github.com/odvcencio/gotreesitter (pure Go, no cgo).
package tsbackend
