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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oito2/mcp-build82/internal/prompt"
	"github.com/oito2/mcp-build82/internal/version"
)

// rewriteTransport sends every request to the test server at target, keeping the original path,
// so URLs naming github.com or GitHub's asset hosts reach the fake release server while the code
// under test still validates the real-looking URLs.
type rewriteTransport struct {
	target *url.URL
}

// RoundTrip forwards a copy of `req` to the test server.
func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.URL.Scheme = rt.target.Scheme
	r.URL.Host = rt.target.Host
	r.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(r)
}

// releaseFixture is a fake GitHub release served over HTTP, a fake running binary, and fake cosign
// behavior, wired into deps for run.
type releaseFixture struct {
	t       *testing.T
	tag     string
	current string
	asset   string

	// assets maps an asset name to its content; urls maps an asset name to its
	// browser_download_url (defaults to the pinned release URL).
	assets map[string][]byte
	urls   map[string]string
	// routes maps a request path to a handler that overrides the default asset serving.
	routes map[string]http.HandlerFunc

	// cosignVersion is the gitVersion reported by the fake cosign; empty means no cosign on PATH.
	cosignVersion string
	// verifyErr makes the fake `cosign verify-blob` fail with this output.
	verifyErr string

	mu         sync.Mutex
	requested  []string
	cosignArgs [][]string
}

// newReleaseFixture builds a current binary reporting v1.0.0 (mode 0750) and a release `tag`
// whose platform asset is a binary reporting `tag`, with matching checksums.txt and a signature
// bundle. It sets version.Current to v1.0.0 for the duration of the test.
func newReleaseFixture(t *testing.T, tag string) *releaseFixture {
	t.Helper()
	original := version.Current
	version.Current = "v1.0.0"
	t.Cleanup(func() { version.Current = original })

	dir := t.TempDir()
	current := buildFakeBinary(t, dir, "build82", "v1.0.0")
	if err := os.Chmod(current, 0o750); err != nil {
		t.Fatal(err)
	}
	newBin, err := os.ReadFile(buildFakeBinary(t, t.TempDir(), "new", tag))
	if err != nil {
		t.Fatal(err)
	}
	asset := AssetName(runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(newBin)
	return &releaseFixture{
		t: t, tag: tag, current: current, asset: asset,
		assets: map[string][]byte{
			asset:               newBin,
			"checksums.txt":     []byte(hex.EncodeToString(sum[:]) + "  " + asset + "\n"),
			signatureBundleName: []byte("{}"),
		},
		urls:   map[string]string{},
		routes: map[string]http.HandlerFunc{},
	}
}

// assetPath returns the request path of the pinned download URL of the asset `name`.
func (f *releaseFixture) assetPath(name string) string {
	return "/" + repoOwner + "/" + repoName + "/releases/download/" + f.tag + "/" + name
}

// start serves the fixture and returns the deps that reach it.
func (f *releaseFixture) start() deps {
	f.t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requested = append(f.requested, r.URL.Path)
		f.mu.Unlock()
		if h, ok := f.routes[r.URL.Path]; ok {
			h(w, r)
			return
		}
		if r.URL.Path == "/repos/"+repoOwner+"/"+repoName+"/releases/latest" {
			rel := Release{TagName: f.tag}
			for name := range f.assets {
				u, ok := f.urls[name]
				if !ok {
					u = "https://github.com" + f.assetPath(name)
				}
				rel.Assets = append(rel.Assets, Asset{Name: name, BrowserDownloadURL: u})
			}
			_ = json.NewEncoder(w).Encode(rel)
			return
		}
		for name, content := range f.assets {
			if r.URL.Path == f.assetPath(name) {
				_, _ = w.Write(content)
				return
			}
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	f.t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		f.t.Fatal(err)
	}
	client := &http.Client{Transport: rewriteTransport{target: target}}
	return deps{
		apiClient:      client,
		downloadClient: client,
		goos:           runtime.GOOS,
		goarch:         runtime.GOARCH,
		executable:     func() (string, error) { return f.current, nil },
		lookCosign: func() (string, error) {
			if f.cosignVersion == "" {
				return "", errors.New("not found")
			}
			return "/fake/cosign", nil
		},
		runCosign: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			f.mu.Lock()
			f.cosignArgs = append(f.cosignArgs, args)
			f.mu.Unlock()
			if len(args) > 0 && args[0] == "version" {
				return []byte(fmt.Sprintf(`{"gitVersion":%q}`, f.cosignVersion)), nil
			}
			if f.verifyErr != "" {
				return []byte(f.verifyErr), errors.New("exit status 1")
			}
			return []byte("Verified OK"), nil
		},
	}
}

// wasRequested reports whether the fake server received a request for `path`.
func (f *releaseFixture) wasRequested(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.requested {
		if p == path {
			return true
		}
	}
	return false
}

// run runs the self-update flow with --yes against the fixture and returns stdout, stderr and the
// error.
func (f *releaseFixture) run(opts RunOptions) (string, string, error) {
	f.t.Helper()
	d := f.start()
	var out, errOut strings.Builder
	opts.Yes = true
	opts.Stdout, opts.Stderr = &out, &errOut
	_, err := run(context.Background(), opts, d)
	return out.String(), errOut.String(), err
}

// currentVersion returns what the binary at the fixture's current path reports for --version.
func (f *releaseFixture) currentVersion() string {
	f.t.Helper()
	v, err := SmokeTest(f.current)
	if err != nil {
		f.t.Fatalf("current binary no longer runs: %v", err)
	}
	return v
}

// TestRun_UpdatesWithoutCosignWarnsAndKeepsMode verifies a full update without cosign: a warning
// on stderr, the checksum-verified binary swapped in with the replaced binary's permission bits,
// and the previous binary kept as .bak.
func TestRun_UpdatesWithoutCosignWarnsAndKeepsMode(t *testing.T) {
	f := newReleaseFixture(t, "v1.1.0")
	out, errOut, err := f.run(RunOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, out)
	}
	if !strings.Contains(errOut, "cosign was not found on PATH") || !strings.Contains(errOut, "only the checksum") {
		t.Errorf("expected a missing-cosign warning on stderr, got: %q", errOut)
	}
	if got := f.currentVersion(); got != "v1.1.0" {
		t.Errorf("expected the new binary in place, got version %q", got)
	}
	info, err := os.Stat(f.current)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o750 {
		t.Errorf("expected the new binary to keep mode 0750, got %o", info.Mode().Perm())
	}
	if _, err := os.Stat(f.current + ".bak"); err != nil {
		t.Errorf("expected the previous binary kept as .bak: %v", err)
	}
	if f.wasRequested(f.assetPath(signatureBundleName)) {
		t.Error("expected the signature bundle not to be downloaded without cosign")
	}
}

// TestRun_RequireSignatureWithoutCosignDownloadsNothing verifies that --require-signature without a
// usable cosign fails before any asset is downloaded.
func TestRun_RequireSignatureWithoutCosignDownloadsNothing(t *testing.T) {
	f := newReleaseFixture(t, "v1.1.0")
	_, _, err := f.run(RunOptions{RequireSignature: true})
	if err == nil || !strings.Contains(err.Error(), "--require-signature") {
		t.Fatalf("expected a --require-signature error, got: %v", err)
	}
	for name := range f.assets {
		if f.wasRequested(f.assetPath(name)) {
			t.Errorf("expected nothing downloaded, but %s was requested", name)
		}
	}
	if got := f.currentVersion(); got != "v1.0.0" {
		t.Errorf("expected the current binary untouched, got version %q", got)
	}
}

// TestRun_OldCosignIsNotUsed verifies that cosign older than v3 is reported and never asked to
// verify, and that the update then proceeds on the checksum alone.
func TestRun_OldCosignIsNotUsed(t *testing.T) {
	f := newReleaseFixture(t, "v1.1.0")
	f.cosignVersion = "v2.6.1"
	_, errOut, err := f.run(RunOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(errOut, "older than the v3.0.0") {
		t.Errorf("expected an old-cosign warning, got: %q", errOut)
	}
	for _, args := range f.cosignArgs {
		if args[0] == "verify-blob" {
			t.Errorf("expected cosign v2 not to be used for verification, got %v", args)
		}
	}
}

// TestRun_OldCosignWithRequireSignatureFails verifies that cosign older than v3 does not satisfy
// --require-signature.
func TestRun_OldCosignWithRequireSignatureFails(t *testing.T) {
	f := newReleaseFixture(t, "v1.1.0")
	f.cosignVersion = "v2.6.1"
	if _, _, err := f.run(RunOptions{RequireSignature: true}); err == nil {
		t.Fatal("expected cosign v2 to fail --require-signature")
	}
}

// TestRun_VerifiesSignatureWithExactIdentity verifies that cosign v3 is run on checksums.txt with
// the downloaded bundle, the exact release-workflow identity for the tag, and GitHub's OIDC issuer.
func TestRun_VerifiesSignatureWithExactIdentity(t *testing.T) {
	f := newReleaseFixture(t, "v1.1.0")
	f.cosignVersion = "v3.0.2"
	out, _, err := f.run(RunOptions{RequireSignature: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Signature verified (cosign).") {
		t.Errorf("expected a signature confirmation, got: %s", out)
	}
	var verify []string
	for _, args := range f.cosignArgs {
		if args[0] == "verify-blob" {
			verify = args
		}
	}
	if verify == nil {
		t.Fatal("expected cosign verify-blob to run")
	}
	joined := strings.Join(verify, " ")
	for _, want := range []string{
		"--certificate-identity https://github.com/oito2/mcp-build82/.github/workflows/release.yml@refs/tags/v1.1.0",
		"--certificate-oidc-issuer https://token.actions.githubusercontent.com",
		"--bundle ",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected verify-blob args to contain %q, got: %s", want, joined)
		}
	}
	if got := f.currentVersion(); got != "v1.1.0" {
		t.Errorf("expected the new binary in place, got version %q", got)
	}
}

// TestRun_SignatureFailureAborts verifies that a failed cosign verification aborts with cosign's
// output, before the binary is downloaded, leaving the current binary untouched.
func TestRun_SignatureFailureAborts(t *testing.T) {
	f := newReleaseFixture(t, "v1.1.0")
	f.cosignVersion = "v3.0.2"
	f.verifyErr = "Error: none of the expected identities matched"
	_, _, err := f.run(RunOptions{})
	if err == nil || !strings.Contains(err.Error(), "none of the expected identities matched") {
		t.Fatalf("expected the cosign output in the error, got: %v", err)
	}
	if f.wasRequested(f.assetPath(f.asset)) {
		t.Error("expected the binary not to be downloaded after a failed signature check")
	}
	if got := f.currentVersion(); got != "v1.0.0" {
		t.Errorf("expected the current binary untouched, got version %q", got)
	}
}

// TestRun_MissingSignatureBundleWithCosignFails verifies that a release without the signature
// bundle is refused when cosign is available.
func TestRun_MissingSignatureBundleWithCosignFails(t *testing.T) {
	f := newReleaseFixture(t, "v1.1.0")
	f.cosignVersion = "v3.0.2"
	delete(f.assets, signatureBundleName)
	_, _, err := f.run(RunOptions{})
	if err == nil || !strings.Contains(err.Error(), signatureBundleName) {
		t.Fatalf("expected an error naming the missing bundle, got: %v", err)
	}
}

// TestRun_RejectsAssetURLsOutsideTheRelease verifies that a browser_download_url that is not the
// pinned URL of this repository's release tag is refused before anything is downloaded.
func TestRun_RejectsAssetURLsOutsideTheRelease(t *testing.T) {
	cases := map[string]string{
		"other tag":         "https://github.com/oito2/mcp-build82/releases/download/v1.0.0/",
		"other repository":  "https://github.com/someone/mcp-build82/releases/download/v1.1.0/",
		"user content host": "https://raw.githubusercontent.com/oito2/mcp-build82/v1.1.0/",
		"query string":      "",
	}
	for name, prefix := range cases {
		t.Run(name, func(t *testing.T) {
			f := newReleaseFixture(t, "v1.1.0")
			if prefix == "" {
				f.urls[f.asset] = "https://github.com" + f.assetPath(f.asset) + "?x=1"
			} else {
				f.urls[f.asset] = prefix + f.asset
			}
			_, _, err := f.run(RunOptions{})
			if err == nil || !strings.Contains(err.Error(), "refusing to download") {
				t.Fatalf("expected the asset URL to be refused, got: %v", err)
			}
			if f.wasRequested(f.assetPath("checksums.txt")) {
				t.Error("expected nothing downloaded")
			}
		})
	}
}

// TestRun_RejectsUnexpectedTagFormat verifies that a release tag that is not vMAJOR.MINOR.PATCH is
// refused, since it is inserted into the pinned asset URL.
func TestRun_RejectsUnexpectedTagFormat(t *testing.T) {
	f := newReleaseFixture(t, "v9.0.0/../../x")
	if _, _, err := f.run(RunOptions{}); err == nil || !strings.Contains(err.Error(), "unexpected release tag") {
		t.Fatalf("expected the tag to be refused, got: %v", err)
	}
}

// TestRun_RejectsRedirectToUntrustedHost verifies that a redirect from the asset URL to a host other
// than GitHub's release hosts is refused and the current binary stays in place.
func TestRun_RejectsRedirectToUntrustedHost(t *testing.T) {
	f := newReleaseFixture(t, "v1.1.0")
	f.routes[f.assetPath(f.asset)] = func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://raw.githubusercontent.com/attacker/repo/main/build82", http.StatusFound)
	}
	_, _, err := f.run(RunOptions{})
	if err == nil || !strings.Contains(err.Error(), "unexpected host") {
		t.Fatalf("expected the redirect to be refused, got: %v", err)
	}
	if f.wasRequested("/attacker/repo/main/build82") {
		t.Error("expected the untrusted redirect target never to be requested")
	}
	if got := f.currentVersion(); got != "v1.0.0" {
		t.Errorf("expected the current binary untouched, got version %q", got)
	}
}

// TestRun_FollowsRedirectToReleaseAssetHost verifies that a redirect to GitHub's release-asset host
// is followed.
func TestRun_FollowsRedirectToReleaseAssetHost(t *testing.T) {
	f := newReleaseFixture(t, "v1.1.0")
	content := f.assets[f.asset]
	f.routes[f.assetPath(f.asset)] = func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://release-assets.githubusercontent.com/blob/1", http.StatusFound)
	}
	f.routes["/blob/1"] = func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(content) }
	if _, _, err := f.run(RunOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := f.currentVersion(); got != "v1.1.0" {
		t.Errorf("expected the new binary in place, got version %q", got)
	}
}

// TestRun_StopsAfterTooManyRedirects verifies that a redirect loop stops after maxRedirects.
func TestRun_StopsAfterTooManyRedirects(t *testing.T) {
	f := newReleaseFixture(t, "v1.1.0")
	f.routes[f.assetPath("checksums.txt")] = func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://objects.githubusercontent.com/loop", http.StatusFound)
	}
	f.routes["/loop"] = func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://objects.githubusercontent.com/loop", http.StatusFound)
	}
	_, _, err := f.run(RunOptions{})
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("stopped after %d redirects", maxRedirects)) {
		t.Fatalf("expected the redirect loop to stop, got: %v", err)
	}
}

// TestRun_CheckReturnsUpdateAvailableWithoutDownloading verifies that check mode reports an
// available update and downloads nothing.
func TestRun_CheckReturnsUpdateAvailableWithoutDownloading(t *testing.T) {
	f := newReleaseFixture(t, "v1.1.0")
	d := f.start()
	var out strings.Builder
	available, err := run(context.Background(), RunOptions{Check: true, Stdout: &out}, d)
	if err != nil || !available {
		t.Fatalf("expected an available update, got available=%v err=%v", available, err)
	}
	for name := range f.assets {
		if f.wasRequested(f.assetPath(name)) {
			t.Errorf("expected nothing downloaded in check mode, but %s was requested", name)
		}
	}
}

// TestBinaryMode verifies that a new binary takes the current binary's permission bits with the
// owner execute bit forced on, and 0755 when the current binary cannot be inspected.
func TestBinaryMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not meaningful on Windows")
	}
	dir := t.TempDir()
	for _, tc := range []struct{ mode, want os.FileMode }{{0o700, 0o700}, {0o644, 0o744}, {0o755, 0o755}} {
		p := filepath.Join(dir, fmt.Sprintf("bin-%o", tc.mode))
		if err := os.WriteFile(p, nil, tc.mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, tc.mode); err != nil {
			t.Fatal(err)
		}
		if got := binaryMode(p); got != tc.want {
			t.Errorf("binaryMode(%o) = %o, want %o", tc.mode, got, tc.want)
		}
	}
	if got := binaryMode(filepath.Join(dir, "missing")); got != 0o755 {
		t.Errorf("binaryMode(missing) = %o, want 755", got)
	}
}

// TestClearBackup_ParksBackupThatCannotBeRemoved verifies that a backup os.Remove refuses (here a
// non-empty directory, standing in for a running binary on Windows) is moved aside to a
// ".old-<n>" name, and that a later call removes the parked copy.
func TestClearBackup_ParksBackupThatCannotBeRemoved(t *testing.T) {
	dir := t.TempDir()
	bak := filepath.Join(dir, "build82.bak")
	if err := os.MkdirAll(filepath.Join(bak, "busy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := clearBackup(bak); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(bak); !os.IsNotExist(err) {
		t.Fatalf("expected the backup moved away, stat err=%v", err)
	}
	parked, err := filepath.Glob(filepath.Join(dir, "build82.bak.old-*"))
	if err != nil || len(parked) != 1 {
		t.Fatalf("expected one parked backup, got %v (err=%v)", parked, err)
	}
	// The parked copy is a non-empty directory, which os.Remove cannot delete; empty it so the
	// next call can remove it, as happens on Windows once the old binary stops running.
	if err := os.Remove(filepath.Join(parked[0], "busy")); err != nil {
		t.Fatal(err)
	}
	if err := clearBackup(bak); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(parked[0]); !os.IsNotExist(err) {
		t.Errorf("expected the parked backup removed by a later call, stat err=%v", err)
	}
}

// TestClearBackup_InUseBackupTellsToRestartClients verifies that a backup that can be neither
// removed nor moved aside yields errBackupInUse with a hint to restart the MCP clients.
func TestClearBackup_InUseBackupTellsToRestartClients(t *testing.T) {
	dir := t.TempDir()
	bak := filepath.Join(dir, "build82.bak")
	if err := os.MkdirAll(filepath.Join(bak, "busy"), 0o755); err != nil {
		t.Fatal(err)
	}
	origRename := osRename
	t.Cleanup(func() { osRename = origRename })
	osRename = func(string, string) error { return errors.New("access is denied") }

	err := clearBackup(bak)
	if !errors.Is(err, errBackupInUse) || !strings.Contains(err.Error(), "restart your MCP clients") {
		t.Fatalf("expected errBackupInUse with a restart hint, got: %v", err)
	}
}

// TestRun_InterruptedPromptChangesNothing verifies that cancelling the context while the
// confirmation prompt waits returns prompt.ErrInterrupted before anything is downloaded.
func TestRun_InterruptedPromptChangesNothing(t *testing.T) {
	f := newReleaseFixture(t, "v1.1.0")
	d := f.start()
	r, w := io.Pipe()
	defer w.Close()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := run(ctx, RunOptions{Stdin: r, Stdout: io.Discard, Stderr: io.Discard}, d)
	if !errors.Is(err, prompt.ErrInterrupted) {
		t.Fatalf("expected prompt.ErrInterrupted, got %v", err)
	}
	for name := range f.assets {
		if f.wasRequested(f.assetPath(name)) {
			t.Errorf("expected nothing downloaded, but %s was requested", name)
		}
	}
	if got := f.currentVersion(); got != "v1.0.0" {
		t.Errorf("expected the current binary untouched, got version %q", got)
	}
}
