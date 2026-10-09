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

package installer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// TestWriteConfig_PreservesKeyOrderAndValues verifies that an install keeps every key in its
// original order, appends build82 at the end of the servers object, keeps numbers and special
// characters exactly as written, and indents with two spaces.
func TestWriteConfig_PreservesKeyOrderAndValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	mustWriteFile(t, path, []byte(`{"zeta": 1, "mcpServers": {"b": {"command": "/b"}, "a": {"url": "https://x/?a=1&b=<2>"}}, "alpha": 12345678901234567890, "f": 1.50}`), 0o644)
	tg := target{ID: "cursor", InstallPaths: fixedPaths(path), Shape: shapeMcpServers}

	if err := writeConfig(tg, path, testBin, testMoodle); err != nil {
		t.Fatal(err)
	}
	got := string(mustReadFile(t, path))
	want := `{
  "zeta": 1,
  "mcpServers": {
    "b": {
      "command": "/b"
    },
    "a": {
      "url": "https://x/?a=1&b=<2>"
    },
    "build82": {
      "args": [],
      "command": "/usr/local/bin/build82",
      "env": {
        "BUILD82_MOODLE_PATH": "/var/www/moodle"
      }
    }
  },
  "alpha": 12345678901234567890,
  "f": 1.50
}
`
	if got != want {
		t.Errorf("unexpected file content:\n%s\nwant:\n%s", got, want)
	}
}

// TestWriteConfig_ReplacesEntryInPlace verifies that re-installing over an outdated build82 entry
// keeps its position among the other servers.
func TestWriteConfig_ReplacesEntryInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	mustWriteFile(t, path, []byte(`{"mcpServers":{"a":{},"build82":{"command":"/old"},"z":{}}}`), 0o644)
	tg := target{ID: "cursor", InstallPaths: fixedPaths(path), Shape: shapeMcpServers}
	if err := writeConfig(tg, path, testBin, testMoodle); err != nil {
		t.Fatal(err)
	}
	m, _, err := readConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	members, err := objectMembers(mustReadFile(t, path))
	if err != nil {
		t.Fatal(err)
	}
	servers, _ := memberValue(members, "mcpServers")
	inner, err := objectMembers(servers)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, mem := range inner {
		keys = append(keys, mem.key)
	}
	if !reflect.DeepEqual(keys, []string{"a", "build82", "z"}) {
		t.Errorf("server order = %v, want [a build82 z]", keys)
	}
	if entry := m["mcpServers"].(map[string]any)["build82"].(map[string]any); entry["command"] != testBin {
		t.Errorf("entry not updated: %+v", entry)
	}
}

// TestRemoveEntryFile_PreservesKeyOrder verifies that an uninstall removes only the build82 entry
// and keeps the order of everything else.
func TestRemoveEntryFile_PreservesKeyOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	mustWriteFile(t, path, []byte(`{"z":true,"mcpServers":{"b":{},"build82":{"command":"/x"},"a":{}},"a":null}`), 0o644)
	removed, err := removeEntryFile(path, shapeMcpServers)
	if err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	want := "{\n  \"z\": true,\n  \"mcpServers\": {\n    \"b\": {},\n    \"a\": {}\n  },\n  \"a\": null\n}\n"
	if got := string(mustReadFile(t, path)); got != want {
		t.Errorf("unexpected content:\n%s\nwant:\n%s", got, want)
	}
}

// TestWriteConfig_KeepsSymlink verifies that a config path that is a symbolic link stays a link
// and the file it points to receives the entry.
func TestWriteConfig_KeepsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need extra privileges on Windows")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles", "mcp.json")
	mustMkdir(t, filepath.Dir(real))
	mustWriteFile(t, real, []byte(`{"mcpServers":{}}`), 0o600)
	link := filepath.Join(dir, "mcp.json")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	tg := target{ID: "cursor", InstallPaths: fixedPaths(link), Shape: shapeMcpServers}
	if err := writeConfig(tg, link, testBin, testMoodle); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected %s to remain a symlink (err=%v)", link, err)
	}
	if entry := readEntry(t, real, "mcpServers"); entry["command"] != testBin {
		t.Errorf("unexpected entry in the link target: %+v", entry)
	}
	if info, err := os.Stat(real); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("expected the link target to keep mode 0600 (err=%v)", err)
	}
	if removed, err := removeEntryFile(link, shapeMcpServers); err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected %s to remain a symlink after uninstall (err=%v)", link, err)
	}
}

// TestWriteConfig_ToleratesBOM verifies that a file starting with a UTF-8 byte order mark is read
// and rewritten as plain JSON with every key kept.
func TestWriteConfig_ToleratesBOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	mustWriteFile(t, path, []byte("\xef\xbb\xbf{\"mcpServers\":{\"other\":{\"command\":\"/o\"}}}"), 0o644)
	tg := target{ID: "cursor", InstallPaths: fixedPaths(path), Shape: shapeMcpServers}
	if err := writeConfig(tg, path, testBin, testMoodle); err != nil {
		t.Fatalf("a BOM must not make the file unparsable: %v", err)
	}
	content := mustReadFile(t, path)
	if strings.HasPrefix(string(content), "\xef\xbb\xbf") {
		t.Error("expected the rewritten file to have no BOM")
	}
	if entry := readEntry(t, path, "mcpServers"); entry["command"] != testBin {
		t.Errorf("unexpected entry: %+v", entry)
	}
	if !strings.Contains(string(content), `"other"`) {
		t.Error("expected the other server to survive")
	}
}

// TestWriteConfig_RefusesNonObjectServersKey verifies that a servers key holding something other
// than an object fails and leaves the file unchanged.
func TestWriteConfig_RefusesNonObjectServersKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	original := `{"mcpServers":["keep","me"]}`
	mustWriteFile(t, path, []byte(original), 0o644)
	tg := target{ID: "cursor", InstallPaths: fixedPaths(path), Shape: shapeMcpServers}
	err := writeConfig(tg, path, testBin, testMoodle)
	if err == nil || onlyManual(err) || !strings.Contains(err.Error(), "is not a JSON object") {
		t.Fatalf("expected a non-manual failure, got %v", err)
	}
	if got := string(mustReadFile(t, path)); got != original {
		t.Errorf("file changed:\n%s", got)
	}
}

// TestInstall_JSONCIsManualStep verifies that a commented config file is reported as "manual step
// needed" with the snippet, never as failed, and is left byte-for-byte unchanged.
func TestInstall_JSONCIsManualStep(t *testing.T) {
	home := fakeHome(t, "linux")
	dir := filepath.Join(home, ".cursor")
	mustMkdir(t, dir)
	path := filepath.Join(dir, "mcp.json")
	original := "{\n  // my servers\n  \"mcpServers\": {},\n}\n"
	mustWriteFile(t, path, []byte(original), 0o644)
	tg := mustTarget(t, "cursor")

	replaced, warnings, err := installTarget(tg, testBin, testMoodle)
	if !onlyManual(err) {
		t.Fatalf("expected only a manual-edit error, got %v", err)
	}
	out := captureStdout(t, func() { reportInstall(tg, replaced, warnings, err) })
	if !strings.Contains(out, "manual step needed") || !strings.Contains(out, testBin) || strings.Contains(out, "failed") {
		t.Errorf("unexpected report: %q", out)
	}
	if got := string(mustReadFile(t, path)); got != original {
		t.Errorf("file changed:\n%s", got)
	}
}

// TestInstall_JSONCWithCorrectEntryCountsAsInstalled verifies that a commented config file that
// already holds the wanted entry is reported as installed and left unchanged.
func TestInstall_JSONCWithCorrectEntryCountsAsInstalled(t *testing.T) {
	home := fakeHome(t, "linux")
	dir := filepath.Join(home, ".cursor")
	mustMkdir(t, dir)
	path := filepath.Join(dir, "mcp.json")
	original := `{
  // build82, added by hand
  "mcpServers": {
    "build82": {"type": "stdio", "command": "/usr/local/bin/build82", "args": [], "env": {"BUILD82_MOODLE_PATH": "/var/www/moodle"}, "disabled": false},
  },
}
`
	mustWriteFile(t, path, []byte(original), 0o644)
	tg := mustTarget(t, "cursor")
	replaced, warnings, err := installTarget(tg, testBin, testMoodle)
	if err != nil {
		t.Fatalf("expected the matching entry to count as installed, got %v", err)
	}
	out := captureStdout(t, func() { reportInstall(tg, replaced, warnings, err) })
	if !strings.Contains(out, "updated.") {
		t.Errorf("expected \"updated.\", got %q", out)
	}
	if got := string(mustReadFile(t, path)); got != original {
		t.Errorf("file changed:\n%s", got)
	}
}

// TestOnlyManual verifies that a mix of manual and real errors is not treated as manual only.
func TestOnlyManual(t *testing.T) {
	manual := &manualEditError{"edit by hand"}
	if !onlyManual(manual) || !onlyManual(errors.Join(manual, manual)) {
		t.Error("expected manual-only errors to be recognized")
	}
	if onlyManual(nil) || onlyManual(errors.New("boom")) || onlyManual(errors.Join(manual, errors.New("boom"))) {
		t.Error("expected any real error to make the outcome a failure")
	}
}

// TestUnreadableConfig_IsFailedNotUnregistered verifies that a config file that cannot be parsed
// is reported as failed by install and uninstall, and is listed by an uninstall without a target.
func TestUnreadableConfig_IsFailedNotUnregistered(t *testing.T) {
	home := fakeHome(t, "linux")
	noCLIsOnPath(t)
	dir := filepath.Join(home, ".cursor")
	mustMkdir(t, dir)
	path := filepath.Join(dir, "mcp.json")
	mustWriteFile(t, path, []byte("{{{ not json"), 0o644)
	tg := mustTarget(t, "cursor")

	if _, _, err := installTarget(tg, testBin, testMoodle); err == nil || onlyManual(err) {
		t.Errorf("install: expected a failure, got %v", err)
	}
	if !hasEntry(tg) {
		t.Error("an unreadable config must be listed by uninstall, not taken as unregistered")
	}
	removed, warnings, err := uninstallTarget(tg)
	out := captureStdout(t, func() { reportUninstall(tg, removed, warnings, err) })
	if !strings.Contains(out, "failed") || strings.Contains(out, "not registered") {
		t.Errorf("uninstall: expected \"failed\", got %q", out)
	}
}

// TestUninstall_NoTargetCleansUndetectedFileTarget verifies that an uninstall without a target
// also cleans a file target whose client is no longer detected but whose config still holds the
// entry, and that it reports failures with an error.
func TestUninstall_NoTargetCleansUndetectedFileTarget(t *testing.T) {
	home := fakeHome(t, "linux")
	noCLIsOnPath(t)
	stubCommands(t, func(recordedCall) error { return errors.New("not found") })
	dir := filepath.Join(home, ".config", "opencode")
	mustMkdir(t, dir)
	path := filepath.Join(dir, "opencode.json")
	mustWriteFile(t, path, []byte(`{"mcp":{"build82":{"type":"local"}},"theme":"x"}`), 0o644)
	if detectTarget(mustTarget(t, "opencode")) {
		t.Fatal("opencode must not be detected without the opencode command")
	}
	withStdin(t, "y\n")
	out := captureStdout(t, func() {
		if err := Uninstall(context.Background(), "", false); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "OpenCode... removed.") {
		t.Errorf("expected OpenCode to be cleaned, got %q", out)
	}
	if has, err := fileHasEntry(path, shapeOpenCode); err != nil || has {
		t.Errorf("expected the entry removed (has=%v err=%v)", has, err)
	}
}

// TestUninstall_DecliningSkipsPurgeAndSaysSo verifies that declining the removal confirmation
// with --purge prints that the purge was skipped too.
func TestUninstall_DecliningSkipsPurgeAndSaysSo(t *testing.T) {
	home := fakeHome(t, "linux")
	noCLIsOnPath(t)
	dir := filepath.Join(home, ".cursor")
	mustMkdir(t, dir)
	mustWriteFile(t, filepath.Join(dir, "mcp.json"), []byte(`{"mcpServers":{"build82":{}}}`), 0o644)
	withStdin(t, "n\n")
	out := captureStdout(t, func() {
		if err := Uninstall(context.Background(), "", true); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "--purge was skipped too") || strings.Contains(out, "--purge will delete") {
		t.Errorf("unexpected output: %q", out)
	}
}

// TestUninstall_NoTargetReturnsErrorOnFailure verifies that an uninstall without a target exits
// with an error when a target could not be cleaned.
func TestUninstall_NoTargetReturnsErrorOnFailure(t *testing.T) {
	home := fakeHome(t, "linux")
	noCLIsOnPath(t)
	dir := filepath.Join(home, ".cursor")
	mustMkdir(t, dir)
	mustWriteFile(t, filepath.Join(dir, "mcp.json"), []byte("{{{"), 0o644)
	withStdin(t, "y\n")
	captureStdout(t, func() {
		if err := Uninstall(context.Background(), "", false); err == nil {
			t.Error("expected an error when a target failed")
		}
	})
}

// TestClaudeDesktop_WindowsMSIXPackage verifies that the virtualized directory of Claude Desktop's
// MSIX package is detected and receives the entry, alongside the classic directory when both
// exist.
func TestClaudeDesktop_WindowsMSIXPackage(t *testing.T) {
	home := fakeHome(t, "windows")
	noCLIsOnPath(t)
	msix := filepath.Join(home, "AppData", "Local", "Packages", "Claude_pzs8sxrjxfjjc", "LocalCache", "Roaming", "Claude")
	classic := filepath.Join(home, "AppData", "Roaming", "Claude")
	mustMkdir(t, filepath.Join(home, "AppData", "Local", "Packages", "SomethingElse_123"))

	tg := mustTarget(t, "claude-desktop")
	if detectTarget(tg) {
		t.Fatal("must not be detected before any Claude directory exists")
	}
	mustMkdir(t, msix)
	tg = mustTarget(t, "claude-desktop")
	if !detectTarget(tg) {
		t.Fatal("expected detection through the MSIX package directory")
	}
	msixFile := filepath.Join(msix, "claude_desktop_config.json")
	if got := tg.InstallPaths(); !reflect.DeepEqual(got, []string{msixFile}) {
		t.Errorf("install paths = %v, want only the MSIX file", got)
	}
	mustMkdir(t, classic)
	classicFile := filepath.Join(classic, "claude_desktop_config.json")
	if got := tg.InstallPaths(); !reflect.DeepEqual(got, []string{classicFile, msixFile}) {
		t.Errorf("install paths = %v, want both files", got)
	}
	if _, _, err := installTarget(tg, testBin, testMoodle); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{classicFile, msixFile} {
		if entry := readEntry(t, p, "mcpServers"); entry["command"] != testBin {
			t.Errorf("unexpected entry in %s: %+v", p, entry)
		}
	}
	if removed, _, err := uninstallTarget(tg); err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	if hasEntry(tg) {
		t.Error("expected both files cleaned")
	}
}

// TestCline_ClineDirReplacesHomeDirectory verifies that an absolute CLINE_DIR replaces ~/.cline
// for detection and settings, and that a relative one is ignored.
func TestCline_ClineDirReplacesHomeDirectory(t *testing.T) {
	home := fakeHome(t, "linux")
	noCLIsOnPath(t)
	custom := filepath.Join(t.TempDir(), "cline-config")
	t.Setenv("CLINE_DIR", custom)
	mustMkdir(t, filepath.Join(home, ".cline"))
	tg := mustTarget(t, "cline")
	if detectTarget(tg) {
		t.Fatal("~/.cline must not mark an install when CLINE_DIR points elsewhere")
	}
	mustMkdir(t, custom)
	if !detectTarget(tg) {
		t.Fatal("expected detection through CLINE_DIR")
	}
	want := filepath.Join(custom, "data", "settings", "cline_mcp_settings.json")
	if got := tg.InstallPaths(); !reflect.DeepEqual(got, []string{want}) {
		t.Errorf("paths = %v, want %v", got, []string{want})
	}

	t.Setenv("CLINE_DIR", "relative/dir")
	tg = mustTarget(t, "cline")
	want = filepath.Join(home, ".cline", "data", "settings", "cline_mcp_settings.json")
	if got := tg.InstallPaths(); !reflect.DeepEqual(got, []string{want}) {
		t.Errorf("relative CLINE_DIR: paths = %v, want %v", got, []string{want})
	}
}

// TestCodex_GetFailureOtherThanNotRegisteredIsAnError verifies that a failing `codex mcp get`
// whose output does not say the server is missing aborts the install before `mcp add`, and makes
// an uninstall without a target list Codex so the failure is reported.
func TestCodex_GetFailureOtherThanNotRegisteredIsAnError(t *testing.T) {
	fakeHome(t, "linux")
	prev := lookPath
	lookPath = func(string) (string, error) { return "/usr/bin/codex", nil }
	t.Cleanup(func() { lookPath = prev })
	calls := stubCommands(t, func(c recordedCall) error {
		if c.Args[1] == "get" {
			return errors.New("Error: failed to read ~/.codex/config.toml: permission denied")
		}
		return nil
	})
	tg := mustTarget(t, "codex")
	_, _, err := installTarget(tg, testBin, testMoodle)
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected the get failure to surface, got %v", err)
	}
	for _, c := range *calls {
		if c.Args[1] == "add" {
			t.Error("must not run `codex mcp add` after an unexplained get failure")
		}
	}
	if !hasEntry(tg) {
		t.Error("expected an unexplained get failure to list Codex for uninstall")
	}
	if _, _, err := uninstallTarget(tg); err == nil {
		t.Error("expected uninstall to report the get failure")
	}
}
