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
	"path/filepath"
	"testing"
)

func TestExtractSubplugins_ModernJson(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "subplugins.json"), `{
  "plugintypes": {
    "workshopform": "mod/workshop/form",
    "workshopallocation": "mod/workshop/allocation"
  }
}`)

	got := ExtractSubplugins(dir)
	if len(got) != 2 {
		t.Fatalf("expected 2 subplugin types, got %d: %+v", len(got), got)
	}
	// Sorted by Type: "workshopallocation" < "workshopform".
	if got[0] != (Subplugin{Type: "workshopallocation", Path: "mod/workshop/allocation"}) {
		t.Errorf("unexpected first entry: %+v", got[0])
	}
	if got[1] != (Subplugin{Type: "workshopform", Path: "mod/workshop/form"}) {
		t.Errorf("unexpected second entry: %+v", got[1])
	}
}

// TestExtractSubplugins_MergesSubplugintypesKey confirms the "subplugintypes" key (used alongside
// the older "plugintypes" key) is also read.
func TestExtractSubplugins_MergesSubplugintypesKey(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "subplugins.json"), `{
  "plugintypes": {"workshopform": "mod/workshop/form"},
  "subplugintypes": {"workshopform": "mod/workshop/form", "workshopeval": "mod/workshop/eval"}
}`)

	got := ExtractSubplugins(dir)
	if len(got) != 2 {
		t.Fatalf("expected 2 merged subplugin types, got %d: %+v", len(got), got)
	}
}

func TestExtractSubplugins_LegacyPhpFallback(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "subplugins.php"), `<?php
$subplugins = array(
    'workshopeval' => 'mod/workshop/eval',
);
`)

	got := ExtractSubplugins(dir)
	if len(got) != 1 || got[0] != (Subplugin{Type: "workshopeval", Path: "mod/workshop/eval"}) {
		t.Fatalf("expected the legacy declaration, got %+v", got)
	}
}

// TestExtractSubplugins_JsonTakesPrecedenceOverLegacy confirms subplugins.json is authoritative
// when both files are present.
func TestExtractSubplugins_JsonTakesPrecedenceOverLegacy(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "db"))
	mustWriteFile(t, filepath.Join(dir, "db", "subplugins.json"), `{"plugintypes": {"fromjson": "mod/workshop/json"}}`)
	mustWriteFile(t, filepath.Join(dir, "db", "subplugins.php"), `<?php
$subplugins = array('fromphp' => 'mod/workshop/php');
`)

	got := ExtractSubplugins(dir)
	if len(got) != 1 || got[0].Type != "fromjson" {
		t.Fatalf("expected only the JSON declaration, got %+v", got)
	}
}

func TestExtractSubplugins_NoDeclarationReturnsNil(t *testing.T) {
	dir := t.TempDir()
	if got := ExtractSubplugins(dir); got != nil {
		t.Errorf("expected nil for a plugin with no subplugin declaration, got %+v", got)
	}
}
