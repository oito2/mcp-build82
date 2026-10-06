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

package phpdoc

import "testing"

// TestClassifyVisibility covers each classification rule in priority order plus the cases where two rules compete.
func TestClassifyVisibility(t *testing.T) {
	cases := []struct {
		name string
		fn   string
		doc  *PhpDocBlock
		want Visibility
	}{
		{"access private alone", "foo", &PhpDocBlock{AccessPrivate: true}, Private},
		{"internal tag alone", "foo", &PhpDocBlock{Internal: true}, Internal},
		{"leading underscore, no doc", "_foo", nil, Private},
		{"_internal suffix, no doc", "foo_internal", nil, Internal},
		{"deprecated, no other markers", "foo", &PhpDocBlock{Deprecated: "yes"}, Deprecated},
		{"no phpdoc at all", "foo", nil, Unverified},
		{"phpdoc present, no markers", "foo", &PhpDocBlock{Summary: "does a thing"}, Public},

		{"private wins over deprecated (step 1 < step 5)", "foo",
			&PhpDocBlock{AccessPrivate: true, Deprecated: "yes"}, Private},
		{"CORRECTED: internal tag wins over leading-underscore name (step 2 < step 3)", "_foo",
			&PhpDocBlock{Internal: true}, Internal},
		{"_internal suffix wins over deprecated (step 4 < step 5)", "foo_internal",
			&PhpDocBlock{Deprecated: "yes"}, Internal},
		{"leading underscore wins over _internal suffix (step 3 < step 4)", "_foo_internal",
			&PhpDocBlock{Deprecated: "yes"}, Private},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyVisibility(c.fn, c.doc)
			if got != c.want {
				t.Errorf("ClassifyVisibility(%q, %+v) = %v, want %v", c.fn, c.doc, got, c.want)
			}
		})
	}
}

// TestParseDocBlock_BasicFields verifies the field extraction of ParseDocBlock.
func TestParseDocBlock_BasicFields(t *testing.T) {
	raw := `/**
 * Returns the number of widgets configured for a course.
 *
 * @param int $courseid Course ID
 * @return int Number of widgets
 * @since Moodle 4.0
 * @throws moodle_exception
 */`
	doc := ParseDocBlock(raw)
	if doc.Summary != "Returns the number of widgets configured for a course." {
		t.Errorf("Summary mismatch: %q", doc.Summary)
	}
	if len(doc.Params) != 1 || doc.Params[0] != "int $courseid Course ID" {
		t.Errorf("Params mismatch: %+v", doc.Params)
	}
	if doc.Returns != "int Number of widgets" {
		t.Errorf("Returns mismatch: %q", doc.Returns)
	}
	if doc.Since != "Moodle 4.0" {
		t.Errorf("Since mismatch: %q", doc.Since)
	}
	if len(doc.Throws) != 1 || doc.Throws[0] != "moodle_exception" {
		t.Errorf("Throws mismatch: %+v", doc.Throws)
	}
	if doc.Raw != raw {
		t.Error("Raw must preserve the original text verbatim")
	}
}

// TestParseDocBlock_BareSummaryWithNoTags verifies that a docblock with only a summary and no @tag
// line (the common shape for a class-level docblock) yields a summary without a stray "/" from the
// closing "*/" line.
func TestParseDocBlock_BareSummaryWithNoTags(t *testing.T) {
	raw := `/**
 * The real class summary that should be extracted.
 */`
	doc := ParseDocBlock(raw)
	if doc.Summary != "The real class summary that should be extracted." {
		t.Errorf("Summary mismatch: %q", doc.Summary)
	}
}

func TestParseDocBlock_SingleLine(t *testing.T) {
	if got := ParseDocBlock("/** text */").Summary; got != "text" {
		t.Errorf("Summary = %q, want %q", got, "text")
	}
	d := ParseDocBlock("/** @deprecated since 4.0 */")
	if d.Summary != "" || d.Deprecated != "since 4.0" {
		t.Errorf("got Summary=%q Deprecated=%q", d.Summary, d.Deprecated)
	}
}
