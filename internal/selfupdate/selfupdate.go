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
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/oito2/mcp-build82/internal/binpath"
	"github.com/oito2/mcp-build82/internal/fsutil"
	"github.com/oito2/mcp-build82/internal/prompt"
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

// maxRedirects is how many redirects a release-asset download follows before failing.
const maxRedirects = 10

// httpClient is the client used for release-metadata requests. It has a 30-second overall timeout
// and refuses any redirect to a non-https URL, so a response can never push the exchange onto an
// unencrypted connection.
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("refusing redirect to non-https URL: %s", req.URL)
		}
		return nil
	},
}

// downloadClient is the client used to download release assets (the binary, checksums.txt and its
// signature bundle). It allows a 5-minute overall timeout, since an asset can be up to maxAssetSize
// bytes on a slow connection. downloadToTemp replaces its redirect policy with one that validates
// every redirect target with validateAssetURL.
var downloadClient = &http.Client{
	Timeout:       5 * time.Minute,
	CheckRedirect: httpClient.CheckRedirect,
}

// assetHosts are the hosts a release-asset download may be served from: github.com, which answers
// the asset URL, and the hosts it redirects release assets to. Other *.githubusercontent.com hosts
// serve user content (raw files, gists) and are refused.
var assetHosts = map[string]bool{
	"github.com":                           true,
	"objects.githubusercontent.com":        true,
	"release-assets.githubusercontent.com": true,
}

// validateAssetURL checks a download URL or redirect target `raw`. It returns an error when the
// URL cannot be parsed, is not https, carries credentials or an explicit port, or its host is not
// one of assetHosts.
func validateAssetURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid asset URL %q: %w", raw, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("asset URL %q must use https", raw)
	}
	if !assetHosts[u.Hostname()] || u.Port() != "" || u.User != nil {
		return fmt.Errorf("asset URL %q has unexpected host %q: only GitHub's release hosts are accepted", raw, u.Host)
	}
	return nil
}

// validateReleaseAsset checks that `raw` is exactly the download URL of the asset `name` of this
// repository's release `tag`: https://github.com/oito2/mcp-build82/releases/download/<tag>/<name>,
// with no query or fragment. It returns an error for any other URL, so a release response cannot
// point a download anywhere else.
func validateReleaseAsset(raw, tag, name string) error {
	want := "https://github.com/" + repoOwner + "/" + repoName + "/releases/download/" + tag + "/" + name
	if raw != want {
		return fmt.Errorf("refusing to download %q: it is not the %s asset of the %s release of %s/%s", raw, name, tag, repoOwner, repoName)
	}
	return nil
}

// releaseTagPattern matches the release tags self-update accepts: "vMAJOR.MINOR.PATCH" with an
// optional pre-release suffix made only of letters, digits, dots and hyphens.
var releaseTagPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)

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

// ReleasePlatforms returns the GOOS/GOARCH pairs every release is built for, each published as the
// asset AssetName names. Each call returns a new slice.
func ReleasePlatforms() [][2]string {
	return [][2]string{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"windows", "amd64"},
		{"windows", "arm64"},
	}
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
// non-200 status, a request failure (including `ctx` ending), or an undecodable body is returned
// as an error. Response reads are capped at maxReleaseResponseSize bytes.
func FetchLatestRelease(ctx context.Context, client *http.Client, apiBaseURL string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", apiBaseURL, repoOwner, repoName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

// downloadToTemp downloads `url` with `client`, until `ctx` ends, into a new temp file in `dir` (named from
// `pattern`, as in os.CreateTemp) and returns its path. `dir` should be the directory of the binary
// to be replaced so a later rename stays on one filesystem. Every redirect target is checked with
// validateAssetURL, and at most maxRedirects redirects are followed. The file is flushed to disk
// before it is closed. It returns an error on a request failure, a rejected redirect, a non-200
// status, a write, sync or close failure, or a body larger than maxAssetSize; in the last cases the
// temp file is removed.
func downloadToTemp(ctx context.Context, client *http.Client, url, dir, pattern string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", "build82/"+version.Current)

	checked := *client
	checked.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		return validateAssetURL(req.URL.String())
	}
	resp, err := checked.Do(req)
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
	if err == nil && n > maxAssetSize {
		err = fmt.Errorf("download %s exceeded max size of %d bytes", url, maxAssetSize)
	} else if err != nil {
		err = fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if err == nil {
		if serr := tmp.Sync(); serr != nil {
			err = fmt.Errorf("sync %s: %w", tmp.Name(), serr)
		}
	}
	if cerr := tmp.Close(); err == nil && cerr != nil {
		err = fmt.Errorf("close %s: %w", tmp.Name(), cerr)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
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

// osRename is fsutil.Rename (os.Rename with a short retry on Windows) held in a variable so tests
// can inject rename failures.
var osRename = fsutil.Rename

// binaryMode returns the permission bits for a new binary replacing the one at `currentPath`: the
// current binary's own bits with the owner's execute bit always set, so an update neither widens
// nor narrows who can run it. It returns 0755 when the current binary cannot be inspected.
func binaryMode(currentPath string) os.FileMode {
	info, err := os.Stat(currentPath)
	if err != nil {
		return 0o755
	}
	return info.Mode().Perm() | 0o100
}

// errBackupInUse reports a backup that can be neither removed nor moved aside, because a running
// process still holds it.
var errBackupInUse = errors.New("the previous version is still in use")

// clearBackup removes the backup at `bak`, if any, so the current binary can take its place. On
// Windows a backup that is still running (an MCP client started it before the previous update)
// cannot be removed, but it can be renamed: it is then moved aside to "<bak>.old-<n>", which a later
// update removes once it no longer runs. Leftover "<bak>.old-*" files are removed on a best-effort
// basis. It returns an error wrapping errBackupInUse, telling the user to restart their MCP
// clients, when `bak` can be neither removed nor moved aside.
func clearBackup(bak string) error {
	dir, prefix := filepath.Dir(bak), filepath.Base(bak)+".old-"
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), prefix) {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	err := os.Remove(bak)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	parked := fmt.Sprintf("%s.old-%d", bak, time.Now().UnixNano())
	if rerr := osRename(bak, parked); rerr != nil {
		return fmt.Errorf("%w: %s can be neither removed nor moved aside (%v); restart your MCP clients, then try again", errBackupInUse, bak, err)
	}
	return nil
}

// AtomicReplace swaps the binary at `newBinaryPath` into the place of `currentBinaryPath`, which is
// first renamed to `currentBinaryPath + ".bak"` (returned as `backupPath`). Both paths must be on
// the same filesystem so the renames are atomic. The new binary gets the permission bits of the
// current one (see binaryMode) and is smoke-tested before the original is touched; when
// `wantVersion` is not empty, its `--version` output must also equal that version (compared without
// a leading "v"), so a failure there leaves the original untouched. An existing backup is cleared
// first (see clearBackup), and the directory is synced after the swap. If moving the new binary
// into place fails, the backup is renamed back; if that also fails, the returned error reports that
// `currentBinaryPath` is missing.
func AtomicReplace(currentBinaryPath, newBinaryPath, wantVersion string) (backupPath string, err error) {
	if err := os.Chmod(newBinaryPath, binaryMode(currentBinaryPath)); err != nil {
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
	if err := clearBackup(backupPath); err != nil {
		return "", err
	}
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
	fsutil.SyncDir(filepath.Dir(currentBinaryPath))
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
	fsutil.SyncDir(filepath.Dir(binaryPath))

	if _, err := SmokeTest(binaryPath); err != nil {
		return fmt.Errorf("restored %s from %s, but it failed the smoke test: %w", binaryPath, backupPath, err)
	}

	return nil
}

// signatureBundleName is the release asset holding the Sigstore bundle that signs checksums.txt.
const signatureBundleName = "checksums.txt.sigstore.json"

// minCosignMajor is the oldest cosign major version that verifies the release's signature bundle
// without extra flags.
const minCosignMajor = 3

// cosignTimeout bounds each cosign invocation.
const cosignTimeout = 2 * time.Minute

// ExitUpdateAvailable is the process exit status of `build82 self-update --check` when a newer
// release exists.
const ExitUpdateAvailable = 10

// RunOptions configures a self-update run.
type RunOptions struct {
	// Check only reports whether a newer release exists, without downloading anything.
	Check bool
	// Yes skips the confirmation prompt.
	Yes bool
	// RequireSignature refuses to update, before anything is downloaded, when no usable cosign is
	// available to verify the release signature.
	RequireSignature bool
	// Channel selects the release channel; empty or "stable" is the only supported value.
	Channel string

	// APIBaseURL overrides the GitHub API base URL; empty means the public GitHub API.
	APIBaseURL string
	// Stdout receives progress output; nil means os.Stdout.
	Stdout io.Writer
	// Stderr receives warnings; nil means os.Stderr.
	Stderr io.Writer
	// Stdin supplies the confirmation answer; nil means os.Stdin.
	Stdin io.Reader
}

// deps bundles what Run needs from its environment, so tests can replace the HTTP clients, the
// running binary's path and the cosign lookup and execution.
type deps struct {
	// apiClient performs the release-metadata request; downloadClient downloads the assets.
	apiClient      *http.Client
	downloadClient *http.Client
	// goos and goarch select the release asset to download.
	goos, goarch string
	// executable returns the symlink-resolved path of the running binary.
	executable func() (string, error)
	// lookCosign returns the path of the cosign binary, or an error when there is none.
	lookCosign func() (string, error)
	// runCosign runs the cosign binary at `path` with `args` and returns its combined output.
	runCosign func(ctx context.Context, path string, args ...string) ([]byte, error)
}

// defaultDeps returns the production dependencies: the package HTTP clients, the running platform,
// binpath.Resolve, and cosign looked up on PATH and run as a subprocess.
func defaultDeps() deps {
	return deps{
		apiClient:      httpClient,
		downloadClient: downloadClient,
		goos:           runtime.GOOS,
		goarch:         runtime.GOARCH,
		executable:     binpath.Resolve,
		lookCosign:     func() (string, error) { return exec.LookPath("cosign") },
		runCosign:      runCosign,
	}
}

// runCosign runs the cosign binary at `path` with `args`, within cosignTimeout, and returns its
// combined output and the error of the run.
func runCosign(ctx context.Context, path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, cosignTimeout)
	defer cancel()
	return exec.CommandContext(ctx, path, args...).CombinedOutput()
}

// findCosign returns the path of a cosign binary able to verify the release signature, or "" and
// the reason there is none: no cosign found by d.lookCosign, a `cosign version --json` that fails or
// reports no version, or a version older than minCosignMajor.
func findCosign(ctx context.Context, d deps) (path, problem string) {
	path, err := d.lookCosign()
	if err != nil {
		return "", "cosign was not found on PATH"
	}
	out, err := d.runCosign(ctx, path, "version", "--json")
	var info struct {
		GitVersion string `json:"gitVersion"`
	}
	if err != nil || json.Unmarshal(out, &info) != nil || info.GitVersion == "" {
		return "", fmt.Sprintf("could not read the version of %s", path)
	}
	major, _, _ := strings.Cut(strings.TrimPrefix(info.GitVersion, "v"), ".")
	if n, err := strconv.Atoi(major); err != nil || n < minCosignMajor {
		return "", fmt.Sprintf("%s is cosign %q, older than the v%d.0.0 needed to verify the release signature", path, info.GitVersion, minCosignMajor)
	}
	return path, ""
}

// verifySignature runs `cosign verify-blob` on the checksums.txt at `checksumsPath` with the
// Sigstore bundle at `bundlePath`, requiring a certificate issued by GitHub Actions to this
// repository's release workflow for exactly `tag`. It returns an error holding cosign's output
// when the verification fails.
func verifySignature(ctx context.Context, d deps, cosignPath, checksumsPath, bundlePath, tag string) error {
	identity := "https://github.com/" + repoOwner + "/" + repoName + "/.github/workflows/release.yml@refs/tags/" + tag
	out, err := d.runCosign(ctx, cosignPath, "verify-blob", checksumsPath,
		"--bundle", bundlePath,
		"--certificate-identity", identity,
		"--certificate-oidc-issuer", "https://token.actions.githubusercontent.com")
	if err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Run is the top-level `build82 self-update` flow. It checks the latest release and, with
// `opts.Check`, only reports it: `updateAvailable` is true when a newer release exists. Otherwise it
// looks for cosign v3 or later (warning on stderr when there is none, or refusing with
// `opts.RequireSignature`), asks for confirmation (unless `opts.Yes`), downloads checksums.txt, the
// signature bundle when cosign is available and the platform binary — each URL pinned to this
// repository's release tag and every redirect restricted to GitHub's release hosts — verifies the
// signature of checksums.txt, then the binary's checksum, and atomically replaces the running
// binary. It returns a nil error when there is nothing to do or the user declines, and an error for
// an unsupported channel, a failed query or download, an unexpected tag or asset URL, a missing
// asset, no usable cosign with `opts.RequireSignature`, a failed signature verification, a checksum
// mismatch or a failed replacement. Requests and the confirmation prompt stop when `ctx` ends; an
// interrupted prompt returns an error wrapping prompt.ErrInterrupted, and nothing is changed.
func Run(ctx context.Context, opts RunOptions) (updateAvailable bool, err error) {
	return run(ctx, opts, defaultDeps())
}

// run implements Run with the dependencies `d`.
func run(ctx context.Context, opts RunOptions, d deps) (bool, error) {
	if opts.Channel != "" && opts.Channel != "stable" {
		return false, fmt.Errorf("unsupported channel %q: only \"stable\" is currently supported", opts.Channel)
	}

	if opts.APIBaseURL == "" {
		opts.APIBaseURL = defaultAPIBaseURL
	}
	out := opts.Stdout
	if out == nil {
		out = os.Stdout
	}
	errOut := opts.Stderr
	if errOut == nil {
		errOut = os.Stderr
	}
	in := opts.Stdin
	if in == nil {
		in = os.Stdin
	}

	fmt.Fprintf(out, "Current version: %s\n", version.Current)

	rel, err := FetchLatestRelease(ctx, d.apiClient, opts.APIBaseURL)
	if err != nil {
		return false, err
	}
	if rel == nil {
		fmt.Fprintln(out, "No releases found.")
		return false, nil
	}
	tag := rel.TagName
	if !releaseTagPattern.MatchString(tag) {
		return false, fmt.Errorf("unexpected release tag format %q", tag)
	}

	if !IsNewer(version.Current, tag) {
		fmt.Fprintf(out, "Already on the latest version (%s).\n", tag)
		return false, nil
	}

	fmt.Fprintf(out, "A new version is available: %s (current: %s)\n", tag, version.Current)
	if opts.Check {
		return true, nil
	}

	// cosign is looked up before the confirmation, so a missing cosign is known before anything
	// is downloaded.
	cosignPath, cosignProblem := findCosign(ctx, d)
	if cosignProblem != "" {
		if opts.RequireSignature {
			return false, fmt.Errorf("%s; --require-signature refuses to update without verifying the release signature. Nothing was changed", cosignProblem)
		}
		fmt.Fprintf(errOut, "Warning: %s; the release signature will not be verified, only the checksum. Install cosign v3 or later to verify it.\n", cosignProblem)
	}

	if !opts.Yes {
		ok, err := prompt.Confirm(ctx, bufio.NewReader(in), out, fmt.Sprintf("Replace the running binary with %s? [y/N] ", tag))
		if err != nil {
			return false, err
		}
		if !ok {
			fmt.Fprintln(out, "Update cancelled.")
			return false, nil
		}
	}

	assetName := AssetName(d.goos, d.goarch)
	names := []string{"checksums.txt", assetName}
	if cosignPath != "" {
		names = append(names, signatureBundleName)
	}
	urls := map[string]string{}
	for _, name := range names {
		asset, ok := rel.FindAsset(name)
		if !ok {
			return false, fmt.Errorf("release %s has no asset named %s", tag, name)
		}
		if err := validateReleaseAsset(asset.BrowserDownloadURL, tag, name); err != nil {
			return false, err
		}
		if err := validateAssetURL(asset.BrowserDownloadURL); err != nil {
			return false, fmt.Errorf("%s: %w", name, err)
		}
		urls[name] = asset.BrowserDownloadURL
	}

	currentBinaryPath, err := d.executable()
	if err != nil {
		return false, err
	}
	dir := filepath.Dir(currentBinaryPath)

	checksumsPath, err := downloadToTemp(ctx, d.downloadClient, urls["checksums.txt"], dir, "build82-checksums-*.txt")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.Remove(checksumsPath) }()

	if cosignPath != "" {
		bundlePath, err := downloadToTemp(ctx, d.downloadClient, urls[signatureBundleName], dir, "build82-sigstore-*.json")
		if err != nil {
			return false, err
		}
		defer func() { _ = os.Remove(bundlePath) }()
		if err := verifySignature(ctx, d, cosignPath, checksumsPath, bundlePath, tag); err != nil {
			return false, fmt.Errorf("signature verification failed, refusing to replace the running binary: %w", err)
		}
		fmt.Fprintln(out, "Signature verified (cosign).")
	}

	checksumsContent, err := os.ReadFile(checksumsPath)
	if err != nil {
		return false, fmt.Errorf("read downloaded checksums.txt: %w", err)
	}

	newBinaryPath, err := downloadToTemp(ctx, d.downloadClient, urls[assetName], dir, "build82-update-*")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.Remove(newBinaryPath) }() // no-op once renamed into place

	if err := VerifyChecksum(string(checksumsContent), assetName, newBinaryPath); err != nil {
		return false, fmt.Errorf("checksum verification failed, refusing to replace the running binary: %w", err)
	}

	backupPath, err := AtomicReplace(currentBinaryPath, newBinaryPath, tag)
	if err != nil {
		return false, err
	}

	fmt.Fprintf(out, "Updated to %s. Previous binary backed up at %s.\n", tag, backupPath)
	return false, nil
}
