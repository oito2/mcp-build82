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

package selfupdate

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/oito2/mcp-build82/internal/binpath"
	"github.com/oito2/mcp-build82/internal/version"
)

const (
	defaultAPIBaseURL = "https://api.github.com"
	repoOwner         = "oito2"
	repoName          = "mcp-build82"
)

// maxAssetSize caps a single downloaded release asset (binary or checksums.txt). Without a limit,
// a slow/hung or compromised/unexpected server response could block indefinitely or fill the disk
// before the checksum verification step ever runs. A package var, not a const, so tests can shrink
// it temporarily instead of needing to actually serve a multi-hundred-MB response.
var maxAssetSize int64 = 200 << 20 // 200 MiB — generous headroom over any real build82 binary

// maxReleaseResponseSize caps how much of a GitHub Releases API response FetchLatestRelease will
// ever read into memory — both the JSON metadata decode on success and the error body read on an
// unexpected status. Release metadata is at most a few KB; 1 MiB is generous headroom while still
// closing off unbounded-read exposure to a slow/hung or unexpectedly huge response. A package var,
// not a const, so tests can shrink it instead of needing to actually serve a multi-hundred-KB body.
var maxReleaseResponseSize int64 = 1 << 20 // 1 MiB

// httpClient is used for every network call self-update makes (release metadata + asset
// downloads). A bare http.Client (the zero value / http.DefaultClient) has no Timeout, so a
// hung connection would block `build82 self-update` indefinitely with no way to cancel it.
// CheckRedirect refuses any redirect hop that downgrades to plain http — otherwise a
// compromised/MITM'd intermediate could force the download of a binary or checksums.txt over an
// unencrypted connection even though the initial URL was validated as https (see
// validateAssetURL).
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("refusing redirect to non-https URL: %s", req.URL)
		}
		return nil
	},
}

// downloadClient is used specifically for downloading release assets (the binary and
// checksums.txt), which can legitimately take much longer than a release-metadata query on a slow
// connection — up to maxAssetSize (200 MiB). httpClient's 30s Timeout covers the entire exchange
// (connect + read the whole body), so reusing it for asset downloads too would abort a perfectly
// healthy, slow-but-progressing download. downloadClient reuses httpClient's CheckRedirect
// (https-only redirects) so both clients enforce the exact same redirect policy, but gives the
// exchange a much longer ceiling instead of httpClient's tight one. FetchLatestRelease keeps using
// httpClient — metadata responses are tiny and should never legitimately take anywhere near 30s.
var downloadClient = &http.Client{
	Timeout:       5 * time.Minute,
	CheckRedirect: httpClient.CheckRedirect,
}

// validateAssetURL rejects any release-asset URL (browser_download_url from the GitHub Releases
// API response) that isn't https, or whose host isn't github.com or a *.githubusercontent.com
// subdomain (GitHub's own asset-hosting domain, e.g. objects.githubusercontent.com), so a
// modified API response or an https->http downgrade cannot point self-update at an arbitrary URL.
// Called before every downloadToTemp call in Run().
func validateAssetURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid asset URL %q: %w", raw, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("asset URL %q must use https", raw)
	}
	if u.Host != "github.com" && !strings.HasSuffix(u.Host, ".githubusercontent.com") {
		return fmt.Errorf("asset URL %q has unexpected host %q", raw, u.Host)
	}
	return nil
}

// Release is the subset of the GitHub Releases API response this package needs.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset is one file attached to a GitHub release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// FindAsset looks up an asset by its exact filename.
func (r *Release) FindAsset(name string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// AssetName returns the exact release-asset filename for goos/goarch, following this project's
// naming convention (`build82_{GOOS}_{GOARCH}{ext}`).
func AssetName(goos, goarch string) string {
	ext := ""
	if goos == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("build82_%s_%s%s", goos, goarch, ext)
}

// FetchLatestRelease queries the GitHub Releases API for the latest release. apiBaseURL is
// injectable so tests can point it at an httptest.Server instead of the real GitHub API. A nil
// Release with a nil error means no releases exist yet (GitHub returns 404 for an empty list).
func FetchLatestRelease(client *http.Client, apiBaseURL string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", apiBaseURL, repoOwner, repoName)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "build82/"+version.Current)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxReleaseResponseSize))
		return nil, fmt.Errorf("unexpected status %d from %s: %s", resp.StatusCode, url, strings.TrimSpace(string(body)))
	}

	var rel Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseResponseSize)).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release response: %w", err)
	}
	return &rel, nil
}

// parseSemver parses a "vX.Y.Z" (or "X.Y.Z") tag into its three numeric components, ignoring any
// pre-release/build suffix after a "-" or "+".
func parseSemver(s string) (major, minor, patch int, ok bool) {
	s = strings.TrimPrefix(s, "v")
	if idx := strings.IndexAny(s, "-+"); idx >= 0 {
		s = s[:idx]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, 0, 0, false
		}
		nums[i] = n
	}
	return nums[0], nums[1], nums[2], true
}

// IsNewer reports whether latest is a strictly newer version than current. An unparsable current
// version (e.g. "dev", which internal/version.Current reports for a binary built without the
// release ldflags) is always treated as outdated, since a dev build has no real version to compare
// against. An unparsable latest tag never triggers an update — we can't safely claim one is
// available.
func IsNewer(current, latest string) bool {
	lMajor, lMinor, lPatch, lOK := parseSemver(latest)
	if !lOK {
		return false
	}
	cMajor, cMinor, cPatch, cOK := parseSemver(current)
	if !cOK {
		return true
	}
	if cMajor != lMajor {
		return lMajor > cMajor
	}
	if cMinor != lMinor {
		return lMinor > cMinor
	}
	return lPatch > cPatch
}

// VerifyChecksum checks that filePath's SHA-256 digest matches the line for assetName inside
// checksumsContent (the "checksums.txt" release asset, standard `sha256sum` output format:
// "<hex digest>  <filename>" per line). This step must never be skipped before replacing the
// running binary — a corrupted or tampered download is a real supply-chain risk.
func VerifyChecksum(checksumsContent, assetName, filePath string) error {
	var want string
	for _, line := range strings.Split(checksumsContent, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == assetName {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum entry found for %s in checksums.txt", assetName)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash %s: %w", filePath, err)
	}
	got := hex.EncodeToString(h.Sum(nil))

	if got != want {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", assetName, got, want)
	}
	return nil
}

// downloadToTemp downloads url into a new temp file inside dir and returns its path. dir should
// be the same directory as the binary that will eventually be replaced, so a later rename can be
// an atomic same-filesystem operation.
func downloadToTemp(client *http.Client, url, dir, pattern string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", "build82/"+version.Current)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: unexpected status %d", url, resp.StatusCode)
	}

	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer tmp.Close()

	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxAssetSize+1))
	if err != nil {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if n > maxAssetSize {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("download %s exceeded max size of %d bytes", url, maxAssetSize)
	}
	return tmp.Name(), nil
}

// smokeTestTimeout bounds how long SmokeTest waits for the downloaded binary to answer
// `--version` before giving up. Without a bound, a hung/broken downloaded binary would block
// `self-update` (and AtomicReplace, which never touches the original binary until after this
// call) forever, with no way to cancel it. A package var, not a const, so tests can shrink it
// instead of needing to actually wait out a real 10s timeout against a deliberately hung process.
var smokeTestTimeout = 10 * time.Second

// SmokeTest runs binaryPath --version as a subprocess and returns its trimmed stdout. Any error
// (non-zero exit, empty output, or timing out after smokeTestTimeout) means the caller must not
// trust this binary — this always runs before AtomicReplace ever touches the currently-running
// executable.
func SmokeTest(binaryPath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), smokeTestTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, binaryPath, "--version").Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("smoke test timed out after %s: %w", smokeTestTimeout, ctx.Err())
		}
		return "", fmt.Errorf("smoke test failed: %w", err)
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return "", errors.New("smoke test produced no version output")
	}
	return v, nil
}

// osRename is os.Rename, indirected through a package variable purely so tests can inject a
// failure at a specific rename call — the real-world "second rename fails after the first already
// succeeded, and the restore rename then ALSO fails" scenario has no other reliable, portable way
// to reproduce deterministically in a unit test.
var osRename = os.Rename

// AtomicReplace swaps newBinaryPath into currentBinaryPath's place. Both paths must be on the
// same filesystem so the final rename is atomic. The new binary is smoke-tested *before* the
// original is touched at all — on any failure the original file is left completely untouched, so
// the system is never left without a working binary. Returns the path the original binary was
// backed up to.
func AtomicReplace(currentBinaryPath, newBinaryPath string) (backupPath string, err error) {
	if err := os.Chmod(newBinaryPath, 0o755); err != nil {
		return "", fmt.Errorf("chmod new binary: %w", err)
	}

	if _, err := SmokeTest(newBinaryPath); err != nil {
		return "", fmt.Errorf("refusing to replace: %w", err)
	}

	backupPath = currentBinaryPath + ".bak"
	if err := osRename(currentBinaryPath, backupPath); err != nil {
		return "", fmt.Errorf("back up current binary: %w", err)
	}
	if err := osRename(newBinaryPath, currentBinaryPath); err != nil {
		// Best-effort restore so the system is never left without a working binary. If the
		// restore itself also fails, currentBinaryPath is missing (neither the old binary nor the
		// new one is there), so both errors are reported.
		if restoreErr := osRename(backupPath, currentBinaryPath); restoreErr != nil {
			return "", fmt.Errorf("move new binary into place: %w (restore also failed, %s is now MISSING — recover it manually from %s: %v)",
				err, currentBinaryPath, backupPath, restoreErr)
		}
		return "", fmt.Errorf("move new binary into place: %w", err)
	}
	return backupPath, nil
}

// Rollback restores the previous binary from its ".bak" backup (created by AtomicReplace) back
// into binaryPath — the manual-recovery path ("mv build82.bak build82") for when a newly
// self-updated binary passes SmokeTest at update time but turns out to be broken in real use
// afterwards. Wired up as `build82 self-update --rollback`.
//
// Unlike AtomicReplace, this is a single rename: binaryPath already holds the (possibly broken)
// current binary and backupPath simply replaces it atomically — there's no "back up the current
// binary first" step, since whatever is at binaryPath is discarded.
// The rename goes through the same osRename seam AtomicReplace uses, so a failure here is surfaced
// with full context rather than silently discarded, matching AtomicReplace's own rigor.
//
// After the rename, SmokeTest runs against the restored binary purely as a diagnostic
// confirmation. If it fails, that error is returned but the rename is NOT undone — the binary has
// already been restored, and reverting would put the known-broken pre-rollback binary back in
// place, which is strictly worse than leaving the (reportedly working, per its own prior
// AtomicReplace smoke test) restored one in place.
func Rollback(binaryPath string) error {
	backupPath := binaryPath + ".bak"

	if _, err := os.Stat(backupPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no backup found at %s — nothing to roll back", backupPath)
		}
		return fmt.Errorf("check backup at %s: %w", backupPath, err)
	}

	if err := osRename(backupPath, binaryPath); err != nil {
		return fmt.Errorf("restore backup %s to %s: %w", backupPath, binaryPath, err)
	}

	if _, err := SmokeTest(binaryPath); err != nil {
		return fmt.Errorf("restored %s from %s, but it failed the smoke test: %w", binaryPath, backupPath, err)
	}

	return nil
}

// RunOptions configures a self-update run.
type RunOptions struct {
	Check   bool
	Yes     bool
	Channel string // reserved for a future pre-release channel; "stable" is the only one today

	// APIBaseURL overrides the GitHub API base URL — only ever set in tests, pointed at an
	// httptest.Server so no test depends on real network access.
	APIBaseURL string
	Stdout     io.Writer
	Stdin      io.Reader
}

// Run is the top-level `build82 self-update` flow: check the latest release, download and verify
// it, then atomically replace the running binary.
func Run(opts RunOptions) error {
	// Channel is reserved for a future pre-release channel; "stable" (or unset) is the only
	// supported value. Any other value is rejected with an error instead of being ignored.
	if opts.Channel != "" && opts.Channel != "stable" {
		return fmt.Errorf("unsupported channel %q: only \"stable\" is currently supported", opts.Channel)
	}

	if opts.APIBaseURL == "" {
		opts.APIBaseURL = defaultAPIBaseURL
	}
	out := opts.Stdout
	if out == nil {
		out = os.Stdout
	}
	in := opts.Stdin
	if in == nil {
		in = os.Stdin
	}

	fmt.Fprintf(out, "Current version: %s\n", version.Current)

	rel, err := FetchLatestRelease(httpClient, opts.APIBaseURL)
	if err != nil {
		return err
	}
	if rel == nil {
		fmt.Fprintln(out, "No releases found.")
		return nil
	}

	if !IsNewer(version.Current, rel.TagName) {
		fmt.Fprintf(out, "Already on the latest version (%s).\n", rel.TagName)
		return nil
	}

	fmt.Fprintf(out, "A new version is available: %s (current: %s)\n", rel.TagName, version.Current)
	if opts.Check {
		return nil
	}

	if !opts.Yes {
		reader := bufio.NewReader(in)
		fmt.Fprintf(out, "Replace the running binary with %s? [y/N] ", rel.TagName)
		answer, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			fmt.Fprintln(out, "Update cancelled.")
			return nil
		}
	}

	assetName := AssetName(runtime.GOOS, runtime.GOARCH)
	asset, ok := rel.FindAsset(assetName)
	if !ok {
		return fmt.Errorf("release %s has no asset named %s", rel.TagName, assetName)
	}
	checksumsAsset, ok := rel.FindAsset("checksums.txt")
	if !ok {
		return fmt.Errorf("release %s is missing checksums.txt", rel.TagName)
	}

	currentBinaryPath, err := binpath.Resolve()
	if err != nil {
		return err
	}
	dir := filepath.Dir(currentBinaryPath)

	if err := validateAssetURL(checksumsAsset.BrowserDownloadURL); err != nil {
		return fmt.Errorf("checksums.txt: %w", err)
	}
	checksumsPath, err := downloadToTemp(downloadClient, checksumsAsset.BrowserDownloadURL, dir, "build82-checksums-*.txt")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(checksumsPath) }()

	checksumsContent, err := os.ReadFile(checksumsPath)
	if err != nil {
		return fmt.Errorf("read downloaded checksums.txt: %w", err)
	}

	if err := validateAssetURL(asset.BrowserDownloadURL); err != nil {
		return fmt.Errorf("%s: %w", assetName, err)
	}
	newBinaryPath, err := downloadToTemp(downloadClient, asset.BrowserDownloadURL, dir, "build82-update-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(newBinaryPath) }() // no-op once successfully renamed into place

	if err := VerifyChecksum(string(checksumsContent), assetName, newBinaryPath); err != nil {
		return fmt.Errorf("checksum verification failed, refusing to replace the running binary: %w", err)
	}

	backupPath, err := AtomicReplace(currentBinaryPath, newBinaryPath)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Updated to %s. Previous binary backed up at %s.\n", rel.TagName, backupPath)
	return nil
}
