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

// stringScanner tracks whether a byte-by-byte scan of JSON text is inside a string literal.
type stringScanner struct {
	inString, escaped bool
}

// consume advances the scanner over `c` and reports whether `c` belongs to a string literal,
// opening and closing quotes included.
func (s *stringScanner) consume(c byte) bool {
	if s.inString {
		switch {
		case s.escaped:
			s.escaped = false
		case c == '\\':
			s.escaped = true
		case c == '"':
			s.inString = false
		}
		return true
	}
	if c == '"' {
		s.inString = true
		return true
	}
	return false
}

// stripJSONC converts JSON-with-comments (as accepted by Zed, OpenCode and VS Code based tools)
// into strict JSON: `//` line comments and `/* */` block comments are replaced by whitespace, and
// commas that directly precede a closing `}` or `]` are dropped. String literals are left intact,
// so comment-like or comma-like text inside them is never touched.
//
// The result is only ever used for reading. Rewriting a file from it would silently discard the
// user's comments, so callers refuse to write back any file that needed this conversion.
func stripJSONC(src []byte) []byte {
	out := make([]byte, 0, len(src))
	var str stringScanner
	for i := 0; i < len(src); i++ {
		c := src[i]
		if str.consume(c) {
			out = append(out, c)
			continue
		}
		switch {
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			if i < len(src) {
				out = append(out, '\n')
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			i += 2
			for i+1 < len(src) && (src[i] != '*' || src[i+1] != '/') {
				i++
			}
			i++ // lands on the closing '/', skipped by the loop increment
			out = append(out, ' ')
		default:
			out = append(out, c)
		}
	}
	return dropTrailingCommas(out)
}

// dropTrailingCommas removes every comma, outside string literals, whose next non-whitespace
// character is `}` or `]`. Its input must already be free of comments.
func dropTrailingCommas(src []byte) []byte {
	out := make([]byte, 0, len(src))
	var str stringScanner
	for i := 0; i < len(src); i++ {
		c := src[i]
		if !str.consume(c) && c == ',' {
			j := i + 1
			for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n' || src[j] == '\r') {
				j++
			}
			if j < len(src) && (src[j] == '}' || src[j] == ']') {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}
