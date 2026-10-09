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

package selfupdate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseContract_DistMatchesSelfUpdate verifies that a release directory produced by
// scripts/release matches what self-update expects. When BUILD82_DIST_DIR names such a directory,
// it must hold one asset per entry of ReleasePlatforms, each named as AssetName produces and listed
// in checksums.txt with its real SHA-256, and checksums.txt must also cover the build82.mcpb bundle.
// The test is skipped when the variable is unset.
func TestReleaseContract_DistMatchesSelfUpdate(t *testing.T) {
	dist := os.Getenv("BUILD82_DIST_DIR")
	if dist == "" {
		t.Skip("BUILD82_DIST_DIR not set")
	}
	checksums, err := os.ReadFile(filepath.Join(dist, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, p := range ReleasePlatforms() {
		known[AssetName(p[0], p[1])] = true
	}
	assets, err := filepath.Glob(filepath.Join(dist, "build82_*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != len(known) {
		t.Errorf("found %d build82_* assets in %s, want one per platform (%d)", len(assets), dist, len(known))
	}
	for _, asset := range append(assets, filepath.Join(dist, "build82.mcpb")) {
		name := filepath.Base(asset)
		if name != "build82.mcpb" && !known[name] {
			t.Errorf("%s is not a name AssetName produces", name)
			continue
		}
		if err := VerifyChecksum(string(checksums), name, asset); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, name := range []string{"server.json"} {
		if _, err := os.Stat(filepath.Join(dist, name)); err != nil {
			t.Errorf("%s missing from %s: %v", name, dist, err)
		}
	}
}

// TestReleaseContract_WorkflowSignsTheBundleSelfUpdateReads verifies that the release workflow
// writes the signature bundle under signatureBundleName, signing checksums.txt, and checks it with
// the exact identity self-update requires.
func TestReleaseContract_WorkflowSignsTheBundleSelfUpdateReads(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(raw)
	for _, want := range []string{
		"cosign sign-blob --yes --bundle " + signatureBundleName + " checksums.txt",
		"cosign verify-blob checksums.txt --bundle " + signatureBundleName,
		`--certificate-identity "https://github.com/${GITHUB_REPOSITORY}/.github/workflows/release.yml@${GITHUB_REF}"`,
		"--certificate-oidc-issuer https://token.actions.githubusercontent.com",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release.yml lacks %q", want)
		}
	}
}
