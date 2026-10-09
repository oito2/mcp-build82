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
	"debug/macho"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// buildTinyDarwin cross-compiles an empty Go program for darwin/goarch and returns its path.
func buildTinyDarwin(t *testing.T, dir, goarch string) string {
	t.Helper()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte("module tiny\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "tiny-"+goarch)
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "GOOS=darwin", "GOARCH="+goarch, "CGO_ENABLED=0", "GOFLAGS=")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cross-compiling darwin/%s: %v\n%s", goarch, err, b)
	}
	return out
}

// TestWriteUniversalMachO_ProducesValidFatBinary verifies the output is a valid fat Mach-O with
// aligned, byte-for-byte slices and the executable bit set.
func TestWriteUniversalMachO_ProducesValidFatBinary(t *testing.T) {
	dir := t.TempDir()
	amd64 := buildTinyDarwin(t, dir, "amd64")
	arm64 := buildTinyDarwin(t, dir, "arm64")
	out := filepath.Join(dir, "universal")

	if err := writeUniversalMachO(out, amd64, arm64); err != nil {
		t.Fatalf("writeUniversalMachO: %v", err)
	}

	fat, err := macho.OpenFat(out)
	if err != nil {
		t.Fatalf("output is not a valid fat Mach-O: %v", err)
	}
	defer fat.Close()
	if len(fat.Arches) != 2 {
		t.Fatalf("expected 2 slices, got %d", len(fat.Arches))
	}

	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	want := map[macho.Cpu]string{macho.CpuAmd64: amd64, macho.CpuArm64: arm64}
	for _, a := range fat.Arches {
		path, ok := want[a.Cpu]
		if !ok {
			t.Fatalf("unexpected slice CPU %v", a.Cpu)
		}
		delete(want, a.Cpu)
		if a.Offset%(1<<pageAlignShift) != 0 || a.Align != pageAlignShift {
			t.Errorf("%v slice not 16 KiB aligned: offset=%d align=%d", a.Cpu, a.Offset, a.Align)
		}
		orig, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(raw[a.Offset:a.Offset+a.Size], orig) {
			t.Errorf("%v slice is not a byte-for-byte copy of %s", a.Cpu, filepath.Base(path))
		}
	}

	info, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("universal binary is not executable: %v", info.Mode())
	}
}

// TestWriteUniversalMachO_RejectsBadInput verifies a single slice, duplicate CPU types and a
// non-Mach-O input are each rejected.
func TestWriteUniversalMachO_RejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	amd64 := buildTinyDarwin(t, dir, "amd64")
	notMachO := filepath.Join(dir, "not-macho")
	if err := os.WriteFile(notMachO, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")

	cases := map[string][]string{
		"single slice":       {amd64},
		"duplicate CPU type": {amd64, amd64},
		"not a Mach-O file":  {amd64, notMachO},
	}
	for name, in := range cases {
		if err := writeUniversalMachO(out, in...); err == nil {
			t.Errorf("%s: expected an error, got nil", name)
		}
	}
}

// TestBuildServerJSON_MatchesRegistrySchema verifies the server.json fields against the registry
// constraints, including that every advertised icon exists with its declared dimensions.
func TestBuildServerJSON_MatchesRegistrySchema(t *testing.T) {
	const sum = "fe333e598595000ae021bd27117db32ec69af6987f507ba7a63c90638ff633ce"
	doc := buildServerJSON("v1.2.3", sum)

	if doc.Schema != "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json" {
		t.Errorf("unexpected $schema %q", doc.Schema)
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9.-]+/[a-zA-Z0-9._-]+$`).MatchString(doc.Name) {
		t.Errorf("name %q does not match the registry pattern", doc.Name)
	}
	if n := len(doc.Description); n < 1 || n > 100 {
		t.Errorf("description must be 1-100 chars, got %d", n)
	}
	if doc.Version != "1.2.3" {
		t.Errorf("expected version without the v prefix, got %q", doc.Version)
	}
	if doc.Repository.Source != "github" || !strings.HasPrefix(doc.Repository.URL, "https://github.com/") {
		t.Errorf("unexpected repository %+v", doc.Repository)
	}

	if len(doc.Packages) != 1 {
		t.Fatalf("expected exactly 1 package (the MCPB bundle), got %d", len(doc.Packages))
	}
	pkg := doc.Packages[0]
	if pkg.RegistryType != "mcpb" || pkg.Transport.Type != "stdio" || pkg.FileSHA256 != sum || pkg.Version != "1.2.3" {
		t.Errorf("unexpected package %+v", pkg)
	}
	if !strings.Contains(pkg.Identifier, "mcp") || !strings.HasSuffix(pkg.Identifier, "/releases/download/v1.2.3/"+bundleName) {
		t.Errorf("identifier %q must be the release URL of the bundle and contain \"mcp\"", pkg.Identifier)
	}

	if len(doc.Icons) == 0 {
		t.Fatal("expected icons in server.json")
	}
	repoRoot := filepath.Join("..", "..")
	for _, ic := range doc.Icons {
		if !strings.HasPrefix(ic.Src, "https://") || len(ic.Src) > 255 {
			t.Errorf("icon src must be an HTTPS URL of at most 255 chars: %q", ic.Src)
		}
		if ic.MIMEType != "image/png" || len(ic.Sizes) != 1 {
			t.Errorf("unexpected icon metadata %+v", ic)
		}
		// The URL points at the tagged copy of a file in this repository; it must exist and
		// have the advertised dimensions.
		_, rel, ok := strings.Cut(ic.Src, "/v1.2.3/")
		if !ok {
			t.Fatalf("icon src %q is not pinned to the release tag", ic.Src)
		}
		assertPNGSize(t, filepath.Join(repoRoot, filepath.FromSlash(rel)), ic.Sizes[0])
	}

	// Every key must use the 2025-12-11 camelCase names.
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, old := range []string{"registry_type", "file_sha256"} {
		if bytes.Contains(raw, []byte(old)) {
			t.Errorf("server.json still uses the old snake_case key %q", old)
		}
	}
}

// assertPNGSize reports a test error unless the PNG at `path` is readable and has the dimensions
// `size`, written as "WxH".
func assertPNGSize(t *testing.T, path, size string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Errorf("icon %s: %v", path, err)
		return
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		t.Errorf("icon %s is not a valid PNG: %v", path, err)
		return
	}
	if got := fmt.Sprintf("%dx%d", cfg.Width, cfg.Height); got != size {
		t.Errorf("icon %s is %s, advertised as %s", path, got, size)
	}
}

// TestServerTools_ListsEveryRegisteredTool verifies serverTools returns the expected number of
// tools, each with a name and a single-line description.
func TestServerTools_ListsEveryRegisteredTool(t *testing.T) {
	tools, err := serverTools()
	if err != nil {
		t.Fatalf("serverTools: %v", err)
	}
	if len(tools) != 13 {
		t.Errorf("expected 13 tools, got %d", len(tools))
	}
	for _, tl := range tools {
		if tl.Name == "" || tl.Description == "" || strings.Contains(tl.Description, "\n") {
			t.Errorf("tool entry must have a name and a single-line description: %+v", tl)
		}
	}
}

// TestWriteMCPB_BundleIsCompleteAndReproducible verifies the bundle is byte-identical across runs,
// contains every file the manifest references with the right modes, and has a valid manifest.
func TestWriteMCPB_BundleIsCompleteAndReproducible(t *testing.T) {
	dir := t.TempDir()
	bins := bundleBinaries{}
	for name, dst := range map[string]*string{
		"darwin": &bins.Darwin, "linux-amd64": &bins.LinuxAMD64, "linux-arm64": &bins.LinuxARM64, "windows": &bins.Windows,
	} {
		*dst = filepath.Join(dir, name)
		if err := os.WriteFile(*dst, []byte("binary for "+name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tools := []mcpbTool{{Name: "doctor", Description: "Runs diagnostics."}}
	manifest := buildManifest("v1.2.3", tools)
	repoRoot := filepath.Join("..", "..")

	first := filepath.Join(dir, "a.mcpb")
	second := filepath.Join(dir, "b.mcpb")
	for _, out := range []string{first, second} {
		if err := writeMCPB(out, repoRoot, manifest, bins); err != nil {
			t.Fatalf("writeMCPB: %v", err)
		}
	}
	a, _ := os.ReadFile(first)
	b, _ := os.ReadFile(second)
	if !bytes.Equal(a, b) {
		t.Error("building the same bundle twice produced different archives")
	}

	zr, err := zip.OpenReader(first)
	if err != nil {
		t.Fatalf("bundle is not a ZIP archive: %v", err)
	}
	defer zr.Close()
	entries := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		entries[f.Name] = f
		// A timestamp taken from the clock would only differ between builds made in different
		// seconds, so the fixed stamp is asserted directly.
		if !f.Modified.Equal(zipEntryTime) {
			t.Errorf("%s has modification time %v, want the fixed %v", f.Name, f.Modified, zipEntryTime)
		}
	}

	mf, ok := entries["manifest.json"]
	if !ok {
		t.Fatal("manifest.json missing from the bundle root")
	}
	rc, err := mf.Open()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(rc)
	rc.Close()
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("manifest.json is not valid JSON: %v", err)
	}
	for _, field := range []string{"manifest_version", "name", "version", "description", "author", "server"} {
		if _, ok := got[field]; !ok {
			t.Errorf("manifest is missing required field %q", field)
		}
	}
	if got["version"] != "1.2.3" || got["manifest_version"] != "0.3" {
		t.Errorf("unexpected version fields: version=%v manifest_version=%v", got["version"], got["manifest_version"])
	}

	// Every path the manifest references must be inside the bundle.
	cfg := manifest.Server.MCPConfig
	referenced := []string{manifest.Icon, manifest.Server.EntryPoint, strings.TrimPrefix(cfg.Command, "${__dirname}/")}
	for _, ov := range cfg.PlatformOverrides {
		referenced = append(referenced, strings.TrimPrefix(ov.Command, "${__dirname}/"))
	}
	for _, ic := range manifest.Icons {
		referenced = append(referenced, ic.Src)
	}
	for _, p := range referenced {
		if _, ok := entries[p]; !ok {
			t.Errorf("manifest references %q, which is not in the bundle", p)
		}
	}
	if len(cfg.PlatformOverrides) != 2 || cfg.PlatformOverrides["darwin"].Command == "" || cfg.PlatformOverrides["win32"].Command == "" {
		t.Errorf("expected darwin and win32 command overrides, got %+v", cfg.PlatformOverrides)
	}
	if cfg.Env["BUILD82_MOODLE_PATH"] != "${user_config.moodle_path}" {
		t.Errorf("BUILD82_MOODLE_PATH must come from user_config.moodle_path, got %q", cfg.Env["BUILD82_MOODLE_PATH"])
	}
	if opt, ok := manifest.UserConfig["moodle_path"]; !ok || opt.Type != "directory" {
		t.Errorf("expected a directory user_config entry moodle_path, got %+v", manifest.UserConfig)
	}

	if manifest.Server.EntryPoint != bundleLinuxLauncher {
		t.Errorf("entry_point = %q, want the Linux launcher %q", manifest.Server.EntryPoint, bundleLinuxLauncher)
	}
	launcher, err := entries[bundleLinuxLauncher].Open()
	if err != nil {
		t.Fatal(err)
	}
	script, _ := io.ReadAll(launcher)
	launcher.Close()
	if !bytes.Equal(script, linuxLauncher) {
		t.Error("bundled Linux launcher differs from build82-linux.sh")
	}
	for _, p := range []string{bundleDarwinBin, bundleLinuxLauncher, bundleLinuxAMD64Bin, bundleLinuxARM64Bin, bundleWindowsBin} {
		if entries[p].Mode().Perm() != 0o755 {
			t.Errorf("%s must be stored as executable (0755), got %v", p, entries[p].Mode())
		}
	}
	for _, ic := range manifest.Icons {
		f, err := entries[ic.Src].Open()
		if err != nil {
			t.Fatal(err)
		}
		cfgPNG, err := png.DecodeConfig(f)
		f.Close()
		if err != nil {
			t.Errorf("%s is not a valid PNG: %v", ic.Src, err)
			continue
		}
		if got := fmt.Sprintf("%dx%d", cfgPNG.Width, cfgPNG.Height); got != ic.Size {
			t.Errorf("%s is %s, declared as %s", ic.Src, got, ic.Size)
		}
	}
}

// runLauncher runs build82-linux.sh from a directory that also holds fake per-architecture
// binaries (shell scripts printing which one ran and their arguments), with `uname -m` faked to
// report arch. It returns stdout, stderr and the exit code.
func runLauncher(t *testing.T, arch string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the Linux launcher is a POSIX shell script; Windows shells cannot fake uname -m the same way")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no POSIX shell available")
	}
	dir := t.TempDir()
	server := filepath.Join(dir, "server dir") // a space checks the launcher's quoting
	fakeBin := filepath.Join(dir, "fakebin")
	for _, d := range []string{server, fakeBin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(server, "build82-linux"), string(linuxLauncher))
	for _, a := range []string{"amd64", "arm64"} {
		write(filepath.Join(server, "build82-linux-"+a), "#!/bin/sh\necho "+a+" \"$@\"\n")
	}
	write(filepath.Join(fakeBin, "uname"), "#!/bin/sh\necho "+arch+"\n")

	var out, errBuf bytes.Buffer
	cmd := exec.Command(filepath.Join(server, "build82-linux"), args...)
	cmd.Env = append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running launcher: %v", err)
	}
	return out.String(), errBuf.String(), code
}

// TestLinuxLauncher_ExecsBinaryForArchitecture verifies the launcher runs the binary matching the
// reported CPU architecture and passes its arguments through unchanged.
func TestLinuxLauncher_ExecsBinaryForArchitecture(t *testing.T) {
	cases := map[string]string{"x86_64": "amd64", "amd64": "amd64", "aarch64": "arm64", "arm64": "arm64"}
	for arch, want := range cases {
		stdout, stderr, code := runLauncher(t, arch, "--http", "a b")
		if code != 0 || stderr != "" {
			t.Errorf("%s: exit %d, stderr %q", arch, code, stderr)
		}
		if stdout != want+" --http a b\n" {
			t.Errorf("%s: stdout = %q, want the %s binary with arguments passed through", arch, stdout, want)
		}
	}
}

// TestLinuxLauncher_RejectsUnsupportedArchitecture verifies an unsupported architecture yields a
// non-zero exit, an error on stderr and nothing on stdout.
func TestLinuxLauncher_RejectsUnsupportedArchitecture(t *testing.T) {
	stdout, stderr, code := runLauncher(t, "riscv64")
	if code == 0 {
		t.Error("expected a non-zero exit code")
	}
	if stdout != "" {
		t.Errorf("stdout must stay empty (it carries the MCP stream), got %q", stdout)
	}
	if !strings.Contains(stderr, "unsupported CPU architecture: riscv64") {
		t.Errorf("expected an explanatory error on stderr, got %q", stderr)
	}
}
