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

package tsbackend

import (
	"os"
	"path/filepath"
	"testing"
)

// capabilitiesFixtureWellFormed is a well-formed db/access.php fixture.
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

// writeAccessPhp writes content to a temporary db/access.php and returns its path.
func writeAccessPhp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "access.php")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestParseAccessPhp_MatchesRegexBackendFixture verifies the parsed capabilities for a well-formed access file match the regex backend's output.
func TestParseAccessPhp_MatchesRegexBackendFixture(t *testing.T) {
	path := writeAccessPhp(t, capabilitiesFixtureWellFormed)
	result := ParseAccessPhp(path)
	if result == nil || len(result.Capabilities) != 2 {
		t.Fatalf("expected 2 capabilities, got %+v", result)
	}
	c := result.Capabilities[0]
	if c.Name != "local/test:view" || c.CapType != "read" || c.ContextLevel != "CONTEXT_COURSE" {
		t.Errorf("capability mismatch: %+v", c)
	}
	if c.RiskBitmask != "RISK_PERSONAL" {
		t.Errorf("expected bare-constant riskbitmask, got %q", c.RiskBitmask)
	}
	if c.Archetypes["teacher"] != "CAP_ALLOW" {
		t.Errorf("archetype mismatch: %+v", c.Archetypes)
	}
	if len(c.Archetypes) != 3 {
		t.Errorf("expected 3 archetypes, got %d: %+v", len(c.Archetypes), c.Archetypes)
	}
}

// TestParseAccessPhp_CapTypeDefaultsToRead verifies a capability without captype defaults to "read".
func TestParseAccessPhp_CapTypeDefaultsToRead(t *testing.T) {
	path := writeAccessPhp(t, `<?php
$capabilities = [
    'local/test:noop' => [
        'contextlevel' => CONTEXT_SYSTEM,
    ],
];`)
	result := ParseAccessPhp(path)
	if result == nil || len(result.Capabilities) != 1 || result.Capabilities[0].CapType != "read" {
		t.Fatalf("expected CapType to default to 'read', got %+v", result)
	}
}

// TestParseAccessPhp_KnownDivergence_DoubleQuotedKeyIsMatched verifies that a double-quoted
// capability key is found. The regex backend's capabilityKeyPattern is single-quote-only;
// tree-sitter has no such restriction.
func TestParseAccessPhp_KnownDivergence_DoubleQuotedKeyIsMatched(t *testing.T) {
	path := writeAccessPhp(t, `<?php
$capabilities = [
    "local/test:doublequoted" => [
        'captype' => 'read',
    ],
];`)
	result := ParseAccessPhp(path)
	if result == nil || len(result.Capabilities) != 1 {
		t.Fatalf("expected the tree-sitter backend to find the double-quoted key, got %+v", result)
	}
	if result.Capabilities[0].Name != "local/test:doublequoted" {
		t.Errorf("unexpected capability name: %q", result.Capabilities[0].Name)
	}
}

// TestParseAccessPhp_CompoundRiskBitmask verifies that a compound riskbitmask such as
// `RISK_SPAM | RISK_PERSONAL | RISK_XSS` is returned as the full expression. The regex backend's
// constant fallback only captures the first ALL_CAPS token.
func TestParseAccessPhp_CompoundRiskBitmask(t *testing.T) {
	path := writeAccessPhp(t, `<?php
$capabilities = [
    'local/test:risky' => [
        'riskbitmask' => RISK_SPAM | RISK_PERSONAL | RISK_XSS,
    ],
];`)
	result := ParseAccessPhp(path)
	if result == nil || len(result.Capabilities) != 1 {
		t.Fatalf("expected 1 capability, got %+v", result)
	}
	if got := result.Capabilities[0].RiskBitmask; got != "RISK_SPAM | RISK_PERSONAL | RISK_XSS" {
		t.Errorf("got %q, want the full compound expression", got)
	}
}

// TestParseAccessPhp_EmptyArray verifies a file without a $capabilities array yields an empty, non-nil capability list.
func TestParseAccessPhp_EmptyArray(t *testing.T) {
	path := writeAccessPhp(t, "<?php\n$capabilities = [];\n")
	result := ParseAccessPhp(path)
	if result == nil || len(result.Capabilities) != 0 {
		t.Fatalf("expected empty capabilities, got %+v", result)
	}
}

// TestParseAccessPhp_MissingFile verifies a nonexistent file yields nil.
func TestParseAccessPhp_MissingFile(t *testing.T) {
	if ParseAccessPhp("/nonexistent/db/access.php") != nil {
		t.Error("expected nil for missing file")
	}
}

// TestParseAccessPhp_RealFileRegression runs the tree-sitter backend against every real
// db/access.php across all 4 real Moodle installations.
func TestParseAccessPhp_RealFileRegression(t *testing.T) {
	var found int
	for _, name := range realMoodleRoots {
		root := filepath.Join("/srv/workspace/www/html/mdle", name)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Base(path) != "access.php" || filepath.Base(filepath.Dir(path)) != "db" {
				return nil
			}
			found++
			if result := ParseAccessPhp(path); result == nil {
				t.Errorf("%s: expected a non-nil result", path)
			}
			return nil
		})
	}
	if found == 0 {
		t.Skip("no db/access.php files found across real Moodle installations")
	}
}
