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
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

const (
	testBin    = "/usr/local/bin/build82"
	testMoodle = "/var/www/moodle"
)

// fakeHome points every home/config environment variable at a fresh temp dir, clears the Cline
// overrides, isolates PATH, and pins the OS the target table is built for.
func fakeHome(t *testing.T, targetOS string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("CLINE_DIR", "")
	t.Setenv("CLINE_DATA_DIR", "")
	// An empty PATH directory keeps a claude, codex or opencode installed on the machine out of
	// reach of the real lookPath and runCommand.
	t.Setenv("PATH", t.TempDir())
	prev := goos
	goos = targetOS
	t.Cleanup(func() { goos = prev })
	return home
}

// noCLIsOnPath makes every PATH lookup fail, so detection depends only on directories.
func noCLIsOnPath(t *testing.T) {
	t.Helper()
	prev := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { lookPath = prev })
}

// recordedCall is one command the installer asked runCommand to execute.
type recordedCall struct {
	Name string
	Args []string
}

// stubCommands replaces runCommand so no real CLI runs. respond decides each call's result; nil
// means every call succeeds.
func stubCommands(t *testing.T, respond func(c recordedCall) error) *[]recordedCall {
	t.Helper()
	var calls []recordedCall
	prev := runCommand
	runCommand = func(name string, args ...string) ([]byte, error) {
		c := recordedCall{Name: name, Args: append([]string{}, args...)}
		calls = append(calls, c)
		if respond != nil {
			if err := respond(c); err != nil {
				return []byte(err.Error()), err
			}
		}
		return nil, nil
	}
	t.Cleanup(func() { runCommand = prev })
	return &calls
}

// mustTarget returns the target with the given ID, failing the test when it is missing.
func mustTarget(t *testing.T, id string) target {
	t.Helper()
	tg, ok, err := targetByID(id)
	if err != nil || !ok {
		t.Fatalf("target %q: ok=%v err=%v", id, ok, err)
	}
	return tg
}

// mustWriteFile writes `data` to `path` with mode `perm`, failing the test on error.
func mustWriteFile(t *testing.T, path string, data []byte, perm os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, perm); err != nil {
		t.Fatal(err)
	}
}

// mustReadFile returns the content of `path`, failing the test on error.
func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

// mustMkdir creates `dir` and its parents, failing the test on error.
func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

// readEntry returns the build82 entry stored under `topKey` in the JSON file at `path`, failing the test when it cannot be read.
func readEntry(t *testing.T, path, topKey string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("expected valid JSON in %s: %v", path, err)
	}
	sub, _ := parsed[topKey].(map[string]any)
	entry, ok := sub["build82"].(map[string]any)
	if !ok {
		t.Fatalf("no build82 entry under %q in %s: %s", topKey, path, content)
	}
	return entry
}

// captureStdout runs fn and returns what it printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prev := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = prev }()
	fn()
	w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

// --- claude -------------------------------------------------------------------------------------

// TestClaude_InstallUsesUserScope verifies the `claude mcp add` command registers build82 in user scope.
func TestClaude_InstallUsesUserScope(t *testing.T) {
	fakeHome(t, "linux")
	calls := fakeClaude(t, nil)
	tg := mustTarget(t, "claude")
	if tg.Label != "Claude Code" {
		t.Errorf("label = %q, want %q", tg.Label, "Claude Code")
	}
	replaced, warnings, err := installTarget(tg, testBin, testMoodle)
	if err != nil || replaced || len(warnings) != 0 {
		t.Fatalf("replaced=%v warnings=%v err=%v", replaced, warnings, err)
	}
	want := []recordedCall{
		{Name: "claude", Args: []string{"mcp", "get", "build82"}},
		{Name: "claude", Args: []string{"mcp", "add", "--scope", "user", "build82",
			"-e", "BUILD82_MOODLE_PATH=" + testMoodle, "--", testBin}},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Errorf("calls = %+v, want %+v", *calls, want)
	}
}

// TestClaude_UninstallRemovesFromUserScope verifies that uninstall removes the user-scope registration.
func TestClaude_UninstallRemovesFromUserScope(t *testing.T) {
	fakeHome(t, "linux")
	calls := fakeClaude(t, map[string]bool{scopeUser: true})
	removed, _, err := uninstallTarget(mustTarget(t, "claude"))
	if err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	want := recordedCall{Name: "claude", Args: []string{"mcp", "remove", "--scope", "user", "build82"}}
	if !containsCall(*calls, want) {
		t.Errorf("calls = %+v, want one %+v", *calls, want)
	}
}

// --- claude-desktop -----------------------------------------------------------------------------

// TestClaudeDesktop_PathsPerOS verifies the Claude Desktop config path for each operating system.
func TestClaudeDesktop_PathsPerOS(t *testing.T) {
	home := fakeHome(t, "darwin")
	tg := mustTarget(t, "claude-desktop")
	want := filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	if got := tg.InstallPaths(); !reflect.DeepEqual(got, []string{want}) {
		t.Errorf("darwin paths = %v, want %v", got, want)
	}

	home = fakeHome(t, "windows")
	tg = mustTarget(t, "claude-desktop")
	want = filepath.Join(home, "AppData", "Roaming", "Claude", "claude_desktop_config.json")
	if got := tg.InstallPaths(); !reflect.DeepEqual(got, []string{want}) {
		t.Errorf("windows paths = %v, want %v", got, want)
	}

	home = fakeHome(t, "linux")
	tg = mustTarget(t, "claude-desktop")
	want = filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
	if got := tg.InstallPaths(); !reflect.DeepEqual(got, []string{want}) {
		t.Errorf("linux paths = %v, want %v", got, want)
	}

	fakeHome(t, "linux")
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	tg = mustTarget(t, "claude-desktop")
	want = filepath.Join(xdg, "Claude", "claude_desktop_config.json")
	if got := tg.InstallPaths(); !reflect.DeepEqual(got, []string{want}) {
		t.Errorf("linux paths with XDG_CONFIG_HOME = %v, want %v", got, want)
	}
}

// TestClaudeDesktop_LinuxDetectInstallUninstall verifies detection, install and uninstall for Claude Desktop on Linux, preserving existing settings and file mode.
func TestClaudeDesktop_LinuxDetectInstallUninstall(t *testing.T) {
	home := fakeHome(t, "linux")
	noCLIsOnPath(t)
	tg := mustTarget(t, "claude-desktop")
	if tg.Unsupported != "" {
		t.Fatalf("claude-desktop must be supported on Linux, got %q", tg.Unsupported)
	}
	if detectTarget(tg) {
		t.Fatal("not detected before the config directory exists")
	}
	dir := filepath.Join(home, ".config", "Claude")
	mustMkdir(t, dir)
	path := filepath.Join(dir, "claude_desktop_config.json")
	// Shape written by Claude Desktop on Linux before any MCP server is configured: only
	// top-level preferences, no mcpServers object, mode 0600.
	mustWriteFile(t, path, []byte(`{"preferences":{"sidebarMode":"chat"},"coworkUserFilesPath":"/tmp/x"}`), 0o600)
	if !detectTarget(tg) {
		t.Fatal("expected detection once ~/.config/Claude exists")
	}
	if _, _, err := installTarget(tg, testBin, testMoodle); err != nil {
		t.Fatal(err)
	}
	entry := readEntry(t, path, "mcpServers")
	if entry["command"] != testBin || entry["env"].(map[string]any)["BUILD82_MOODLE_PATH"] != testMoodle {
		t.Errorf("unexpected entry: %+v", entry)
	}
	content := mustReadFile(t, path)
	if !strings.Contains(string(content), `"sidebarMode"`) || !strings.Contains(string(content), `"coworkUserFilesPath"`) {
		t.Errorf("existing top-level keys must be preserved, got %s", content)
	}
	if info, err := os.Stat(path); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Errorf("config file mode must stay 0600, got %v (err %v)", info.Mode().Perm(), err)
	}
	if removed, _, err := uninstallTarget(tg); err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	content = mustReadFile(t, path)
	if strings.Contains(string(content), "build82") || !strings.Contains(string(content), "preferences") {
		t.Errorf("expected only the build82 entry removed, got %s", content)
	}
}

// TestClaudeDesktop_UnsupportedOnOtherOS verifies that Claude Desktop is reported as unsupported on other operating systems.
func TestClaudeDesktop_UnsupportedOnOtherOS(t *testing.T) {
	home := fakeHome(t, "freebsd")
	mustMkdir(t, filepath.Join(home, ".config", "Claude"))
	tg := mustTarget(t, "claude-desktop")
	if tg.Unsupported == "" {
		t.Error("expected claude-desktop to be marked unsupported on FreeBSD")
	}
	if detectTarget(tg) {
		t.Error("claude-desktop must never be detected on an unsupported OS")
	}
	out := captureStdout(t, func() {
		if err := Uninstall(context.Background(), "claude-desktop", false); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "only available for macOS, Windows and Linux") {
		t.Errorf("expected a clear unsupported message, got %q", out)
	}
}

// TestClaudeDesktop_DetectInstallUninstall verifies detection, install and uninstall for Claude Desktop.
func TestClaudeDesktop_DetectInstallUninstall(t *testing.T) {
	home := fakeHome(t, "darwin")
	noCLIsOnPath(t)
	tg := mustTarget(t, "claude-desktop")
	if detectTarget(tg) {
		t.Fatal("not detected before the config directory exists")
	}
	dir := filepath.Join(home, "Library", "Application Support", "Claude")
	mustMkdir(t, dir)
	path := filepath.Join(dir, "claude_desktop_config.json")
	mustWriteFile(t, path, []byte(`{"mcpServers":{"other":{"command":"/bin/other"}},"preferences":{"x":1}}`), 0o600)
	if !detectTarget(tg) {
		t.Fatal("expected detection once the config directory exists")
	}
	if _, _, err := installTarget(tg, testBin, testMoodle); err != nil {
		t.Fatal(err)
	}
	entry := readEntry(t, path, "mcpServers")
	if entry["command"] != testBin || entry["env"].(map[string]any)["BUILD82_MOODLE_PATH"] != testMoodle {
		t.Errorf("unexpected entry: %+v", entry)
	}
	if removed, _, err := uninstallTarget(tg); err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	content := mustReadFile(t, path)
	if strings.Contains(string(content), "build82") || !strings.Contains(string(content), "preferences") {
		t.Errorf("expected only the build82 entry removed, got %s", content)
	}
}

// --- codex --------------------------------------------------------------------------------------

// TestCodex_DelegatesToCLIAndWritesNoJSON verifies that Codex is configured through its CLI and no JSON file is written.
func TestCodex_DelegatesToCLIAndWritesNoJSON(t *testing.T) {
	home := fakeHome(t, "linux")
	calls := fakeCodex(t, false)
	tg := mustTarget(t, "codex")
	if _, _, err := installTarget(tg, testBin, testMoodle); err != nil {
		t.Fatal(err)
	}
	want := []recordedCall{
		{Name: "codex", Args: []string{"mcp", "get", "build82"}},
		{Name: "codex", Args: []string{"mcp", "add", "build82",
			"--env", "BUILD82_MOODLE_PATH=" + testMoodle, "--", testBin}},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Errorf("calls = %+v, want %+v", *calls, want)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "config.json")); err == nil {
		t.Error("codex install must not write ~/.codex/config.json")
	}

	*calls = nil
	if removed, _, err := uninstallTarget(tg); err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	wantRemove := recordedCall{Name: "codex", Args: []string{"mcp", "remove", "build82"}}
	if got := (*calls)[len(*calls)-1]; !reflect.DeepEqual(got, wantRemove) {
		t.Errorf("last call = %+v, want %+v", got, wantRemove)
	}
}

// --- antigravity --------------------------------------------------------------------------------

// TestAntigravity_OfficialGlobalPathAndIDEDetection verifies the Antigravity config path and its detection by directory.
func TestAntigravity_OfficialGlobalPathAndIDEDetection(t *testing.T) {
	home := fakeHome(t, "linux")
	noCLIsOnPath(t)
	tg := mustTarget(t, "antigravity")
	if tg.Label != "Antigravity (IDE / CLI)" {
		t.Errorf("label = %q", tg.Label)
	}
	want := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	if got := tg.InstallPaths(); !reflect.DeepEqual(got, []string{want}) {
		t.Errorf("paths = %v, want %v", got, want)
	}
	if detectTarget(tg) {
		t.Fatal("not detected without agy or the Antigravity config directory")
	}
	mustMkdir(t, filepath.Join(home, ".gemini", "config"))
	if !detectTarget(tg) {
		t.Error("an IDE-only install (no agy on PATH) must be detected by its config directory")
	}
}

// --- opencode -----------------------------------------------------------------------------------

// TestOpenCode_OfficialFileAndShape verifies the OpenCode config file and entry shape.
func TestOpenCode_OfficialFileAndShape(t *testing.T) {
	home := fakeHome(t, "linux")
	tg := mustTarget(t, "opencode")
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	if got := tg.InstallPaths(); !reflect.DeepEqual(got, []string{path}) {
		t.Errorf("paths = %v, want %v", got, path)
	}
	if _, _, err := installTarget(tg, testBin, testMoodle); err != nil {
		t.Fatal(err)
	}
	entry := readEntry(t, path, "mcp")
	if _, ok := entry["env"]; ok {
		t.Errorf("OpenCode uses \"environment\", not \"env\": %+v", entry)
	}
	if entry["environment"].(map[string]any)["BUILD82_MOODLE_PATH"] != testMoodle {
		t.Errorf("unexpected environment: %+v", entry)
	}
	if entry["type"] != "local" || entry["enabled"] != true {
		t.Errorf("unexpected entry: %+v", entry)
	}
	if cmd, _ := entry["command"].([]any); len(cmd) != 1 || cmd[0] != testBin {
		t.Errorf("command must be [binary], got %+v", entry["command"])
	}
}

// TestOpenCode_ReusesExistingJSONCFile verifies that an existing opencode.jsonc is used instead of creating opencode.json.
func TestOpenCode_ReusesExistingJSONCFile(t *testing.T) {
	home := fakeHome(t, "linux")
	dir := filepath.Join(home, ".config", "opencode")
	mustMkdir(t, dir)
	jsonc := filepath.Join(dir, "opencode.jsonc")
	mustWriteFile(t, jsonc, []byte(`{"$schema": "https://opencode.ai/config.json"}`), 0o644)
	if got := mustTarget(t, "opencode").InstallPaths(); !reflect.DeepEqual(got, []string{jsonc}) {
		t.Errorf("paths = %v, want %v", got, jsonc)
	}
}

// TestOpenCode_UninstallCleansLegacyConfigJSON verifies that uninstall also removes the entry from the legacy config.json.
func TestOpenCode_UninstallCleansLegacyConfigJSON(t *testing.T) {
	home := fakeHome(t, "linux")
	dir := filepath.Join(home, ".config", "opencode")
	mustMkdir(t, dir)
	legacy := filepath.Join(dir, "config.json")
	mustWriteFile(t, legacy, []byte(`{"mcp":{"build82":{"type":"local"},"other":{"type":"local"}}}`), 0o644)
	tg := mustTarget(t, "opencode")
	if !hasEntry(tg) {
		t.Fatal("expected the legacy entry to be found")
	}
	if removed, _, err := uninstallTarget(tg); err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	content := mustReadFile(t, legacy)
	if strings.Contains(string(content), "build82") || !strings.Contains(string(content), "other") {
		t.Errorf("unexpected legacy file after uninstall: %s", content)
	}
}

// --- cursor -------------------------------------------------------------------------------------

// TestCursor_EntryDeclaresStdioType verifies that the Cursor entry declares the stdio type.
func TestCursor_EntryDeclaresStdioType(t *testing.T) {
	home := fakeHome(t, "linux")
	tg := mustTarget(t, "cursor")
	if _, _, err := installTarget(tg, testBin, testMoodle); err != nil {
		t.Fatal(err)
	}
	entry := readEntry(t, filepath.Join(home, ".cursor", "mcp.json"), "mcpServers")
	if entry["type"] != "stdio" || entry["command"] != testBin {
		t.Errorf("unexpected entry: %+v", entry)
	}
}

// --- zed ----------------------------------------------------------------------------------------

// TestZed_EntryMatchesDocumentedShape verifies the entry written under context_servers for Zed.
func TestZed_EntryMatchesDocumentedShape(t *testing.T) {
	home := fakeHome(t, "linux")
	tg := mustTarget(t, "zed")
	if _, _, err := installTarget(tg, testBin, testMoodle); err != nil {
		t.Fatal(err)
	}
	entry := readEntry(t, filepath.Join(home, ".config", "zed", "settings.json"), "context_servers")
	env, ok := entry["env"].(map[string]any)
	if !ok || env["BUILD82_MOODLE_PATH"] != testMoodle {
		t.Errorf("env must sit next to command/args: %+v", entry)
	}
	if entry["command"] != testBin {
		t.Errorf("command must be the binary path: %+v", entry)
	}
}

// TestZed_WindowsPath verifies the Zed settings path on Windows.
func TestZed_WindowsPath(t *testing.T) {
	home := fakeHome(t, "windows")
	want := filepath.Join(home, "AppData", "Roaming", "Zed", "settings.json")
	if got := mustTarget(t, "zed").InstallPaths(); !reflect.DeepEqual(got, []string{want}) {
		t.Errorf("paths = %v, want %v", got, want)
	}
}

const zedSettingsWithComments = `// Zed settings
{
  "theme": "One Dark", // keep me
  /* block */
  "context_servers": {
    "build82": {"command": "/old/build82", "args": [],},
  },
}
`

// TestZed_JSONCSettingsAreReadButNeverRewritten verifies that a settings file with comments is parsed but left unchanged, with an error showing the snippet to add.
func TestZed_JSONCSettingsAreReadButNeverRewritten(t *testing.T) {
	home := fakeHome(t, "linux")
	dir := filepath.Join(home, ".config", "zed")
	mustMkdir(t, dir)
	path := filepath.Join(dir, "settings.json")
	mustWriteFile(t, path, []byte(zedSettingsWithComments), 0o644)
	tg := mustTarget(t, "zed")

	if !hasEntry(tg) {
		t.Error("expected the entry in a commented settings.json to be found")
	}
	_, _, err := installTarget(tg, testBin, testMoodle)
	if err == nil || !strings.Contains(err.Error(), "manually") || !strings.Contains(err.Error(), testBin) {
		t.Errorf("expected a refusal carrying the manual snippet, got %v", err)
	}
	if _, _, err := uninstallTarget(tg); err == nil {
		t.Error("expected uninstall to refuse rewriting a commented settings.json")
	}
	content := mustReadFile(t, path)
	if string(content) != zedSettingsWithComments {
		t.Errorf("settings.json must be left byte-for-byte unchanged, got:\n%s", content)
	}
}

// TestStripJSONC_KeepsStringsIntact verifies that comment- and comma-like text inside strings survives stripping.
func TestStripJSONC_KeepsStringsIntact(t *testing.T) {
	src := `{"url": "http://x//y", "s": "a,}" /* c */, "arr": [1, 2,], // tail
}`
	var m map[string]any
	if err := json.Unmarshal(stripJSONC([]byte(src)), &m); err != nil {
		t.Fatalf("stripped output is not JSON: %v", err)
	}
	if m["url"] != "http://x//y" || m["s"] != "a,}" {
		t.Errorf("string contents were altered: %+v", m)
	}
}

// --- cline --------------------------------------------------------------------------------------

// TestCline_InstallsIntoEveryDetectedLocation verifies that Cline is configured in both the VS Code extension and CLI locations that exist.
func TestCline_InstallsIntoEveryDetectedLocation(t *testing.T) {
	home := fakeHome(t, "linux")
	noCLIsOnPath(t)
	vscode := filepath.Join(home, ".config", "Code", "User", "globalStorage", "saoudrizwan.claude-dev")
	cli := filepath.Join(home, ".cline")
	tg := mustTarget(t, "cline")

	mustMkdir(t, cli)
	if !detectTarget(tg) {
		t.Fatal("a Cline CLI install (~/.cline) must be detected")
	}
	mustMkdir(t, vscode)
	if _, _, err := installTarget(tg, testBin, testMoodle); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join(vscode, "settings", "cline_mcp_settings.json"),
		filepath.Join(cli, "data", "settings", "cline_mcp_settings.json"),
	} {
		if entry := readEntry(t, p, "mcpServers"); entry["command"] != testBin {
			t.Errorf("unexpected entry in %s: %+v", p, entry)
		}
	}
	if removed, _, err := uninstallTarget(tg); err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	if hasEntry(tg) {
		t.Error("expected every location cleaned by uninstall")
	}
}

// --- uninstall reporting ------------------------------------------------------------------------

// TestUninstall_ReportsNotRegisteredWhenNothingWasRemoved verifies the "not registered" report for a file target with no entry.
func TestUninstall_ReportsNotRegisteredWhenNothingWasRemoved(t *testing.T) {
	fakeHome(t, "linux")
	out := captureStdout(t, func() {
		if err := Uninstall(context.Background(), "cursor", false); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if strings.Contains(out, "removed.") || !strings.Contains(out, "not registered") {
		t.Errorf("expected \"not registered\", got %q", out)
	}
}

// TestUninstall_CLITargetNotRegistered verifies the "not registered" report for a CLI target with no registration.
func TestUninstall_CLITargetNotRegistered(t *testing.T) {
	fakeHome(t, "linux")
	prev := lookPath
	lookPath = func(string) (string, error) { return "/usr/bin/claude", nil }
	t.Cleanup(func() { lookPath = prev })
	calls := stubCommands(t, func(c recordedCall) error {
		if c.Args[1] == "get" {
			return errors.New(`No MCP server named "build82"`)
		}
		return nil
	})
	out := captureStdout(t, func() {
		if err := Uninstall(context.Background(), "claude", false); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "not registered") {
		t.Errorf("expected \"not registered\", got %q", out)
	}
	for _, c := range *calls {
		if c.Args[1] == "remove" {
			t.Error("must not run `mcp remove` when build82 is not registered")
		}
	}
}
