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
	"os"
	"path/filepath"
	"testing"
)

const capabilitiesFixtureWellFormed = `<?php
$capabilities = [
    'local/test:view' => [
        'riskbitmask'  => RISK_PERSONAL,
        'captype'      => 'read',
        'contextlevel' => CONTEXT_COURSE,
        'archetypes'   => [
            'teacher'        => CAP_ALLOW,
            'editingteacher' => CAP_ALLOW,
            'manager'        => CAP_ALLOW,
        ],
    ],
    'local/test:manage' => [
        'captype'      => 'write',
        'contextlevel' => CONTEXT_SYSTEM,
        'archetypes'   => [
            'manager' => CAP_ALLOW,
        ],
    ],
];`

func TestParseAccessPhp(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "access.php"), capabilitiesFixtureWellFormed)

	result := ParseAccessPhp(filepath.Join(dir, "db", "access.php"))
	if result == nil || len(result.Capabilities) != 2 {
		t.Fatalf("expected 2 capabilities, got %+v", result)
	}
	cap := result.Capabilities[0]
	if cap.Name != "local/test:view" || cap.CapType != "read" || cap.ContextLevel != "CONTEXT_COURSE" {
		t.Errorf("capability mismatch: %+v", cap)
	}
	if cap.RiskBitmask != "RISK_PERSONAL" {
		t.Errorf("expected bare-constant riskbitmask fallback, got %q", cap.RiskBitmask)
	}
	if cap.Archetypes["teacher"] != "CAP_ALLOW" {
		t.Errorf("archetype mismatch: %+v", cap.Archetypes)
	}
	if len(cap.Archetypes) != 3 {
		t.Errorf("expected 3 archetypes, got %d: %+v", len(cap.Archetypes), cap.Archetypes)
	}
}

func TestParseAccessPhp_CapTypeDefaultsToRead(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	content := `<?php
$capabilities = [
    'local/test:noop' => [
        'contextlevel' => CONTEXT_SYSTEM,
    ],
];`
	mustWriteFile(t, filepath.Join(dir, "db", "access.php"), content)

	result := ParseAccessPhp(filepath.Join(dir, "db", "access.php"))
	if result == nil || len(result.Capabilities) != 1 || result.Capabilities[0].CapType != "read" {
		t.Fatalf("expected CapType to default to 'read', got %+v", result)
	}
}

func TestParseAccessPhp_DoubleQuotedKeyNotMatched(t *testing.T) {
	// Asymmetry of the *regex* backend specifically: only single-quoted capability keys are
	// matched. Pin the backend explicitly — the tree-sitter backend also matches double-quoted keys —
	// so this test is not sensitive to whichever backend the environment selects.
	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	content := `<?php
$capabilities = [
    "local/test:doublequoted" => [
        'captype' => 'read',
    ],
];`
	mustWriteFile(t, filepath.Join(dir, "db", "access.php"), content)

	result := ParseAccessPhp(filepath.Join(dir, "db", "access.php"))
	if result == nil || len(result.Capabilities) != 0 {
		t.Errorf("expected double-quoted capability keys to be ignored, got %+v", result)
	}
}

// TestParseAccessPhp_TreesitterBackendParity confirms BUILD82_EXTRACTOR_BACKEND=treesitter
// produces identical output to the regex backend for a normal, well-formed access.php (all
// single-quoted keys, no compound riskbitmask — the two known divergences don't apply here).
func TestParseAccessPhp_TreesitterBackendParity(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "access.php"), capabilitiesFixtureWellFormed)
	path := filepath.Join(dir, "db", "access.php")

	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
	regexResult := ParseAccessPhp(path)
	t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
	tsResult := ParseAccessPhp(path)

	if len(regexResult.Capabilities) != len(tsResult.Capabilities) {
		t.Fatalf("capability count mismatch: regex=%d treesitter=%d", len(regexResult.Capabilities), len(tsResult.Capabilities))
	}
	for i := range regexResult.Capabilities {
		assertCapabilityParity(t, path, regexResult.Capabilities[i].Name, regexResult.Capabilities[i], tsResult.Capabilities[i])
	}
}

// assertCapabilityParity compares one capability's fields, with an exception for RiskBitmask:
// the regex backend truncates a compound bitwise-OR expression to its first flag, while
// tree-sitter returns the full expression; the difference is logged, not failed. Every other
// field (including Archetypes) is asserted exactly.
func assertCapabilityParity(t *testing.T, path, name string, regex, ts Capability) {
	t.Helper()
	if regex.Name != ts.Name || regex.CapType != ts.CapType || regex.ContextLevel != ts.ContextLevel {
		t.Errorf("%s: capability %q mismatch (non-riskbitmask fields): regex=%+v treesitter=%+v", path, name, regex, ts)
		return
	}
	if len(regex.Archetypes) != len(ts.Archetypes) {
		t.Errorf("%s: capability %q archetype count mismatch: regex=%+v treesitter=%+v", path, name, regex.Archetypes, ts.Archetypes)
	} else {
		for k, v := range regex.Archetypes {
			if ts.Archetypes[k] != v {
				t.Errorf("%s: capability %q archetype %q mismatch: regex=%q treesitter=%q", path, name, k, v, ts.Archetypes[k])
			}
		}
	}
	if regex.RiskBitmask != ts.RiskBitmask {
		t.Logf("%s: capability %q RiskBitmask differs (expected per KNOWN_DIVERGENCES.md #4): regex=%q treesitter=%q", path, name, regex.RiskBitmask, ts.RiskBitmask)
	}
}

// TestParseAccessPhp_TreesitterBackendParity_RealFiles runs both backends against every real
// db/access.php in the available Moodle installations and confirms they agree, matched by
// capability Name rather than positional index: the regex backend also counts commented-out
// entries and misses double-quoted keys, so counts can legitimately differ and a name-keyed
// comparison verifies parity on the capabilities both backends find.
func TestParseAccessPhp_TreesitterBackendParity_RealFiles(t *testing.T) {
	var checked int
	for _, name := range realMoodleRoots {
		root := filepath.Join("/srv/workspace/www/html/mdle", name)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Base(path) != "access.php" || filepath.Base(filepath.Dir(path)) != "db" {
				return nil
			}
			checked++

			t.Setenv("BUILD82_EXTRACTOR_BACKEND", "")
			regexResult := ParseAccessPhp(path)
			t.Setenv("BUILD82_EXTRACTOR_BACKEND", "treesitter")
			tsResult := ParseAccessPhp(path)

			regexByName := make(map[string]Capability, len(regexResult.Capabilities))
			for _, c := range regexResult.Capabilities {
				regexByName[c.Name] = c
			}
			tsByName := make(map[string]Capability, len(tsResult.Capabilities))
			for _, c := range tsResult.Capabilities {
				tsByName[c.Name] = c
			}

			for name, rc := range regexByName {
				tc, ok := tsByName[name]
				if !ok {
					t.Logf("%s: capability %q found by regex but not treesitter (likely a commented-out entry — KNOWN_DIVERGENCES.md #1)", path, name)
					continue
				}
				assertCapabilityParity(t, path, name, rc, tc)
			}
			for name := range tsByName {
				if _, ok := regexByName[name]; !ok {
					t.Logf("%s: capability %q found by treesitter but not regex (likely a double-quoted key — KNOWN_DIVERGENCES.md #4)", path, name)
				}
			}
			return nil
		})
	}
	if checked == 0 {
		t.Skip("no db/access.php files found across real Moodle installations")
	}
}

// TestGetCapabilityNames_NilInputDoesNotPanic verifies that GetCapabilityNames handles a nil
// *CapabilitiesExtraction without panicking.
func TestGetCapabilityNames_NilInputDoesNotPanic(t *testing.T) {
	if got := GetCapabilityNames(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestGetCapabilityNames_Sorted(t *testing.T) {
	names := GetCapabilityNames(&CapabilitiesExtraction{Capabilities: []Capability{
		{Name: "local/test:z"}, {Name: "local/test:a"},
	}})
	if names[0] != "local/test:a" || names[1] != "local/test:z" {
		t.Errorf("expected sorted names, got %v", names)
	}
}
