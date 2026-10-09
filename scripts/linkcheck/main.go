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

// Command linkcheck verifies the relative links of the project's Markdown documentation: every
// link to a local file must point to a file or directory that exists, and every "#anchor" must
// match a heading (or an explicit HTML anchor) of the target page, using GitHub's heading slugs.
// External links (http, https, mailto) are not checked. Usage, from the module root:
//
//	go run ./scripts/linkcheck
//
// It prints one line per broken link and exits with status 1 when there is any.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// roots are the Markdown files and directories checked, relative to the module root.
var roots = []string{"README.md", "CONTRIBUTING.md", "CODE_OF_CONDUCT.md", "CHANGELOG.md", "docs"}

// linkPattern matches an inline Markdown link or image target: the text in parentheses after
// "](", up to the first closing parenthesis or space (a link title is ignored).
var linkPattern = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)

// headingPattern matches an ATX heading line and captures its text.
var headingPattern = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)

// htmlAnchorPattern matches an explicit HTML anchor such as <a id="x"> or <a name="x">.
var htmlAnchorPattern = regexp.MustCompile(`<a\s+(?:id|name)="([^"]+)"`)

// main checks every Markdown file under roots and reports the broken links.
func main() {
	files, err := markdownFiles(roots)
	if err != nil {
		fmt.Fprintln(os.Stderr, "linkcheck:", err)
		os.Exit(1)
	}
	problems := check(files)
	for _, p := range problems {
		fmt.Println(p)
	}
	if len(problems) > 0 {
		fmt.Fprintf(os.Stderr, "linkcheck: %d broken link(s) in %d file(s) checked\n", len(problems), len(files))
		os.Exit(1)
	}
	fmt.Printf("linkcheck: %d file(s), no broken links\n", len(files))
}

// markdownFiles returns every .md file named by `paths` or found under the directories among them,
// sorted. A missing path is an error.
func markdownFiles(paths []string) ([]string, error) {
	var files []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			files = append(files, p)
			continue
		}
		err = filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".md") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

// check returns one "file:line: message" entry per broken link found in `files`.
func check(files []string) []string {
	anchorCache := map[string]map[string]bool{}
	anchorsOf := func(path string) (map[string]bool, error) {
		if a, ok := anchorCache[path]; ok {
			return a, nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		a := anchors(string(content))
		anchorCache[path] = a
		return a, nil
	}

	var problems []string
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", file, err))
			continue
		}
		for _, l := range links(string(content)) {
			target, anchor, _ := strings.Cut(l.target, "#")
			if isExternal(target) {
				continue
			}
			path := file
			if target != "" {
				path = filepath.Join(filepath.Dir(file), filepath.FromSlash(target))
				info, err := os.Stat(path)
				if err != nil {
					problems = append(problems, fmt.Sprintf("%s:%d: %s: target does not exist", file, l.line, l.target))
					continue
				}
				if info.IsDir() || anchor == "" || !strings.HasSuffix(path, ".md") {
					continue
				}
			}
			if anchor == "" {
				continue
			}
			a, err := anchorsOf(path)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s:%d: %s: %v", file, l.line, l.target, err))
				continue
			}
			if !a[strings.ToLower(anchor)] {
				problems = append(problems, fmt.Sprintf("%s:%d: %s: no heading or anchor %q in %s", file, l.line, l.target, anchor, path))
			}
		}
	}
	return problems
}

// link is one link target found in a Markdown file, with its 1-based line number.
type link struct {
	target string
	line   int
}

// isExternal reports whether `target` is a link this checker does not resolve: a URL with a
// scheme such as http, https or mailto.
func isExternal(target string) bool {
	return strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:")
}

// links returns the inline link targets of the Markdown `content`, skipping fenced code blocks and
// inline code spans.
func links(content string) []link {
	var out []link
	inFence := false
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, m := range linkPattern.FindAllStringSubmatch(stripCodeSpans(line), -1) {
			target := strings.Trim(m[1], "<>")
			out = append(out, link{target: target, line: i + 1})
		}
	}
	return out
}

// stripCodeSpans removes the inline code spans (text between backticks) from `line`.
func stripCodeSpans(line string) string {
	var b strings.Builder
	inCode := false
	for _, r := range line {
		if r == '`' {
			inCode = !inCode
			continue
		}
		if !inCode {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// anchors returns the set of anchors a Markdown page exposes: the GitHub slug of every heading
// outside code fences (repeated slugs get "-1", "-2", ... suffixes) and every explicit HTML anchor.
func anchors(content string) map[string]bool {
	out := map[string]bool{}
	seen := map[string]int{}
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, m := range htmlAnchorPattern.FindAllStringSubmatch(line, -1) {
			out[strings.ToLower(m[1])] = true
		}
		m := headingPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		slug := slugify(m[1])
		if n := seen[slug]; n > 0 {
			out[fmt.Sprintf("%s-%d", slug, n)] = true
		} else {
			out[slug] = true
		}
		seen[slug]++
	}
	return out
}

// mdLinkInHeading matches a Markdown link inside heading text, whose visible text is kept.
var mdLinkInHeading = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

// slugify returns GitHub's anchor for the heading text `heading`: links reduced to their text,
// lower-cased, every character that is not a letter, mark, number, connector punctuation, space or
// hyphen removed, and each space replaced by a hyphen.
func slugify(heading string) string {
	heading = mdLinkInHeading.ReplaceAllString(heading, "$1")
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r) || unicode.Is(unicode.Pc, r):
			b.WriteRune(r)
		}
	}
	return b.String()
}
