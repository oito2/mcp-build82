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

package phparray

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// keyedPatternCache caches the compiled regexes that ExtractArrayBody, ExtractString, ExtractInt
// and ExtractBool build per call from a caller-supplied key. The pattern template is fixed per
// function but the key varies, and callers pass the same small set of keys repeatedly, so caching
// by the fully assembled pattern string avoids recompiling the same regex on every call.
// CachedPattern exposes the cache to other packages.
var keyedPatternCache sync.Map // string pattern -> *regexp.Regexp

// CachedPattern compiles pattern on first use and returns the cached *regexp.Regexp on every
// subsequent call with the same pattern string. Safe for concurrent use (backed by sync.Map), so
// it can be shared by goroutines handling concurrent requests or running extractors in parallel.
func CachedPattern(pattern string) *regexp.Regexp {
	if v, ok := keyedPatternCache.Load(pattern); ok {
		return v.(*regexp.Regexp)
	}
	re := regexp.MustCompile(pattern)
	actual, _ := keyedPatternCache.LoadOrStore(pattern, re)
	return actual.(*regexp.Regexp)
}

// cachedPattern is the package-internal alias of CachedPattern.
func cachedPattern(pattern string) *regexp.Regexp {
	return CachedPattern(pattern)
}

// ExtractArrayBody finds "$varName = [ ... ]" or "$varName = array( ... )" and returns the inner
// body (everything between the outer brackets, exclusive). Returns ("", false) if the assignment
// isn't found or the bracket never closes.
func ExtractArrayBody(content, varName string) (string, bool) {
	re := cachedPattern(`\$` + regexp.QuoteMeta(varName) + `\s*=\s*`)
	loc := re.FindStringIndex(content)
	if loc == nil {
		return "", false
	}

	startIdx := -1
	for i := loc[1]; i < len(content); i++ {
		if content[i] == '[' || content[i] == '(' {
			startIdx = i
			break
		}
	}
	if startIdx == -1 {
		return "", false
	}

	openChar, closeChar := content[startIdx], byte(')')
	if openChar == '[' {
		closeChar = ']'
	}

	end := FindBalancedEnd(content, startIdx, openChar, closeChar)
	if end == -1 {
		return "", false
	}
	return content[startIdx+1 : end], true
}

// phpScanState is the running state of a minimal PHP lexical scanner shared by every
// bracket/paren-depth helper below: whether the current byte is inside a single- or
// double-quoted string, a `//`/`#` line comment, or a `/* */` block comment. In any of those
// states, bracket/paren/quote characters are not real PHP syntax and must be skipped verbatim —
// otherwise a help string like "range [0, 1)", a regex literal `'/[a-z]/'`, or an apostrophe
// inside a comment ("// We don't modify the session.") desynchronizes the depth count.
type phpScanState struct {
	quote        byte // 0 when not inside a string; else the active quote character
	lineComment  bool
	blockComment bool
}

// step advances the scanner past s[i] given the current state (mutated in place), returning how
// many extra bytes beyond s[i] were also consumed (0 normally; 1 for a two-byte escape sequence or
// comment delimiter) and whether s[i] is live PHP syntax the caller should interpret (false while
// absorbed into a string/comment). When live is true, extra is always 0.
func (st *phpScanState) step(s string, i int) (extra int, live bool) {
	c := s[i]
	switch {
	case st.quote != 0:
		if c == '\\' {
			return 1, false
		}
		if c == st.quote {
			st.quote = 0
		}
		return 0, false
	case st.lineComment:
		if c == '\n' {
			st.lineComment = false
		}
		return 0, false
	case st.blockComment:
		if c == '*' && i+1 < len(s) && s[i+1] == '/' {
			st.blockComment = false
			return 1, false
		}
		return 0, false
	case c == '\'' || c == '"':
		st.quote = c
		return 0, false
	case c == '/' && i+1 < len(s) && s[i+1] == '/':
		st.lineComment = true
		return 1, false
	case c == '#':
		st.lineComment = true
		return 0, false
	case c == '/' && i+1 < len(s) && s[i+1] == '*':
		st.blockComment = true
		return 1, false
	default:
		return 0, true
	}
}

// FindBalancedEnd scans s starting at openIdx (which must hold an open/closeChar-typed bracket or
// paren) and returns the index of the matching close delimiter, skipping string/comment content
// via phpScanState. Returns -1 if the delimiter never closes.
func FindBalancedEnd(s string, openIdx int, openChar, closeChar byte) int {
	depth := 0
	var st phpScanState
	for i := openIdx; i < len(s); i++ {
		extra, live := st.step(s, i)
		if extra > 0 {
			i += extra
		}
		if !live {
			continue
		}
		switch s[i] {
		case openChar:
			depth++
		case closeChar:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// SplitIntoBlocks splits a sequential array body ("[...], [...], [...]") into individual
// top-level bracket-balanced blocks, tracking depth so nested arrays inside an entry don't cause
// a premature split.
func SplitIntoBlocks(body string) []string {
	var blocks []string
	depth := 0
	start := -1
	var st phpScanState

	for i := 0; i < len(body); i++ {
		extra, live := st.step(body, i)
		if extra > 0 {
			i += extra
		}
		if !live {
			continue
		}
		switch body[i] {
		case '[', '(':
			if depth == 0 {
				start = i
			}
			depth++
		case ']', ')':
			depth--
			if depth == 0 && start != -1 {
				blocks = append(blocks, body[start:i+1])
				start = -1
			}
		}
	}
	return blocks
}

// ExtractString extracts a single-quoted or double-quoted string value for the given key from a
// block, e.g. 'key' => 'value'. Single-quoted values honor the \\ and \' escapes; double-quoted values honor \\ and \".
func ExtractString(block, key string) string {
	re := cachedPattern(`['"]` + regexp.QuoteMeta(key) + `['"]\s*=>\s*(?:'((?:[^'\\]|\\.)+)'|"((?:[^"\\]|\\.)+)")`)
	m := re.FindStringSubmatch(block)
	if m == nil {
		return ""
	}
	if m[1] != "" {
		return UnescapeString(m[1])
	}
	return strings.ReplaceAll(strings.ReplaceAll(m[2], `\\`, `\`), `\"`, `"`)
}

// classConstExpr matches a `X::class` class-name constant, capturing the class name as written
// (with or without a leading backslash).
const classConstExpr = `(\\?[A-Za-z_][A-Za-z0-9_]*(?:\\[A-Za-z_][A-Za-z0-9_]*)*)\s*::\s*class\b`

// ExtractStringOrClass extracts the value of `key` in `block` like ExtractString, and also accepts
// the class-name forms Moodle documents for hook, event, task and service registrations:
// `X::class` gives "X", `X::class . '::m'` gives "X::m", and `[X::class, 'm']` or `['X', 'm']`
// (or the array(...) spelling) gives "X::m". The class name keeps its leading backslash when
// written with one.
func ExtractStringOrClass(block, key string) string {
	if s := ExtractString(block, key); s != "" {
		return s
	}
	k := `['"]` + regexp.QuoteMeta(key) + `['"]\s*=>\s*`
	if m := cachedPattern(k + `(?:\[|array\s*\()\s*` + classConstExpr + `\s*,\s*['"](\w+)['"]\s*(?:\]|\))`).FindStringSubmatch(block); m != nil {
		return m[1] + "::" + m[2]
	}
	if m := cachedPattern(k + `(?:\[|array\s*\()\s*['"]([^'"]+)['"]\s*,\s*['"](\w+)['"]\s*(?:\]|\))`).FindStringSubmatch(block); m != nil {
		return UnescapeString(m[1]) + "::" + m[2]
	}
	if m := cachedPattern(k + classConstExpr + `(?:\s*\.\s*['"](::\w+)['"])?`).FindStringSubmatch(block); m != nil {
		return m[1] + m[2]
	}
	return ""
}

// UnescapeString applies PHP single-quoted-string unescaping to raw string content already
// stripped of its surrounding quotes: \\ -> \, then \' -> ' (order matters — a doubled backslash
// must not be re-touched by the second replacement). Shared by every caller that extracts a raw
// string literal's inner text, regex-based or otherwise (e.g. a tree-sitter `string_content` node).
func UnescapeString(raw string) string {
	v := strings.ReplaceAll(raw, `\\`, `\`)
	v = strings.ReplaceAll(v, `\'`, `'`)
	return v
}

// ExtractInt extracts an integer value for the given key, or defaultVal if the key is absent.
func ExtractInt(block, key string, defaultVal int) int {
	re := cachedPattern(`['"]` + regexp.QuoteMeta(key) + `['"]\s*=>\s*(-?\d+)`)
	m := re.FindStringSubmatch(block)
	if m == nil {
		return defaultVal
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// ExtractBool extracts a boolean value (true/false/1/0, case-insensitive) for the given key, or
// defaultVal if the key is absent.
func ExtractBool(block, key string, defaultVal bool) bool {
	re := cachedPattern(`(?i)['"]` + regexp.QuoteMeta(key) + `['"]\s*=>\s*(true|false|1|0)\b`)
	m := re.FindStringSubmatch(block)
	if m == nil {
		return defaultVal
	}
	v := strings.ToLower(m[1])
	return v == "true" || v == "1"
}

// KeyedBlock is one entry found by SplitKeyedEntries: a top-level array key and its
// bracket-balanced value body.
type KeyedBlock struct {
	Key  string
	Body string
}

// SplitKeyedEntries finds top-level `'key' => [ ... ]` or `'key' => array( ... )` entries inside a
// keyed array body and returns each key with its bracket-balanced body. keyPattern must have
// exactly two capturing groups: the key itself, and the opening bracket/`array(` marker
// immediately following it.
func SplitKeyedEntries(body string, keyPattern *regexp.Regexp) []KeyedBlock {
	var entries []KeyedBlock
	matches := keyPattern.FindAllStringSubmatchIndex(body, -1)
	for _, m := range matches {
		key := body[m[2]:m[3]]
		blockStart := m[1] - 1 // position of the opening [ or ( — last char of the full match
		openChar := body[blockStart]
		closeChar := byte(')')
		if openChar == '[' {
			closeChar = ']'
		}
		end := FindBalancedEnd(body, blockStart, openChar, closeChar)
		if end != -1 {
			entries = append(entries, KeyedBlock{Key: key, Body: body[blockStart : end+1]})
		}
	}
	return entries
}

// Patterns of the PHP 8 attributes Moodle (4.4+) puts on hook classes: the label, the tags and the
// legacy callbacks the hook replaces. The namespace may be written in full (\core\attribute\...),
// or relative to an imported core namespace (attribute\...).
var (
	hookLabelAttrPattern    = regexp.MustCompile(`#\[\s*\\?(?:core\\)?attribute\\label\(\s*(?:'((?:[^'\\]|\\.)*)'|"((?:[^"\\]|\\.)*)")`)
	hookTagsAttrPattern     = regexp.MustCompile(`#\[\s*\\?(?:core\\)?attribute\\tags\(([^)]*)\)`)
	hookReplacesAttrPattern = regexp.MustCompile(`#\[\s*\\?(?:core\\)?attribute\\hook\\replaces_callbacks\(([^)]*)\)`)
	quotedArgPattern        = regexp.MustCompile(`'((?:[^'\\]|\\.)*)'|"((?:[^"\\]|\\.)*)"`)
)

// HookAttributes reads the hook metadata a hook class declares through attributes in `content`:
// the #[\core\attribute\label(...)] text, the #[\core\attribute\tags(...)] values, and the
// #[\core\attribute\hook\replaces_callbacks(...)] callbacks joined with ", ". Each is empty when
// the attribute is absent.
func HookAttributes(content string) (label string, tags []string, replaces string) {
	if m := hookLabelAttrPattern.FindStringSubmatch(content); m != nil {
		label = UnescapeString(m[1] + m[2])
	}
	if m := hookTagsAttrPattern.FindStringSubmatch(content); m != nil {
		tags = quotedArgs(m[1])
	}
	if m := hookReplacesAttrPattern.FindStringSubmatch(content); m != nil {
		replaces = strings.Join(quotedArgs(m[1]), ", ")
	}
	return label, tags, replaces
}

// quotedArgs returns the quoted string literals of an argument list, unescaped, in order.
func quotedArgs(args string) []string {
	var out []string
	for _, m := range quotedArgPattern.FindAllStringSubmatch(args, -1) {
		out = append(out, UnescapeString(m[1]+m[2]))
	}
	return out
}
