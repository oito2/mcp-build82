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
	"regexp"
	"strings"
	"testing"
)

// readRepoFile returns the content of the file at the slash-separated path `rel`, relative to the
// repository root (two levels above this package).
func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// jobHeaderPattern matches the header line of a job in a workflow's "jobs:" map (two-space indent).
var jobHeaderPattern = regexp.MustCompile(`(?m)^  ([A-Za-z0-9_-]+):\s*$`)

// workflowJob returns the text of the job `name` of `workflow`, from its header line up to the next
// job header or the end of the file. It fails the test when the job is missing.
func workflowJob(t *testing.T, workflow, name string) string {
	t.Helper()
	jobsAt := strings.Index(workflow, "\njobs:\n")
	if jobsAt < 0 {
		t.Fatal("workflow has no jobs map")
	}
	body := workflow[jobsAt:]
	headers := jobHeaderPattern.FindAllStringSubmatchIndex(body, -1)
	for i, h := range headers {
		if body[h[2]:h[3]] != name {
			continue
		}
		end := len(body)
		if i+1 < len(headers) {
			end = headers[i+1][0]
		}
		return body[h[0]:end]
	}
	t.Fatalf("workflow has no job %q", name)
	return ""
}

// requireContains fails the test for every entry of `wants` missing from `text`, naming `what`.
func requireContains(t *testing.T, what, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("%s lacks %q", what, want)
		}
	}
}

// TestWorkflows_ReleaseRunsTheCIChecks verifies that CI and the release run the same reusable check
// workflow, the release without the Go cache, and that the release workflow no longer carries its
// own copy of the checks.
func TestWorkflows_ReleaseRunsTheCIChecks(t *testing.T) {
	ci := readRepoFile(t, ".github/workflows/ci.yml")
	release := readRepoFile(t, ".github/workflows/release.yml")
	checks := readRepoFile(t, ".github/workflows/checks.yml")

	requireContains(t, "ci.yml checks job", workflowJob(t, ci, "checks"), "uses: ./.github/workflows/checks.yml")
	requireContains(t, "release.yml checks job", workflowJob(t, release, "checks"),
		"needs: validate", "uses: ./.github/workflows/checks.yml", "go-cache: false")
	requireContains(t, "checks.yml", checks, "workflow_call:", "COVERAGE_FLOOR:", "go mod tidy -diff",
		"actionlint", "go run ./scripts/linkcheck", "govulncheck", "BUILD82_EXTRACTOR_BACKEND: treesitter",
		`TMPDIR="$RUNNER_TEMP/link-tmp"`)
	for _, wf := range map[string]string{"ci.yml": ci, "release.yml": release} {
		if strings.Contains(wf, "go test -race") {
			t.Error("a test run is defined outside checks.yml")
		}
	}
}

// TestWorkflows_ReleaseRequiresTagOnMainAndEnvironment verifies that the release fails for a tag
// whose commit is not on main, and that the signing job runs in the protected "release"
// environment.
func TestWorkflows_ReleaseRequiresTagOnMainAndEnvironment(t *testing.T) {
	release := readRepoFile(t, ".github/workflows/release.yml")
	requireContains(t, "release.yml validate job", workflowJob(t, release, "validate"),
		"fetch-depth: 0", `git merge-base --is-ancestor "${GITHUB_SHA}" origin/main`)
	requireContains(t, "release.yml sign-publish job", workflowJob(t, release, "sign-publish"),
		"environment: release")
}

// TestWorkflows_BuildUploadsBeforeThirdPartyValidators verifies that the release build packages
// with the shared action and uploads dist/ before the validators run, and that CI's
// release-contract job uses the same two actions.
func TestWorkflows_BuildUploadsBeforeThirdPartyValidators(t *testing.T) {
	build := workflowJob(t, readRepoFile(t, ".github/workflows/release.yml"), "build")
	pkg := strings.Index(build, "uses: ./.github/actions/package-release")
	upload := strings.Index(build, "uses: actions/upload-artifact@")
	validate := strings.Index(build, "uses: ./.github/actions/validate-release")
	if pkg < 0 || upload < 0 || validate < 0 || pkg >= upload || upload >= validate {
		t.Errorf("build job must package, then upload, then validate (offsets %d, %d, %d)", pkg, upload, validate)
	}
	if strings.Contains(build, "install-mcp-publisher") || strings.Contains(build, `"$RUNNER_TEMP/mcp-publisher"`) {
		t.Error("build job runs mcp-publisher outside the validate-release action")
	}
	requireContains(t, "ci.yml release-contract job", workflowJob(t, readRepoFile(t, ".github/workflows/ci.yml"), "release-contract"),
		"uses: ./.github/actions/package-release", "uses: ./.github/actions/validate-release")
}

// TestWorkflows_ValidatorsSeeOnlyCopies verifies that the validate-release action checks that
// server.json describes the bundle, and runs both third-party validators (the MCP Registry one and
// the pinned MCPB one) on copies outside dist/.
func TestWorkflows_ValidatorsSeeOnlyCopies(t *testing.T) {
	action := readRepoFile(t, ".github/actions/validate-release/action.yml")
	requireContains(t, "validate-release", action,
		`jq -r '.packages[0].fileSha256' server.json`,
		`cd "$RUNNER_TEMP/validate-release" && "$RUNNER_TEMP/mcp-publisher" validate`,
		`cd "$RUNNER_TEMP/validate-release" && npx --yes "@anthropic-ai/mcpb@${MCPB_VERSION}" validate mcpb/manifest.json`)
	if !regexp.MustCompile(`MCPB_VERSION: \d+\.\d+\.\d+\n`).MatchString(action) {
		t.Error("validate-release does not pin MCPB_VERSION to an exact version")
	}
}

// TestWorkflows_RegistryPublishesTheVerifiedArtifact verifies that the registry job publishes the
// server.json of the digest-checked workflow artifact, after checking it against checksums.txt and
// the tag, and never a copy downloaded from the mutable release.
func TestWorkflows_RegistryPublishesTheVerifiedArtifact(t *testing.T) {
	job := workflowJob(t, readRepoFile(t, ".github/workflows/release.yml"), "publish-registry")
	if strings.Contains(job, "gh release download") {
		t.Error("publish-registry downloads server.json from the release")
	}
	requireContains(t, "release.yml publish-registry job", job,
		"uses: actions/download-artifact@", "sha256sum --check --strict checksums.txt",
		`jq -r '.packages[0].fileSha256' server.json`, `"${GITHUB_REF_NAME#v}"`)
}
