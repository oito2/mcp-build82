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

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSlugify verifies GitHub's heading slugs, including code spans, punctuation, emoji (whose
// variation selector is kept) and non-ASCII letters.
func TestSlugify(t *testing.T) {
	for heading, want := range map[string]string{
		"`self-update`":                        "self-update",
		"Subcommand help, `--` and interrupts": "subcommand-help----and-interrupts",
		"⚠️ Troubleshooting":                   "️-troubleshooting",
		"Códigos de saída":                     "códigos-de-saída",
		"Step 1: [Install](x.md) it!":          "step-1-install-it",
	} {
		if got := slugify(heading); got != want {
			t.Errorf("slugify(%q) = %q, want %q", heading, got, want)
		}
	}
}

// TestCheck_ReportsMissingFilesAndAnchors verifies that a missing file and a missing anchor are
// reported, while valid relative links, repeated headings, HTML anchors, code and external links
// are accepted.
func TestCheck_ReportsMissingFilesAndAnchors(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write("b.md", "# Title\n\n## Usage\n\n## Usage\n\n<a id=\"custom\"></a>\n")
	a := write("a.md", strings.Join([]string{
		"[ok](b.md) [ok](b.md#usage) [ok](b.md#usage-1) [ok](b.md#custom) [ok](#local)",
		"[ext](https://example.com/x#y) `[code](missing.md)`",
		"```",
		"[fenced](missing.md)",
		"```",
		"# Local",
		"[bad](missing.md) [bad](b.md#nope)",
	}, "\n"))

	problems := check([]string{a})
	if len(problems) != 2 {
		t.Fatalf("expected 2 problems, got %d: %v", len(problems), problems)
	}
	if !strings.Contains(problems[0], "missing.md: target does not exist") || !strings.Contains(problems[1], `no heading or anchor "nope"`) {
		t.Errorf("unexpected problems: %v", problems)
	}
}
