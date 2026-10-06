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
	"slices"
	"strings"
	"testing"
)

// TestSemverTagPattern_ValidatesStrictly verifies that semverTagPattern accepts only strict
// "vMAJOR.MINOR.PATCH" tags and rejects values that could inject extra linker flags.
func TestSemverTagPattern_ValidatesStrictly(t *testing.T) {
	valid := []string{"v0.1.0", "v1.2.3", "v10.20.30"}
	for _, v := range valid {
		if !semverTagPattern.MatchString(v) {
			t.Errorf("expected %q to be accepted as a valid semver tag", v)
		}
	}

	invalid := []string{
		"0.1.0",              // missing v prefix
		"v1.2",               // missing patch
		"v1.2.3-beta",        // pre-release suffix not supported
		"v1.2.3.4",           // extra component
		"v1.2.3 extra flags", // command-injection shape: space-separated tokens appended to a tag
		`v1.2.3" -X foo=bar`, // quote-breakout shape
		"vX.Y.Z",
		"",
	}
	for _, v := range invalid {
		if semverTagPattern.MatchString(v) {
			t.Errorf("expected %q to be rejected", v)
		}
	}
}

// TestReleaseLDFlags_StripsAndStampsVersion verifies that the release linker flags strip the symbol
// table and DWARF info (-s -w) and stamp the version into internal/version.Current.
func TestReleaseLDFlags_StripsAndStampsVersion(t *testing.T) {
	got := strings.Fields(releaseLDFlags("v1.2.3"))
	want := []string{"-s", "-w", "-X", module + "/internal/version.Current=v1.2.3"}
	if !slices.Equal(got, want) {
		t.Errorf("releaseLDFlags = %q, want %q", got, want)
	}
}
