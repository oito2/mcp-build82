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

// Package phpdoc holds the PHPDoc-block parsing and visibility-classification logic shared by
// both extraction backends (internal/extractors' regex backend and
// internal/extractors/tsbackend's tree-sitter backend). Neither backend imports the other, so the
// shared logic lives in this package. The tree-sitter backend only replaces the file-scanning step
// (finding function declarations and their preceding doc-comment); parsing and classification of
// the doc-comment happen here for both.
package phpdoc

import (
	"regexp"
	"strings"
)

// Visibility is the API visibility classification of a documented function; internal/extractors
// aliases it as ApiVisibility.
type Visibility string

// Visibility values, from most to least trusted API surface.
const (
	Public     Visibility = "public"
	Deprecated Visibility = "deprecated"
	Internal   Visibility = "internal"
	Private    Visibility = "private"
	Unverified Visibility = "unverified"
)

// PhpDocBlock is a parsed PHPDoc comment block; internal/extractors aliases it as PhpDocBlock.
type PhpDocBlock struct {
	Raw           string
	Summary       string
	Params        []string
	Returns       string
	Since         string
	Deprecated    string
	Throws        []string
	AccessPrivate bool
	Internal      bool
}

// docLineStripPattern matches the leading whitespace and `*` (plus one optional space) of a
// PHPDoc body line.
var docLineStripPattern = regexp.MustCompile(`^\s*\*\s?`)

// ParseDocBlock parses a raw PHPDoc block (including the /** and */ marker lines) into structured
// fields. The summary is the leading prose joined into one line; @param, @return, @since,
// @deprecated, @throws, @access and @internal tags populate the other fields, and Raw keeps the
// unmodified input. It never fails: unrecognized content is ignored.
func ParseDocBlock(raw string) PhpDocBlock {
	rawLines := strings.Split(raw, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, raw := range rawLines {
		// The opening/closing delimiter check runs against the line as originally written, before
		// docLineStripPattern's leading "*" strip: that pattern (^\s*\*\s?) consumes a solitary
		// "*/" line's own "*" too, leaving a stray "/" that would be treated as summary/tag content.
		if strings.TrimSpace(raw) == "/**" || strings.TrimSpace(raw) == "*/" {
			continue
		}
		// Inline delimiters, as in a single-line "/** text */" block, are removed from the line itself.
		if t := strings.TrimSpace(raw); strings.HasPrefix(t, "/**") {
			raw = strings.TrimPrefix(t, "/**")
		}
		if t := strings.TrimSpace(raw); strings.HasSuffix(t, "*/") {
			raw = strings.TrimSuffix(t, "*/")
		}
		l := strings.TrimSpace(docLineStripPattern.ReplaceAllString(raw, ""))
		lines = append(lines, l)
	}

	var doc PhpDocBlock
	inSummary := true

	for _, line := range lines {
		if inSummary {
			switch {
			case strings.HasPrefix(line, "@"):
				inSummary = false
			case line != "":
				if doc.Summary == "" {
					doc.Summary = line
				} else {
					doc.Summary = doc.Summary + " " + line
				}
				continue
			case doc.Summary != "":
				inSummary = false
				continue
			}
		}

		switch {
		case strings.HasPrefix(line, "@param"):
			doc.Params = append(doc.Params, strings.TrimSpace(strings.TrimPrefix(line, "@param")))
		case strings.HasPrefix(line, "@return"):
			doc.Returns = strings.TrimSpace(strings.TrimPrefix(line, "@return"))
		case strings.HasPrefix(line, "@since"):
			doc.Since = strings.TrimSpace(strings.TrimPrefix(line, "@since"))
		case strings.HasPrefix(line, "@deprecated"):
			msg := strings.TrimSpace(strings.TrimPrefix(line, "@deprecated"))
			if msg == "" {
				msg = "yes"
			}
			doc.Deprecated = msg
		case strings.HasPrefix(line, "@throws"):
			doc.Throws = append(doc.Throws, strings.TrimSpace(strings.TrimPrefix(line, "@throws")))
		case strings.HasPrefix(line, "@access"):
			val := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "@access")))
			if val == "private" || val == "protected" {
				doc.AccessPrivate = true
			}
		case strings.HasPrefix(line, "@internal"):
			doc.Internal = true
		}
	}

	doc.Raw = raw
	return doc
}

// ClassifyVisibility determines a function's visibility from its name and PHPDoc, checked in this
// exact priority order: an explicit @access private/protected tag always wins first, then an
// explicit @internal tag, then a leading-underscore name (Moodle's own private-by-convention
// naming), then an "_internal" name suffix, then @deprecated, then — if there's no PHPDoc at all —
// Unverified, and Public otherwise. Explicit PHPDoc tags are checked before name-based conventions
// throughout, since an explicit annotation is a stronger signal of intent than a naming pattern.
func ClassifyVisibility(name string, doc *PhpDocBlock) Visibility {
	if doc != nil && doc.AccessPrivate {
		return Private
	}
	if doc != nil && doc.Internal {
		return Internal
	}
	if strings.HasPrefix(name, "_") {
		return Private
	}
	if strings.HasSuffix(name, "_internal") {
		return Internal
	}
	if doc != nil && doc.Deprecated != "" {
		return Deprecated
	}
	if doc == nil {
		return Unverified
	}
	return Public
}
