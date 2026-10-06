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

// GitHub API endpoint and repository that releases are fetched from.
const (
	defaultAPIBaseURL = "https://api.github.com"
	repoOwner         = "oito2"
	repoName          = "mcp-build82"
)

// maxAssetSize caps the size in bytes of a single downloaded release asset (binary or
// checksums.txt), so an oversized response cannot fill the disk before checksum verification runs.
// It is a variable so tests can lower it.
var maxAssetSize int64 = 200 << 20 // 200 MiB — generous headroom over any real build82 binary

// maxReleaseResponseSize caps how many bytes of a GitHub Releases API response FetchLatestRelease
// reads into memory, for both the JSON body on success and the error body on an unexpected status.
// It is a variable so tests can lower it.
var maxReleaseResponseSize int64 = 1 << 20 // 1 MiB

// httpClient is the client used for release-metadata requests. It has a 30-second overall timeout
// and refuses any redirect to a non-https URL, so a response can never push the exchange onto an
// unencrypted connection (see validateAssetURL for the initial-URL check).
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("refusing redirect to non-https URL: %s", req.URL)
		}
		return nil
	},
}

// downloadClient is the client used to download release assets (the binary and checksums.txt). It
// applies the same https-only redirect policy as httpClient but allows a 5-minute overall timeout,
// since an asset can be up to maxAssetSize bytes on a slow connection.
var downloadClient = &http.Client{
	Timeout:       5 * time.Minute,
	CheckRedirect: httpClient.CheckRedirect,
}

// validateAssetURL checks a release-asset download URL (`raw`, a browser_download_url from the
// release metadata). It returns an error when the URL cannot be parsed, is not https, or its host
// is neither github.com nor a *.githubusercontent.com subdomain. Run calls it before every asset
// download.
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

// FindAsset returns the asset whose filename equals `name` exactly; ok is false when the release
// has no such asset.
func (r *Release) FindAsset(name string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// AssetName returns the release-asset filename for the given `goos` and `goarch`, in the form
// `build82_{GOOS}_{GOARCH}`, with a `.exe` suffix on windows.
func AssetName(goos, goarch string) string {
	ext := ""
	if goos == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("build82_%s_%s%s", goos, goarch, ext)
}

// FetchLatestRelease queries the GitHub Releases API at `apiBaseURL` using `client` and returns the
// latest release. A nil Release with a nil error means no release exists (HTTP 404). Any other
// non-200 status, a request failure, or an undecodable body is returned as an error. Response
// reads are capped at maxReleaseResponseSize bytes.
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

// semver is a parsed "vX.Y.Z[-pre-release][+build]" version. Build metadata is discarded.
type semver struct {
	major, minor, patch int
	pre                 []string
}

// parseSemver parses a "vX.Y.Z" (or "X.Y.Z") tag `s`, with an optional "-" pre-release part
// and an optional "+" build part that is ignored. ok is false when the core does not have exactly
// three numeric dot-separated parts or a pre-release identifier is empty.
func parseSemver(s string) (v semver, ok bool) {
	s = strings.TrimPrefix(s, "v")
	if idx := strings.Index(s, "+"); idx >= 0 {
		s = s[:idx]
	}
	if idx := strings.Index(s, "-"); idx >= 0 {
		v.pre = strings.Split(s[idx+1:], ".")
		for _, id := range v.pre {
			if id == "" {
				return semver{}, false
			}
		}
		s = s[:idx]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, false
		}
		nums[i] = n
	}
	v.major, v.minor, v.patch = nums[0], nums[1], nums[2]
	return v, true
}

// isNumeric reports whether `id` consists only of ASCII digits.
func isNumeric(id string) bool {
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return id != ""
}

// comparePre compares two pre-release identifier lists by semver precedence and returns -1, 0 or
// 1. An empty list (a normal release) ranks above any pre-release. Identifiers compare left to
// right: numeric ones numerically, alphanumeric ones lexically in ASCII order, numeric below
// alphanumeric, and a shorter list below a longer one when all shared identifiers are equal.
func comparePre(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return 1
	case len(b) == 0:
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] == b[i] {
			continue
		}
		aNum, bNum := isNumeric(a[i]), isNumeric(b[i])
		switch {
		case aNum && bNum:
			// Compare by length first so arbitrarily long numbers need no integer conversion.
			an, bn := strings.TrimLeft(a[i], "0"), strings.TrimLeft(b[i], "0")
			if len(an) != len(bn) {
				return cmpInt(len(an), len(bn))
			}
			return strings.Compare(an, bn)
		case aNum:
			return -1
		case bNum:
			return 1
		default:
			return strings.Compare(a[i], b[i])
		}
	}
	return cmpInt(len(a), len(b))
}

// cmpInt returns -1, 0 or 1 as a is less than, equal to or greater than b.
func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// IsNewer reports whether `latest` is a strictly newer version than `current` by semver
// precedence: major, minor and patch numbers first, then pre-release identifiers (a release
// outranks the same version with a pre-release suffix). Build metadata is ignored. An unparsable
// `current` (such as "dev", the default of version.Current) is always treated as outdated. An
// unparsable `latest` never counts as newer.
func IsNewer(current, latest string) bool {
	l, lOK := parseSemver(latest)
	if !lOK {
		return false
	}
	c, cOK := parseSemver(current)
	if !cOK {
		return true
	}
	if c.major != l.major {
		return l.major > c.major
	}
	if c.minor != l.minor {
		return l.minor > c.minor
	}
	if c.patch != l.patch {
		return l.patch > c.patch
	}
	return comparePre(c.pre, l.pre) < 0
}

// VerifyChecksum checks that the SHA-256 digest of the file at `filePath` matches the entry for
// `assetName` in `checksumsContent` (checksums.txt content in `sha256sum` format, one
// "<hex digest>  <filename>" per line). It returns an error when there is no entry for
// `assetName`, the file cannot be read, or the digests differ.
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

// downloadToTemp downloads `url` with `client` into a new temp file in `dir` (named from
// `pattern`, as in os.CreateTemp) and returns its path. `dir` should be the directory of the binary
// to be replaced so a later rename stays on one filesystem. It returns an error on a request
// failure, a non-200 status, a write or close failure, or a body larger than maxAssetSize; in the
// last three cases the temp file is removed.
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

	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxAssetSize+1))
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if n > maxAssetSize {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("download %s exceeded max size of %d bytes", url, maxAssetSize)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("close %s: %w", tmp.Name(), err)
	}
	return tmp.Name(), nil
}

// smokeTestTimeout bounds how long SmokeTest waits for a binary to answer `--version`. It is a
// variable so tests can lower it.
var smokeTestTimeout = 10 * time.Second

// SmokeTest runs `binaryPath --version` as a subprocess and returns its trimmed stdout. It returns
// an error on a non-zero exit, empty output, or a timeout after smokeTestTimeout, in which case the
// binary must not be trusted.
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

// osRename is os.Rename held in a variable so tests can inject rename failures.
var osRename = os.Rename

// AtomicReplace swaps the binary at `newBinaryPath` into the place of `currentBinaryPath`, which is
// first renamed to `currentBinaryPath + ".bak"` (returned as `backupPath`). Both paths must be on
// the same filesystem so the renames are atomic. The new binary is made executable and smoke-tested
// before the original is touched; when `wantVersion` is not empty, its `--version` output must also
// equal that version (compared without a leading "v"), so a failure there leaves the original untouched. If moving the
// new binary into place fails, the backup is renamed back; if that also fails, the returned error
// reports that `currentBinaryPath` is missing.
func AtomicReplace(currentBinaryPath, newBinaryPath, wantVersion string) (backupPath string, err error) {
	if err := os.Chmod(newBinaryPath, 0o755); err != nil {
		return "", fmt.Errorf("chmod new binary: %w", err)
	}

	got, err := SmokeTest(newBinaryPath)
	if err != nil {
		return "", fmt.Errorf("refusing to replace: %w", err)
	}
	if wantVersion != "" && strings.TrimPrefix(got, "v") != strings.TrimPrefix(wantVersion, "v") {
		return "", fmt.Errorf("refusing to replace: new binary reports version %q, expected %s", got, wantVersion)
	}

	backupPath = currentBinaryPath + ".bak"
	if err := osRename(currentBinaryPath, backupPath); err != nil {
		return "", fmt.Errorf("back up current binary: %w", err)
	}
	if err := osRename(newBinaryPath, currentBinaryPath); err != nil {
		// Best-effort restore of the original binary. If the restore also fails,
		// currentBinaryPath is missing, so both errors are reported.
		if restoreErr := osRename(backupPath, currentBinaryPath); restoreErr != nil {
			return "", fmt.Errorf("move new binary into place: %w (restore also failed, %s is now MISSING — recover it manually from %s: %v)",
				err, currentBinaryPath, backupPath, restoreErr)
		}
		return "", fmt.Errorf("move new binary into place: %w", err)
	}
	return backupPath, nil
}

// Rollback restores the binary at `binaryPath` from its ".bak" backup created by AtomicReplace,
// as used by `build82 self-update --rollback`. The backup is renamed over `binaryPath`, discarding
// whatever is there. It returns an error when the backup is missing or cannot be inspected, or the
// rename fails. After the rename the restored binary is smoke-tested; a failure there is returned
// as an error, but the rename is not undone.
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
	// Check only reports whether a newer release exists, without downloading anything.
	Check bool
	// Yes skips the confirmation prompt.
	Yes bool
	// Channel selects the release channel; empty or "stable" is the only supported value.
	Channel string

	// APIBaseURL overrides the GitHub API base URL; empty means the public GitHub API.
	APIBaseURL string
	// Stdout receives progress output; nil means os.Stdout.
	Stdout io.Writer
	// Stdin supplies the confirmation answer; nil means os.Stdin.
	Stdin io.Reader
}

// Run is the top-level `build82 self-update` flow: it checks the latest release, asks for
// confirmation (unless `opts.Yes` or `opts.Check`), downloads the platform binary and checksums.txt,
// verifies the checksum, and atomically replaces the running binary. It returns nil when there is
// nothing to do or the user declines, and an error for an unsupported channel, a failed query or
// download, a missing asset, a checksum mismatch or a failed replacement.
func Run(opts RunOptions) error {
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
	defer func() { _ = os.Remove(newBinaryPath) }() // no-op once renamed into place

	if err := VerifyChecksum(string(checksumsContent), assetName, newBinaryPath); err != nil {
		return fmt.Errorf("checksum verification failed, refusing to replace the running binary: %w", err)
	}

	backupPath, err := AtomicReplace(currentBinaryPath, newBinaryPath, rel.TagName)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Updated to %s. Previous binary backed up at %s.\n", rel.TagName, backupPath)
	return nil
}
