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

// Command release builds every cross-compiled build82 binary for a tagged release, packages the
// MCPB bundle (build82.mcpb), produces checksums.txt, and writes the MCP Registry server.json
// descriptor, all into dist/ at the repository root (which is deleted and recreated on each run).
// Usage:
//
//	go run ./scripts/release v1.0.0
//	go run ./scripts/release -allow-prerelease v0.0.0-ci
//
// The version must match vMAJOR.MINOR.PATCH. The -allow-prerelease flag also accepts a "-suffix" of
// letters, digits, dots and hyphens (e.g. v0.0.0-ci); CI uses it to exercise the full packaging
// without a real release tag. It shells out to `go build` for each target platform and is not one
// of build82's own subcommands.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/oito2/mcp-build82/internal/selfupdate"
)

// Identity of the project used in build flags, asset names and release URLs.
const (
	module   = "github.com/oito2/mcp-build82"
	binary   = "build82"
	repoOrg  = "oito2"
	repoName = "mcp-build82"
	distDir  = "dist"
)

// assetName returns the release asset file name for the given `goos` and `goarch`, the name
// self-update looks for (internal/selfupdate.AssetName).
func assetName(goos, goarch string) string {
	return selfupdate.AssetName(goos, goarch)
}

// semverTagPattern matches exactly "vMAJOR.MINOR.PATCH". The version argument is interpolated
// verbatim into a -ldflags value passed to `go build`, so anything looser (spaces, quotes) could
// inject extra linker flags.
var semverTagPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// prereleaseTagPattern additionally accepts a pre-release suffix of letters, digits, dots and
// hyphens; it is only used with -allow-prerelease.
var prereleaseTagPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z][0-9A-Za-z.-]*)?$`)

// validVersion reports whether `version` is an acceptable release tag: a strict
// vMAJOR.MINOR.PATCH, or, when `allowPrerelease` is set, one with an optional pre-release suffix.
func validVersion(version string, allowPrerelease bool) bool {
	if allowPrerelease {
		return prereleaseTagPattern.MatchString(version)
	}
	return semverTagPattern.MatchString(version)
}

// main builds the release artifacts for the version given as the only argument into dist/: one
// binary per entry of selfupdate.ReleasePlatforms, the MCPB bundle, checksums.txt and server.json.
// It exits with status 1 and a message on stderr on a usage error or any failure.
func main() {
	allowPrerelease := flag.Bool("allow-prerelease", false, "also accept a pre-release suffix such as v0.0.0-ci")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/release [-allow-prerelease] vX.Y.Z")
	}
	flag.Parse()
	if flag.NArg() != 1 || !validVersion(flag.Arg(0), *allowPrerelease) {
		flag.Usage()
		os.Exit(1)
	}
	version := flag.Arg(0)

	repoRoot, err := repoRootDir()
	if err != nil {
		fatal(err)
	}
	dist := filepath.Join(repoRoot, distDir)
	if err := os.RemoveAll(dist); err != nil {
		fatal(fmt.Errorf("clean %s: %w", dist, err))
	}
	if err := os.MkdirAll(dist, 0o755); err != nil {
		fatal(fmt.Errorf("create %s: %w", dist, err))
	}

	platforms := selfupdate.ReleasePlatforms()
	checksums := make(map[string]string, len(platforms)+1)
	for _, p := range platforms {
		goos, goarch := p[0], p[1]
		name := assetName(goos, goarch)
		out := filepath.Join(dist, name)
		fmt.Printf("Building %s...\n", name)
		if err := buildOne(repoRoot, goos, goarch, version, out); err != nil {
			fatal(fmt.Errorf("build %s: %w", name, err))
		}
		sum, err := sha256File(out)
		if err != nil {
			fatal(fmt.Errorf("checksum %s: %w", name, err))
		}
		checksums[name] = sum
	}

	bundlePath := filepath.Join(dist, bundleName)
	fmt.Printf("Packaging %s...\n", bundleName)
	if err := buildBundle(repoRoot, dist, version, bundlePath); err != nil {
		fatal(fmt.Errorf("package %s: %w", bundleName, err))
	}
	bundleSum, err := sha256File(bundlePath)
	if err != nil {
		fatal(fmt.Errorf("checksum %s: %w", bundleName, err))
	}
	checksums[bundleName] = bundleSum

	checksumsPath := filepath.Join(dist, "checksums.txt")
	if err := writeChecksumsFile(checksumsPath, checksums); err != nil {
		fatal(err)
	}
	fmt.Printf("Wrote %s\n", checksumsPath)

	// server.json embeds the bundle's SHA-256, so it is generated next to the bundle it describes
	// and published as a release asset.
	serverJSONPath := filepath.Join(dist, "server.json")
	if err := writeServerJSON(serverJSONPath, version, bundleSum); err != nil {
		fatal(err)
	}
	fmt.Printf("Wrote %s\n", serverJSONPath)
}

// buildBundle writes the MCPB bundle to `out` for `version`, using the binaries already built in
// `dist` and the icons under `repoRoot`. MCPB selects a binary per OS, not per CPU architecture:
// the two darwin binaries are merged into one universal binary, both linux binaries ship behind a
// launcher script that picks one by `uname -m`, and windows/amd64 ships as-is (Windows on Arm runs
// it through its x64 emulation; the native windows/arm64 binary is a separate release asset). The manifest tool
// list is read from the in-process server. It returns the first error encountered.
func buildBundle(repoRoot, dist, version, out string) error {
	tmp, err := os.MkdirTemp("", "build82-mcpb-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	universal := filepath.Join(tmp, "build82-darwin")
	if err := writeUniversalMachO(universal,
		filepath.Join(dist, assetName("darwin", "amd64")),
		filepath.Join(dist, assetName("darwin", "arm64")),
	); err != nil {
		return fmt.Errorf("universal darwin binary: %w", err)
	}

	tools, err := serverTools()
	if err != nil {
		return err
	}
	return writeMCPB(out, repoRoot, buildManifest(version, tools), bundleBinaries{
		Darwin:     universal,
		LinuxAMD64: filepath.Join(dist, assetName("linux", "amd64")),
		LinuxARM64: filepath.Join(dist, assetName("linux", "arm64")),
		Windows:    filepath.Join(dist, assetName("windows", "amd64")),
	})
}

// repoRootDir returns the directory containing the go.mod of the current module, as reported by
// `go env GOMOD`. It fails when the working directory is outside a Go module.
func repoRootDir() (string, error) {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		return "", fmt.Errorf("locate module root: %w", err)
	}
	goModPath := strings.TrimSpace(string(out))
	if goModPath == "" || goModPath == os.DevNull {
		return "", fmt.Errorf("not inside a Go module (go.mod not found)")
	}
	return filepath.Dir(goModPath), nil
}

// releaseLDFlags returns the linker flags for a release build: -s and -w drop the symbol table and
// DWARF debug info (panic stack traces still work, they rely on the runtime's own pclntab), an
// empty -buildid keeps the output reproducible, and -X stamps version into
// internal/version.Current.
func releaseLDFlags(version string) string {
	return fmt.Sprintf("-s -w -buildid= -X %s/internal/version.Current=%s", module, version)
}

// releaseBuildEnv returns the environment entries that pin a release build of `goos`/`goarch`,
// appended after the caller's environment so they win: cgo off, the baseline CPU level of each
// architecture, and no GOFLAGS or GOEXPERIMENT, so a local build gives the same binaries as CI
// whatever the developer's shell sets.
func releaseBuildEnv(goos, goarch string) []string {
	return []string{
		"GOOS=" + goos, "GOARCH=" + goarch, "CGO_ENABLED=0",
		"GOAMD64=v1", "GOARM64=v8.0", "GOFLAGS=", "GOEXPERIMENT=",
	}
}

// buildOne cross-compiles ./cmd/build82 for `goos`/`goarch` into the file `out` with the pinned
// environment of releaseBuildEnv and -trimpath (so the binary embeds no local path), stamping
// `version` into the binary, and streams the build output to the console. It returns the error of
// the failed `go build`, if any.
func buildOne(repoRoot, goos, goarch, version, out string) error {
	cmd := exec.Command("go", "build", "-trimpath",
		"-ldflags", releaseLDFlags(version),
		"-o", out, "./cmd/build82")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), releaseBuildEnv(goos, goarch)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// sha256File returns the hex-encoded SHA-256 digest of the file at `path`.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// writeChecksumsFile writes `checksums` (file name to hex digest) to `path` in `sha256sum` output
// format, one "<hex digest>  <filename>" line per entry sorted by file name, which is the format
// internal/selfupdate.VerifyChecksum parses.
func writeChecksumsFile(path string, checksums map[string]string) error {
	names := make([]string, 0, len(checksums))
	for name := range checksums {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		fmt.Fprintf(&b, "%s  %s\n", checksums[name], name)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// serverJSON mirrors the subset of the MCP Registry's server.json schema (2025-12-11) that
// build82 publishes. The nested types below describe its repository, icon, package and transport
// entries.
type serverJSON struct {
	Schema      string           `json:"$schema"`
	Name        string           `json:"name"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Version     string           `json:"version"`
	WebsiteURL  string           `json:"websiteUrl"`
	Repository  serverJSONRepo   `json:"repository"`
	Icons       []serverJSONIcon `json:"icons"`
	Packages    []serverJSONPkg  `json:"packages"`
}

type serverJSONRepo struct {
	URL    string `json:"url"`
	Source string `json:"source"`
}

type serverJSONIcon struct {
	Src      string   `json:"src"`
	MIMEType string   `json:"mimeType"`
	Sizes    []string `json:"sizes"`
}

type serverJSONPkg struct {
	RegistryType string              `json:"registryType"`
	Identifier   string              `json:"identifier"`
	Version      string              `json:"version"`
	FileSHA256   string              `json:"fileSha256"`
	Transport    serverJSONTransport `json:"transport"`
}

type serverJSONTransport struct {
	Type string `json:"type"`
}

// serverJSONIconSizes are the square icon sizes, in pixels, advertised in server.json. The registry
// only accepts HTTPS icon URLs, so each points at the repository's icon PNG at the release tag.
var serverJSONIconSizes = []int{64, 128, 256, 512}

// buildServerJSON assembles the MCP Registry descriptor for `version` (a "vX.Y.Z" tag). Its single
// package is the MCPB bundle release asset, identified by its download URL and the hex SHA-256
// `bundleSHA256`.
func buildServerJSON(version, bundleSHA256 string) serverJSON {
	repoURL := fmt.Sprintf("https://github.com/%s/%s", repoOrg, repoName)
	semver := strings.TrimPrefix(version, "v")

	icons := make([]serverJSONIcon, 0, len(serverJSONIconSizes))
	for _, n := range serverJSONIconSizes {
		icons = append(icons, serverJSONIcon{
			Src: fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/docs/img/icons/icon-build82-%d.png",
				repoOrg, repoName, version, n),
			MIMEType: "image/png",
			Sizes:    []string{fmt.Sprintf("%dx%d", n, n)},
		})
	}

	return serverJSON{
		Schema:      "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json",
		Name:        fmt.Sprintf("io.github.%s/%s", repoOrg, repoName),
		Title:       binary,
		Description: description,
		Version:     semver,
		WebsiteURL:  repoURL,
		Repository:  serverJSONRepo{URL: repoURL, Source: "github"},
		Icons:       icons,
		Packages: []serverJSONPkg{{
			RegistryType: "mcpb",
			Identifier:   fmt.Sprintf("%s/releases/download/%s/%s", repoURL, version, bundleName),
			Version:      semver,
			FileSHA256:   bundleSHA256,
			Transport:    serverJSONTransport{Type: "stdio"},
		}},
	}
}

// writeServerJSON writes the indented JSON descriptor from buildServerJSON(`version`,
// `bundleSHA256`) to `path`, followed by a newline.
func writeServerJSON(path, version, bundleSHA256 string) error {
	b, err := json.MarshalIndent(buildServerJSON(version, bundleSHA256), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// fatal prints `err` to stderr and exits the process with status 1.
func fatal(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}
