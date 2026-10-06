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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oito2/mcp-build82/internal/version"
)

// TestAssetName verifies the asset filename for each OS and architecture.
func TestAssetName(t *testing.T) {
	tests := []struct {
		goos, goarch, want string
	}{
		{"linux", "amd64", "build82_linux_amd64"},
		{"darwin", "arm64", "build82_darwin_arm64"},
		{"windows", "amd64", "build82_windows_amd64.exe"},
	}
	for _, tt := range tests {
		if got := AssetName(tt.goos, tt.goarch); got != tt.want {
			t.Errorf("AssetName(%s, %s) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
		}
	}
}

// TestIsNewer verifies version comparison, including dev builds and unparsable tags.
func TestIsNewer(t *testing.T) {
	tests := []struct {
		name, current, latest string
		want                  bool
	}{
		{"patch bump", "v0.1.0", "v0.1.1", true},
		{"minor bump", "v0.1.0", "v0.2.0", true},
		{"major bump", "v0.1.0", "v1.0.0", true},
		{"same version", "v0.1.0", "v0.1.0", false},
		{"latest older", "v0.2.0", "v0.1.0", false},
		{"dev build always outdated", "dev", "v0.1.0", true},
		{"unparsable latest never triggers update", "v0.1.0", "not-a-version", false},
		{"no v prefix on either side", "0.1.0", "0.2.0", true},
		{"release newer than its rc", "v1.2.3-rc1", "v1.2.3", true},
		{"rc not newer than release", "v1.2.3", "v1.2.3-rc1", false},
		{"same rc", "v1.2.3-rc1", "v1.2.3-rc1", false},
		{"rc2 newer than rc1", "v1.2.3-rc.1", "v1.2.3-rc.2", true},
		{"numeric ids compare numerically", "v1.0.0-rc.2", "v1.0.0-rc.10", true},
		{"alphanumeric above numeric", "v1.0.0-1", "v1.0.0-alpha", true},
		{"alpha before beta", "v1.0.0-beta", "v1.0.0-alpha", false},
		{"longer pre-release is newer", "v1.0.0-alpha", "v1.0.0-alpha.1", true},
		{"build metadata ignored", "v1.0.0+a", "v1.0.0+b", false},
		{"next patch rc newer than prior release", "v1.2.3", "v1.2.4-rc1", true},
		{"empty pre-release id invalid", "v1.0.0", "v1.0.1-", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNewer(tt.current, tt.latest); got != tt.want {
				t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

// TestFetchLatestRelease_ReturnsRelease verifies that a 200 response is decoded into a Release.
func TestFetchLatestRelease_ReturnsRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/oito2/mcp-build82/releases/latest" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"tag_name":"v0.1.0","assets":[{"name":"build82_linux_amd64","browser_download_url":"http://example/asset"}]}`)
	}))
	defer srv.Close()

	rel, err := FetchLatestRelease(http.DefaultClient, srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel == nil || rel.TagName != "v0.1.0" {
		t.Fatalf("unexpected release: %+v", rel)
	}
	asset, ok := rel.FindAsset("build82_linux_amd64")
	if !ok || asset.BrowserDownloadURL != "http://example/asset" {
		t.Errorf("unexpected asset lookup: %+v, ok=%v", asset, ok)
	}
}

// TestFetchLatestRelease_NoReleases exercises the "possibly-empty releases list" path: GitHub
// returns 404 when a repo has no releases yet, and that must be reported as "no releases found",
// not an error.
func TestFetchLatestRelease_NoReleases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Not Found"}`)
	}))
	defer srv.Close()

	rel, err := FetchLatestRelease(http.DefaultClient, srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel != nil {
		t.Errorf("expected nil release for a 404, got %+v", rel)
	}
}

// TestFetchLatestRelease_SuccessBodyIsCapped verifies that the success-path response body read is
// bounded. maxReleaseResponseSize is shrunk so the response is truncated well before the JSON
// object closes, which must fail the decode in a controlled way (a JSON syntax/EOF error) rather
// than reading an unbounded amount of data into memory.
func TestFetchLatestRelease_SuccessBodyIsCapped(t *testing.T) {
	original := maxReleaseResponseSize
	maxReleaseResponseSize = 10
	defer func() { maxReleaseResponseSize = original }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"tag_name":"v1.0.0","assets":[{"name":"build82_linux_amd64","browser_download_url":"https://example/asset"}]}`)
	}))
	defer srv.Close()

	rel, err := FetchLatestRelease(http.DefaultClient, srv.URL)
	if err == nil {
		t.Fatalf("expected the truncated body to fail JSON decoding, got release: %+v", rel)
	}
	if !strings.Contains(err.Error(), "decode release response") {
		t.Errorf("expected a decode error, got: %v", err)
	}
}

// TestFetchLatestRelease_ErrorBodyIsCapped verifies that the error body read on an unexpected
// status is bounded.
func TestFetchLatestRelease_ErrorBodyIsCapped(t *testing.T) {
	original := maxReleaseResponseSize
	maxReleaseResponseSize = 10
	defer func() { maxReleaseResponseSize = original }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, strings.Repeat("x", 1000))
	}))
	defer srv.Close()

	_, err := FetchLatestRelease(http.DefaultClient, srv.URL)
	if err == nil {
		t.Fatal("expected an error for the 500 status")
	}
	// The error message must only ever contain the capped portion of the body (well under the
	// full 1000 bytes served), confirming the read was bounded.
	if len(err.Error()) > 200 {
		t.Errorf("expected the error message's body excerpt to be capped, got %d chars: %s", len(err.Error()), err.Error())
	}
}

// TestDownloadClient_HasLongerTimeoutThanMetadataClient verifies that downloadClient has a
// materially longer timeout than httpClient's 30s (a release-asset download of up to maxAssetSize
// = 200 MiB can take longer on a slow connection), while still enforcing the same https-only
// CheckRedirect policy as httpClient.
func TestDownloadClient_HasLongerTimeoutThanMetadataClient(t *testing.T) {
	if downloadClient.Timeout <= httpClient.Timeout {
		t.Errorf("expected downloadClient.Timeout (%s) to be greater than httpClient.Timeout (%s)", downloadClient.Timeout, httpClient.Timeout)
	}
	if downloadClient.CheckRedirect == nil {
		t.Error("expected downloadClient to enforce a CheckRedirect policy, got nil")
	}
}

// sha256Hex returns the hex-encoded SHA-256 digest of `data`.
func sha256Hex(t *testing.T, data []byte) string {
	t.Helper()
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// TestVerifyChecksum_MatchAndMismatch verifies that a matching digest passes and a modified file is rejected.
func TestVerifyChecksum_MatchAndMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "build82_linux_amd64")
	content := []byte("fake binary contents")
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256Hex(t, content)
	checksums := sum + "  build82_linux_amd64\n" + sha256Hex(t, []byte("other")) + "  build82_darwin_arm64\n"

	if err := VerifyChecksum(checksums, "build82_linux_amd64", path); err != nil {
		t.Errorf("expected checksum to match: %v", err)
	}

	// Corrupt the file after the checksum was computed — must refuse, not silently accept.
	if err := os.WriteFile(path, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := VerifyChecksum(checksums, "build82_linux_amd64", path); err == nil {
		t.Error("expected a checksum mismatch error, got nil")
	}
}

// TestVerifyChecksum_NoEntryForAsset verifies that a missing checksum entry is an error.
func TestVerifyChecksum_NoEntryForAsset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bin")
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := VerifyChecksum("deadbeef  some-other-asset\n", "build82_linux_amd64", path)
	if err == nil {
		t.Error("expected an error when no checksum entry matches the asset name")
	}
}

// buildFakeBinary compiles a tiny standalone Go program into dir that responds to --version, to
// stand in for a downloaded release asset without touching the network.
func buildFakeBinary(t *testing.T, dir, name, versionOutput string) string {
	t.Helper()
	srcDir := t.TempDir()
	src := fmt.Sprintf(`package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Print(%q)
		return
	}
	os.Exit(3)
}
`, versionOutput)
	srcFile := filepath.Join(srcDir, "main.go")
	if err := os.WriteFile(srcFile, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", out, srcFile)
	cmd.Dir = srcDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build fake binary: %v\n%s", err, output)
	}
	return out
}

// TestSmokeTest_RealBinary verifies that SmokeTest returns the version printed by a working binary.
func TestSmokeTest_RealBinary(t *testing.T) {
	dir := t.TempDir()
	bin := buildFakeBinary(t, dir, "fake-good", "v9.9.9\n")

	got, err := SmokeTest(bin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v9.9.9" {
		t.Errorf("got %q, want %q", got, "v9.9.9")
	}
}

// TestSmokeTest_FailingBinary verifies that SmokeTest fails for a binary that exits non-zero.
func TestSmokeTest_FailingBinary(t *testing.T) {
	dir := t.TempDir()
	bin := buildFakeBinary(t, dir, "fake-bad", "")

	if _, err := SmokeTest(bin); err == nil {
		t.Error("expected an error for a binary that produces no version output")
	}
}

// buildHangingBinary compiles a tiny standalone Go program that sleeps for sleepFor before ever
// returning, regardless of its arguments — a stand-in for a hung/broken downloaded release
// binary, to exercise SmokeTest's timeout without depending on any real hang condition.
func buildHangingBinary(t *testing.T, dir, name string, sleepFor time.Duration) string {
	t.Helper()
	srcDir := t.TempDir()
	src := fmt.Sprintf(`package main

import "time"

func main() {
	time.Sleep(%d * time.Millisecond)
}
`, sleepFor.Milliseconds())
	srcFile := filepath.Join(srcDir, "main.go")
	if err := os.WriteFile(srcFile, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", out, srcFile)
	cmd.Dir = srcDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build hanging binary: %v\n%s", err, output)
	}
	return out
}

// TestSmokeTest_TimesOutOnHungBinary verifies that SmokeTest returns an error when the binary
// hangs instead of blocking forever. smokeTestTimeout is shrunk so the test doesn't have to wait
// out a real 10s timeout.
func TestSmokeTest_TimesOutOnHungBinary(t *testing.T) {
	original := smokeTestTimeout
	smokeTestTimeout = 200 * time.Millisecond
	defer func() { smokeTestTimeout = original }()

	dir := t.TempDir()
	bin := buildHangingBinary(t, dir, "fake-hung", 5*time.Second)

	start := time.Now()
	_, err := SmokeTest(bin)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error for a binary that never returns --version output")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected a timeout error, got: %v", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("SmokeTest took %s to return, expected it bounded by the (shrunk) timeout instead of the binary's full sleep", elapsed)
	}
}

// TestAtomicReplace_Success exercises the rename/backup sequence against two real local binaries
// (no network involved).
func TestAtomicReplace_Success(t *testing.T) {
	dir := t.TempDir()
	current := buildFakeBinary(t, dir, "build82", "v0.1.0\n")
	newBin := buildFakeBinary(t, filepath.Join(dir, "staging"), "build82-new", "v0.2.0\n")

	backupPath, err := AtomicReplace(current, newBin, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backupPath != current+".bak" {
		t.Errorf("unexpected backup path: %s", backupPath)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("expected backup to exist: %v", err)
	}
	if _, err := os.Stat(current); err != nil {
		t.Errorf("expected new binary to be in place at the original path: %v", err)
	}

	out, err := exec.Command(current, "--version").Output()
	if err != nil {
		t.Fatalf("unexpected error running replaced binary: %v", err)
	}
	if strings.TrimSpace(string(out)) != "v0.2.0" {
		t.Errorf("replaced binary reports %q, want v0.2.0", strings.TrimSpace(string(out)))
	}
}

// TestAtomicReplace_RefusesOnFailedSmokeTest verifies that when the smoke test fails, the original
// binary is left completely untouched.
func TestAtomicReplace_RefusesOnFailedSmokeTest(t *testing.T) {
	dir := t.TempDir()
	current := buildFakeBinary(t, dir, "build82", "v0.1.0\n")
	broken := buildFakeBinary(t, filepath.Join(dir, "staging"), "build82-broken", "") // exits 3, no output

	_, err := AtomicReplace(current, broken, "")
	if err == nil {
		t.Fatal("expected an error from a failing smoke test")
	}

	if _, err := os.Stat(current + ".bak"); err == nil {
		t.Error("original binary must not have been backed up when the smoke test fails")
	}
	out, verErr := exec.Command(current, "--version").Output()
	if verErr != nil {
		t.Fatalf("original binary must still be runnable: %v", verErr)
	}
	if strings.TrimSpace(string(out)) != "v0.1.0" {
		t.Errorf("original binary was altered: got %q", strings.TrimSpace(string(out)))
	}
}

// TestAtomicReplace_DoubleRenameFailureSurfacesBothErrors covers the scenario where the second
// rename (new binary into place) fails AND the best-effort restore ALSO fails, leaving
// currentBinaryPath missing: both errors must be reported. Failures are injected via the osRename
// seam.
func TestAtomicReplace_DoubleRenameFailureSurfacesBothErrors(t *testing.T) {
	dir := t.TempDir()
	current := buildFakeBinary(t, dir, "build82", "v0.1.0\n")
	newBin := buildFakeBinary(t, filepath.Join(dir, "staging"), "build82-new", "v0.2.0\n")

	origRename := osRename
	defer func() { osRename = origRename }()

	moveErr := errors.New("simulated move failure")
	restoreErr := errors.New("simulated restore failure")
	callCount := 0
	osRename = func(oldpath, newpath string) error {
		callCount++
		switch callCount {
		case 1:
			return origRename(oldpath, newpath) // real backup rename succeeds
		case 2:
			return moveErr // simulate "move new binary into place" failing
		case 3:
			return restoreErr // simulate the best-effort restore ALSO failing
		default:
			t.Fatalf("unexpected extra osRename call #%d", callCount)
			return nil
		}
	}

	_, err := AtomicReplace(current, newBin, "")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), moveErr.Error()) {
		t.Errorf("expected the original move error surfaced, got: %v", err)
	}
	if !strings.Contains(err.Error(), restoreErr.Error()) {
		t.Errorf("expected the restore failure to ALSO be surfaced (not silently swallowed), got: %v", err)
	}
	if !strings.Contains(err.Error(), "MISSING") {
		t.Errorf("expected an explicit warning that the binary is now missing, got: %v", err)
	}
	if _, statErr := os.Stat(current); statErr == nil {
		t.Error("expected currentBinaryPath to be genuinely missing after both renames failed")
	}
}

// TestAtomicReplace_SingleRenameFailureStillRestoresSuccessfully verifies the case where the
// restore succeeds: the caller gets back the plain original error, with no "MISSING" warning, and
// the original binary is runnable again.
func TestAtomicReplace_SingleRenameFailureStillRestoresSuccessfully(t *testing.T) {
	dir := t.TempDir()
	current := buildFakeBinary(t, dir, "build82", "v0.1.0\n")
	newBin := buildFakeBinary(t, filepath.Join(dir, "staging"), "build82-new", "v0.2.0\n")

	origRename := osRename
	defer func() { osRename = origRename }()

	moveErr := errors.New("simulated move failure")
	callCount := 0
	osRename = func(oldpath, newpath string) error {
		callCount++
		if callCount == 2 {
			return moveErr
		}
		return origRename(oldpath, newpath)
	}

	_, err := AtomicReplace(current, newBin, "")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), moveErr.Error()) {
		t.Errorf("expected the move error surfaced, got: %v", err)
	}
	if strings.Contains(err.Error(), "MISSING") {
		t.Errorf("expected no MISSING warning when restore succeeds, got: %v", err)
	}

	out, verErr := exec.Command(current, "--version").Output()
	if verErr != nil {
		t.Fatalf("expected the original binary restored and runnable: %v", verErr)
	}
	if strings.TrimSpace(string(out)) != "v0.1.0" {
		t.Errorf("expected original binary content restored, got %q", strings.TrimSpace(string(out)))
	}
}

// TestRun_RejectsUnsupportedChannel verifies that Run returns an error for a channel other than
// "stable" (e.g. `build82 self-update --channel beta`).
func TestRun_RejectsUnsupportedChannel(t *testing.T) {
	err := Run(RunOptions{Channel: "beta"})
	if err == nil {
		t.Fatal("expected an error for an unsupported channel")
	}
	if !strings.Contains(err.Error(), `unsupported channel "beta"`) {
		t.Errorf("expected the error to name the unsupported channel, got: %v", err)
	}
}

// TestRun_EmptyAndStableChannelAreBothAccepted verifies that both the zero-value "" and the
// explicit "stable" channel are accepted.
func TestRun_EmptyAndStableChannelAreBothAccepted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	for _, channel := range []string{"", "stable"} {
		var out strings.Builder
		err := Run(RunOptions{Channel: channel, APIBaseURL: srv.URL, Stdout: &out})
		if err != nil {
			t.Errorf("channel %q: expected no error, got: %v", channel, err)
		}
	}
}

// TestRun_CheckReportsUpdateAvailable verifies that Run in check mode reports a newer release without downloading.
func TestRun_CheckReportsUpdateAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"tag_name":"v99.0.0","assets":[]}`)
	}))
	defer srv.Close()

	var out strings.Builder
	err := Run(RunOptions{Check: true, APIBaseURL: srv.URL, Stdout: &out})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "A new version is available: v99.0.0") {
		t.Errorf("unexpected output: %s", out.String())
	}
}

// TestRun_CheckAgainstEmptyReleaseList verifies that Run reports no releases when none exist.
func TestRun_CheckAgainstEmptyReleaseList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	var out strings.Builder
	if err := Run(RunOptions{Check: true, APIBaseURL: srv.URL, Stdout: &out}); err != nil {
		t.Fatalf("unexpected error against an empty release list: %v", err)
	}
	if !strings.Contains(out.String(), "No releases found.") {
		t.Errorf("unexpected output: %s", out.String())
	}
}

// TestRun_AlreadyLatestSkipsDownload verifies that Run does not download when already on the latest version.
func TestRun_AlreadyLatestSkipsDownload(t *testing.T) {
	original := version.Current
	version.Current = "v1.0.0"
	defer func() { version.Current = original }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"tag_name":"v1.0.0","assets":[]}`)
	}))
	defer srv.Close()

	var out strings.Builder
	if err := Run(RunOptions{APIBaseURL: srv.URL, Stdout: &out}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "Already on the latest version") {
		t.Errorf("unexpected output: %s", out.String())
	}
}

// TestDownloadToTemp_ExceedsMaxSizeIsRejected verifies that downloadToTemp rejects a response
// larger than maxAssetSize. maxAssetSize is temporarily shrunk so the test doesn't need to serve a
// multi-hundred-MB response.
func TestDownloadToTemp_ExceedsMaxSizeIsRejected(t *testing.T) {
	original := maxAssetSize
	maxAssetSize = 10
	defer func() { maxAssetSize = original }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "this response body is well over ten bytes long")
	}))
	defer srv.Close()

	dir := t.TempDir()
	path, err := downloadToTemp(http.DefaultClient, srv.URL, dir, "oversized-*")
	if err == nil {
		t.Fatalf("expected an error for a response exceeding maxAssetSize, got a file at %s", path)
	}
	if !strings.Contains(err.Error(), "exceeded max size") {
		t.Errorf("expected a max-size error, got: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("expected the oversized temp file to be removed, found: %v", entries)
	}
}

// TestValidateAssetURL verifies that asset URLs are accepted only when the scheme is https and the
// host is github.com or a *.githubusercontent.com subdomain.
func TestValidateAssetURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"valid https github.com release asset", "https://github.com/oito2/mcp-build82/releases/download/v1.0.0/build82_linux_amd64", false},
		{"valid https objects.githubusercontent.com redirect target", "https://objects.githubusercontent.com/github-production-release-asset-2e65be/1/abc123", false},
		{"http is rejected", "http://github.com/oito2/mcp-build82/releases/download/v1.0.0/build82_linux_amd64", true},
		{"arbitrary host is rejected", "https://evil.example.com/build82_linux_amd64", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAssetURL(tt.raw)
			if tt.wantErr && err == nil {
				t.Errorf("validateAssetURL(%q): expected an error, got nil", tt.raw)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateAssetURL(%q): expected no error, got: %v", tt.raw, err)
			}
		})
	}
}

// TestFetchLatestRelease_SendsUserAgent verifies that FetchLatestRelease sends an explicit
// User-Agent header.
func TestFetchLatestRelease_SendsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := FetchLatestRelease(http.DefaultClient, srv.URL); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "build82/" + version.Current
	if gotUA != want {
		t.Errorf("expected User-Agent %q, got %q", want, gotUA)
	}
}

// TestDownloadToTemp_SendsUserAgent verifies that downloadToTemp sends an explicit User-Agent
// header.
func TestDownloadToTemp_SendsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		fmt.Fprint(w, "body")
	}))
	defer srv.Close()

	dir := t.TempDir()
	if _, err := downloadToTemp(http.DefaultClient, srv.URL, dir, "ua-test-*"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "build82/" + version.Current
	if gotUA != want {
		t.Errorf("expected User-Agent %q, got %q", want, gotUA)
	}
}

// TestRollback_NoBackupReturnsError covers the "nothing to roll back" case:
// binaryPath exists but binaryPath+".bak" doesn't — must return a clear, actionable error rather
// than an opaque os.Stat failure.
func TestRollback_NoBackupReturnsError(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "build82")
	if err := os.WriteFile(bin, []byte("current binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := Rollback(bin)
	if err == nil {
		t.Fatal("expected an error when no .bak file exists")
	}
	if !strings.Contains(err.Error(), "no backup found") {
		t.Errorf("expected a 'no backup found' error, got: %v", err)
	}
}

// TestRollback_PromotesBackupSuccessfully verifies the success path: a real .bak (built as a
// working fake binary, so SmokeTest also genuinely passes) is renamed back into place, and the
// restored file is the backup's actual content, not just any file happening to exist at that path.
func TestRollback_PromotesBackupSuccessfully(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "build82")
	if err := os.WriteFile(current, []byte("broken placeholder"), 0o755); err != nil {
		t.Fatal(err)
	}
	// current + ".bak" == filepath.Join(dir, "build82.bak"), exactly what AtomicReplace would have
	// produced for a binary named "build82" in dir.
	buildFakeBinary(t, dir, "build82.bak", "v0.1.0\n")

	if err := Rollback(current); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(current + ".bak"); err == nil {
		t.Error("expected the .bak file to be gone after being promoted back into place")
	}

	out, err := exec.Command(current, "--version").Output()
	if err != nil {
		t.Fatalf("expected the restored binary to be runnable: %v", err)
	}
	if strings.TrimSpace(string(out)) != "v0.1.0" {
		t.Errorf("restored binary reports %q, want v0.1.0 (the backup's content)", strings.TrimSpace(string(out)))
	}
}

// TestRollback_RenameFailureIsSurfaced verifies that Rollback goes through the osRename seam (like
// AtomicReplace) and wraps a rename failure with enough context to act on.
func TestRollback_RenameFailureIsSurfaced(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "build82")
	if err := os.WriteFile(current, []byte("current binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	buildFakeBinary(t, dir, "build82.bak", "v0.1.0\n")

	origRename := osRename
	defer func() { osRename = origRename }()
	renameErr := errors.New("simulated rename failure")
	osRename = func(oldpath, newpath string) error { return renameErr }

	err := Rollback(current)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), renameErr.Error()) {
		t.Errorf("expected the underlying rename error surfaced, got: %v", err)
	}
}

// TestRollback_SmokeTestFailureDoesNotUndoRename verifies that if the restored (.bak) binary fails
// the post-rollback smoke test, Rollback returns an error but the rename is not undone.
func TestRollback_SmokeTestFailureDoesNotUndoRename(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "build82")
	if err := os.WriteFile(current, []byte("current binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	buildFakeBinary(t, dir, "build82.bak", "") // exits 3, no --version output -> fails SmokeTest

	err := Rollback(current)
	if err == nil {
		t.Fatal("expected an error when the restored binary fails its smoke test")
	}
	if !strings.Contains(err.Error(), "smoke test") {
		t.Errorf("expected a smoke-test-failure error, got: %v", err)
	}
	if _, statErr := os.Stat(current + ".bak"); statErr == nil {
		t.Error("expected the rename to have already happened (.bak consumed), not undone")
	}
	if _, statErr := os.Stat(current); statErr != nil {
		t.Errorf("expected the restored (if broken) binary to still be in place at %s: %v", current, statErr)
	}
}

// TestDownloadToTemp_WithinMaxSizeSucceeds verifies that a normal-sized download is not rejected by the cap.
func TestDownloadToTemp_WithinMaxSizeSucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "small body")
	}))
	defer srv.Close()

	dir := t.TempDir()
	path, err := downloadToTemp(http.DefaultClient, srv.URL, dir, "normal-*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil || string(content) != "small body" {
		t.Errorf("unexpected downloaded content: %q (err=%v)", content, readErr)
	}
}

// TestAtomicReplace_RefusesWrongVersion verifies that a new binary reporting a version other than
// the expected release leaves the original untouched, and that the matching version is accepted.
func TestAtomicReplace_RefusesWrongVersion(t *testing.T) {
	dir := t.TempDir()
	current := buildFakeBinary(t, dir, "build82", "v0.1.0\n")
	newBin := buildFakeBinary(t, filepath.Join(dir, "staging"), "build82-new", "v0.2.0\n")

	if _, err := AtomicReplace(current, newBin, "v0.3.0"); err == nil {
		t.Fatal("expected an error for a version mismatch")
	}
	if got, err := SmokeTest(current); err != nil || got != "v0.1.0" {
		t.Errorf("original binary should be untouched, got %q, %v", got, err)
	}
	if _, err := AtomicReplace(current, newBin, "v0.2.0"); err != nil {
		t.Fatalf("matching version should be accepted: %v", err)
	}
}
