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
	"archive/zip"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/server"
)

// Name of the bundle file and the short description shared by the manifest and server.json.
const (
	bundleName  = binary + ".mcpb"
	description = "MCP server for Moodle plugin development: structural context of a Moodle install for AI agents."
)

// bundleBinaries holds the paths of the binaries shipped in the bundle.
type bundleBinaries struct {
	Darwin     string // universal (amd64 + arm64) Mach-O
	LinuxAMD64 string // linux/amd64
	LinuxARM64 string // linux/arm64
	Windows    string // windows/amd64
}

// Paths inside the bundle, relative to its root. MCPB selects a command per operating system but
// not per CPU architecture, so on Linux the command is a launcher script that execs the binary
// matching `uname -m`; macOS gets a universal binary instead.
const (
	bundleDarwinBin     = "server/build82-darwin"
	bundleLinuxLauncher = "server/build82-linux"
	bundleLinuxAMD64Bin = "server/build82-linux-amd64"
	bundleLinuxARM64Bin = "server/build82-linux-arm64"
	bundleWindowsBin    = "server/build82-windows.exe"
)

// linuxLauncher is the POSIX shell script stored at bundleLinuxLauncher.
//
//go:embed build82-linux.sh
var linuxLauncher []byte

// bundleIcon is one icon file copied into the bundle: src is relative to the repository root, dst
// to the bundle root.
type bundleIcon struct {
	src, dst, size string
}

// bundleIcons lists the icons shipped in the bundle. The cropped variant (no background tile, mark
// filling the canvas) is used at 16/32 px where the tiled icon's margin would shrink the mark; the
// tiled icon is used from 64 px up.
var bundleIcons = []bundleIcon{
	{"docs/img/icons/icon-build82-cropped-16.png", "icons/icon-16.png", "16x16"},
	{"docs/img/icons/icon-build82-cropped-32.png", "icons/icon-32.png", "32x32"},
	{"docs/img/icons/icon-build82-64.png", "icons/icon-64.png", "64x64"},
	{"docs/img/icons/icon-build82-128.png", "icons/icon-128.png", "128x128"},
	{"docs/img/icons/icon-build82-256.png", "icons/icon-256.png", "256x256"},
	{"docs/img/icons/icon-build82-512.png", "icon.png", "512x512"},
}

// mcpbManifest mirrors the subset of the MCPB manifest (manifest_version 0.3) build82 uses. The
// nested mcpb* types below describe its author, repository, icon, server, tool, compatibility and
// user-configuration entries.
type mcpbManifest struct {
	ManifestVersion string             `json:"manifest_version"`
	Name            string             `json:"name"`
	DisplayName     string             `json:"display_name"`
	Version         string             `json:"version"`
	Description     string             `json:"description"`
	LongDescription string             `json:"long_description"`
	Author          mcpbAuthor         `json:"author"`
	Repository      mcpbRepository     `json:"repository"`
	Homepage        string             `json:"homepage"`
	Documentation   string             `json:"documentation"`
	Support         string             `json:"support"`
	Icon            string             `json:"icon"`
	Icons           []mcpbIcon         `json:"icons"`
	Keywords        []string           `json:"keywords"`
	License         string             `json:"license"`
	Server          mcpbServer         `json:"server"`
	Tools           []mcpbTool         `json:"tools"`
	ToolsGenerated  bool               `json:"tools_generated"`
	Compatibility   mcpbCompatibility  `json:"compatibility"`
	UserConfig      map[string]mcpbOpt `json:"user_config"`
}

type mcpbAuthor struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

type mcpbRepository struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type mcpbIcon struct {
	Src  string `json:"src"`
	Size string `json:"size"`
}

type mcpbServer struct {
	Type       string        `json:"type"`
	EntryPoint string        `json:"entry_point"`
	MCPConfig  mcpbMCPConfig `json:"mcp_config"`
}

type mcpbMCPConfig struct {
	Command           string                    `json:"command"`
	Args              []string                  `json:"args"`
	Env               map[string]string         `json:"env"`
	PlatformOverrides map[string]mcpbPlatformOv `json:"platform_overrides"`
}

type mcpbPlatformOv struct {
	Command string `json:"command"`
}

type mcpbTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type mcpbCompatibility struct {
	Platforms []string `json:"platforms"`
}

type mcpbOpt struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// buildManifest assembles the MCPB manifest for `version` (a "vX.Y.Z" tag) listing `tools`. The
// server runs the Linux launcher by default, with per-platform command overrides for darwin and
// windows, and receives the Moodle root through the BUILD82_MOODLE_PATH variable taken from the
// required moodle_path user setting.
func buildManifest(version string, tools []mcpbTool) mcpbManifest {
	repoURL := fmt.Sprintf("https://github.com/%s/%s", repoOrg, repoName)
	icons := make([]mcpbIcon, 0, len(bundleIcons))
	for _, ic := range bundleIcons {
		icons = append(icons, mcpbIcon{Src: ic.dst, Size: ic.size})
	}
	return mcpbManifest{
		ManifestVersion: "0.3",
		Name:            binary,
		DisplayName:     binary,
		Version:         strings.TrimPrefix(version, "v"),
		Description:     description,
		LongDescription: "build82 indexes a Moodle installation and its plugins (APIs, hooks, events, " +
			"capabilities, database schema) and exposes that context to AI assistants as MCP tools, " +
			"resources and prompts, so they can answer questions about the codebase and build plugins " +
			"that follow its real conventions.",
		Author:        mcpbAuthor{Name: repoOrg, URL: "https://github.com/" + repoOrg},
		Repository:    mcpbRepository{Type: "git", URL: repoURL + ".git"},
		Homepage:      repoURL,
		Documentation: repoURL + "/blob/main/docs/en/index.md",
		Support:       repoURL + "/issues",
		Icon:          "icon.png",
		Icons:         icons,
		Keywords:      []string{"moodle", "php", "plugin-development", "lms"},
		License:       "GPL-3.0-or-later",
		Server: mcpbServer{
			Type:       "binary",
			EntryPoint: bundleLinuxLauncher,
			MCPConfig: mcpbMCPConfig{
				Command: "${__dirname}/" + bundleLinuxLauncher,
				Args:    []string{},
				Env:     map[string]string{"BUILD82_MOODLE_PATH": "${user_config.moodle_path}"},
				PlatformOverrides: map[string]mcpbPlatformOv{
					"darwin": {Command: "${__dirname}/" + bundleDarwinBin},
					"win32":  {Command: "${__dirname}/" + bundleWindowsBin},
				},
			},
		},
		Tools:          tools,
		ToolsGenerated: false,
		Compatibility:  mcpbCompatibility{Platforms: []string{"darwin", "win32", "linux"}},
		UserConfig: map[string]mcpbOpt{
			"moodle_path": {
				Type:        "directory",
				Title:       "Moodle root directory",
				Description: "Root directory of the Moodle installation (the folder containing version.php).",
				Required:    true,
			},
		},
	}
}

// serverTools returns the name and one-sentence description of every tool the build82 server
// registers, obtained by connecting an in-memory client to it, so the manifest always matches the
// shipped server. It returns an error when the server cannot start or tools cannot be listed.
func serverTools() ([]mcpbTool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.NewServer().Connect(ctx, serverTransport, nil); err != nil {
		return nil, fmt.Errorf("start in-memory server: %w", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "release", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect in-memory client: %w", err)
	}
	defer session.Close()

	var tools []mcpbTool
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("list tools: %w", err)
		}
		tools = append(tools, mcpbTool{Name: tool.Name, Description: firstSentence(tool.Description)})
	}
	return tools, nil
}

// firstSentence returns `s` up to and including the period of its first ". " boundary, or all of
// `s` when there is none, with runs of whitespace (including newlines) collapsed to single spaces.
func firstSentence(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}

// zipEntryTime is the fixed modification time stamped on every bundle entry, so building the same
// inputs twice produces a byte-identical archive (and thus the same fileSha256).
var zipEntryTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// writeMCPB writes the MCPB bundle, a ZIP archive with manifest.json at its root, to `out`. It
// contains the JSON of `manifest`, the embedded Linux launcher, the binaries listed in `bins`
// (mode 0755) and the bundleIcons files read from under `repoRoot`. Every entry has a fixed
// modification time, so identical inputs yield an identical archive. It returns the first error
// from reading an input or writing the archive.
func writeMCPB(out, repoRoot string, manifest mcpbManifest, bins bundleBinaries) (err error) {
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	zw := zip.NewWriter(f)

	add := func(name string, mode os.FileMode, r io.Reader) error {
		hdr := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: zipEntryTime}
		hdr.SetMode(mode)
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, r)
		return err
	}
	addFile := func(name, src string, mode os.FileMode) error {
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		return add(name, mode, in)
	}

	if err := add("manifest.json", 0o644, strings.NewReader(string(manifestJSON)+"\n")); err != nil {
		return err
	}
	if err := add(bundleLinuxLauncher, 0o755, bytes.NewReader(linuxLauncher)); err != nil {
		return fmt.Errorf("add %s: %w", bundleLinuxLauncher, err)
	}
	for _, b := range []struct{ dst, src string }{
		{bundleDarwinBin, bins.Darwin},
		{bundleLinuxAMD64Bin, bins.LinuxAMD64},
		{bundleLinuxARM64Bin, bins.LinuxARM64},
		{bundleWindowsBin, bins.Windows},
	} {
		if err := addFile(b.dst, b.src, 0o755); err != nil {
			return fmt.Errorf("add %s: %w", b.dst, err)
		}
	}
	for _, ic := range bundleIcons {
		if err := addFile(ic.dst, filepath.Join(repoRoot, filepath.FromSlash(ic.src)), 0o644); err != nil {
			return fmt.Errorf("add %s: %w", ic.dst, err)
		}
	}
	return zw.Close()
}
