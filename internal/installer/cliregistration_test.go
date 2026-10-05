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

package installer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Outputs of `claude mcp get build82` as printed by Claude Code, one per scope.
const (
	claudeGetUser = `build82:
  Scope: User config (available in all your projects)
  Status: ✘ Failed to connect
  Issue: CONNECTION_CLOSED: Connection closed
  Type: stdio
  Command: /usr/local/bin/build82
  Args:
  Environment:
    BUILD82_MOODLE_PATH=/var/www/moodle

To remove this server, run: claude mcp remove build82 -s user
`
	claudeGetLocal = `build82:
  Scope: Local config (private to you in this project)
  Status: ✔ Connected
  Type: stdio
  Command: /usr/local/bin/build82
  Args:
  Environment:

To remove this server, run: claude mcp remove build82 -s local
`
	claudeGetProject = `build82:
  Scope: Project config (shared via .mcp.json)
  Status: ⏸ Pending approval (run ` + "`claude`" + ` to approve)
  Type: stdio
  Command: /usr/local/bin/build82
  Args:
  Environment:

To remove this server, run: claude mcp remove build82 -s project
`
)

// fakeClaude replaces runCommand with a model of Claude Code's `mcp get/add/remove` over the
// given scopes: `get` reports only the registration with the highest precedence (local, then
// project, then user), `add` refuses a name already present in the same scope, and `remove`
// refuses a scope without the name.
func fakeClaude(t *testing.T, scopes map[string]bool) *[]recordedCall {
	t.Helper()
	if scopes == nil {
		scopes = map[string]bool{}
	}
	outputs := map[string]string{scopeLocal: claudeGetLocal, scopeProject: claudeGetProject, scopeUser: claudeGetUser}
	return fakeCLI(t, func(c recordedCall) (string, error) {
		failure := errors.New("exit status 1")
		switch c.Args[1] {
		case "get":
			for _, s := range []string{scopeLocal, scopeProject, scopeUser} {
				if scopes[s] {
					return outputs[s], nil
				}
			}
			return "No MCP server named \"build82\". Run `claude mcp add` to add one.", failure
		case "add":
			if scopes[c.Args[3]] {
				return fmt.Sprintf("MCP server build82 already exists in %s config", c.Args[3]), failure
			}
			scopes[c.Args[3]] = true
			return "Added stdio MCP server build82", nil
		case "remove":
			if !scopes[c.Args[3]] {
				return fmt.Sprintf("No MCP server named \"build82\" in %s scope", c.Args[3]), failure
			}
			delete(scopes, c.Args[3])
			return "Removed MCP server build82", nil
		}
		return "unknown command", failure
	})
}

// fakeCodex replaces runCommand with a model of Codex's single global MCP configuration, whose
// `add` refuses a name that is already registered.
func fakeCodex(t *testing.T, registered bool) *[]recordedCall {
	t.Helper()
	return fakeCLI(t, func(c recordedCall) (string, error) {
		failure := errors.New("exit status 1")
		switch c.Args[1] {
		case "get":
			if registered {
				return "build82\n  enabled: true\n  transport: stdio\n", nil
			}
			return "Error: No MCP server named 'build82' found.", failure
		case "add":
			if registered {
				return "Error: MCP server 'build82' already exists", failure
			}
			registered = true
			return "Added global MCP server 'build82'.", nil
		case "remove":
			registered = false
			return "Removed global MCP server 'build82'.", nil
		}
		return "unknown command", failure
	})
}

func fakeCLI(t *testing.T, respond func(c recordedCall) (string, error)) *[]recordedCall {
	t.Helper()
	var calls []recordedCall
	prev := runCommand
	runCommand = func(name string, args ...string) ([]byte, error) {
		c := recordedCall{Name: name, Args: append([]string{}, args...)}
		calls = append(calls, c)
		out, err := respond(c)
		return []byte(out), err
	}
	t.Cleanup(func() { runCommand = prev })
	return &calls
}

func claudeOnPath(t *testing.T) {
	t.Helper()
	prev := lookPath
	lookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }
	t.Cleanup(func() { lookPath = prev })
}

func containsCall(calls []recordedCall, want recordedCall) bool {
	for _, c := range calls {
		if reflect.DeepEqual(c, want) {
			return true
		}
	}
	return false
}

func claudeRemove(scope string) recordedCall {
	return recordedCall{Name: "claude", Args: []string{"mcp", "remove", "--scope", scope, "build82"}}
}

// --- scope parsing -------------------------------------------------------------------------------

func TestClaudeScope_ParsesGetOutput(t *testing.T) {
	cases := map[string]string{
		claudeGetUser:    scopeUser,
		claudeGetLocal:   scopeLocal,
		claudeGetProject: scopeProject,
		strings.ReplaceAll(claudeGetProject, "\n", "\r\n"): scopeProject,
		"build82:\n  scope: LOCAL config\n":                scopeLocal,
		// Without a Scope line, the removal hint names the scope.
		"build82:\n\nTo remove this server, run: claude mcp remove build82 -s user\n": scopeUser,
		"run `claude mcp remove --scope=project build82`":                             scopeProject,
		"build82:\n  Scope: Enterprise config\n":                                      "",
		"":                                                                            "",
	}
	for out, want := range cases {
		if got := claudeScope([]byte(out)); got != want {
			t.Errorf("claudeScope(%q) = %q, want %q", out, got, want)
		}
	}
}

// --- install replaces an existing registration ----------------------------------------------------

func TestClaude_InstallReplacesExistingUserRegistration(t *testing.T) {
	fakeHome(t, "linux")
	scopes := map[string]bool{scopeUser: true}
	calls := fakeClaude(t, scopes)
	replaced, warnings, err := installTarget(mustTarget(t, "claude"), testBin, testMoodle)
	if err != nil || !replaced || len(warnings) != 0 {
		t.Fatalf("replaced=%v warnings=%v err=%v", replaced, warnings, err)
	}
	if !containsCall(*calls, claudeRemove(scopeUser)) {
		t.Errorf("expected the user registration to be removed first, calls = %+v", *calls)
	}
	if last := (*calls)[len(*calls)-1]; last.Args[1] != "add" {
		t.Errorf("expected `mcp add` last, got %+v", last)
	}
	if !scopes[scopeUser] || len(scopes) != 1 {
		t.Errorf("expected only a user registration afterwards, got %v", scopes)
	}
}

func TestClaude_InstallReplacesLocalAndUserRegistrations(t *testing.T) {
	fakeHome(t, "linux")
	scopes := map[string]bool{scopeLocal: true, scopeUser: true}
	calls := fakeClaude(t, scopes)
	replaced, _, err := installTarget(mustTarget(t, "claude"), testBin, testMoodle)
	if err != nil || !replaced {
		t.Fatalf("replaced=%v err=%v", replaced, err)
	}
	for _, s := range []string{scopeLocal, scopeUser} {
		if !containsCall(*calls, claudeRemove(s)) {
			t.Errorf("expected removal from the %s scope, calls = %+v", s, *calls)
		}
	}
	if !reflect.DeepEqual(scopes, map[string]bool{scopeUser: true}) {
		t.Errorf("expected only the new user registration, got %v", scopes)
	}
}

func TestClaude_InstallKeepsProjectRegistrationAndWarns(t *testing.T) {
	fakeHome(t, "linux")
	scopes := map[string]bool{scopeProject: true, scopeUser: true}
	calls := fakeClaude(t, scopes)
	replaced, warnings, err := installTarget(mustTarget(t, "claude"), testBin, testMoodle)
	if err != nil || !replaced {
		t.Fatalf("replaced=%v err=%v", replaced, err)
	}
	if containsCall(*calls, claudeRemove(scopeProject)) {
		t.Error("a project-scope registration must never be removed")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "claude mcp remove --scope project build82") {
		t.Errorf("expected a warning with the manual command, got %v", warnings)
	}
	if !scopes[scopeProject] || !scopes[scopeUser] {
		t.Errorf("expected the project entry kept and the user entry re-added, got %v", scopes)
	}
}

func TestClaude_InstallWithOnlyProjectRegistrationAddsUserScope(t *testing.T) {
	fakeHome(t, "linux")
	scopes := map[string]bool{scopeProject: true}
	fakeClaude(t, scopes)
	replaced, warnings, err := installTarget(mustTarget(t, "claude"), testBin, testMoodle)
	if err != nil || replaced || len(warnings) != 1 {
		t.Fatalf("replaced=%v warnings=%v err=%v", replaced, warnings, err)
	}
	if !scopes[scopeProject] || !scopes[scopeUser] {
		t.Errorf("expected project kept and user added, got %v", scopes)
	}
}

func TestClaude_UnknownScopeFailsWithoutChanges(t *testing.T) {
	fakeHome(t, "linux")
	calls := fakeCLI(t, func(c recordedCall) (string, error) {
		if c.Args[1] == "get" {
			return "build82:\n  Scope: Enterprise config\n", nil
		}
		return "", nil
	})
	if _, _, err := installTarget(mustTarget(t, "claude"), testBin, testMoodle); err == nil {
		t.Fatal("expected an error when the scope cannot be determined")
	}
	if len(*calls) != 1 {
		t.Errorf("expected only `mcp get` to run, calls = %+v", *calls)
	}
}

func TestClaude_RunPrintsUpdated(t *testing.T) {
	fakeHome(t, "linux")
	claudeOnPath(t)
	fakeClaude(t, map[string]bool{scopeUser: true})
	moodle := t.TempDir()
	for _, f := range []string{"version.php", "config-dist.php"} {
		os.WriteFile(filepath.Join(moodle, f), []byte("<?php\n"), 0o644)
	}
	mustMkdir(t, filepath.Join(moodle, "lib"))
	withStdin(t, moodle+"\n")
	prevWd, _ := os.Getwd()
	os.Chdir(t.TempDir())
	t.Cleanup(func() { os.Chdir(prevWd) })

	out := captureStdout(t, func() {
		if err := Run("claude"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "Claude Code... updated.") {
		t.Errorf("expected \"Claude Code... updated.\", got %q", out)
	}
}

func TestCodex_InstallReplacesExistingRegistration(t *testing.T) {
	fakeHome(t, "linux")
	calls := fakeCodex(t, true)
	replaced, _, err := installTarget(mustTarget(t, "codex"), testBin, testMoodle)
	if err != nil || !replaced {
		t.Fatalf("replaced=%v err=%v", replaced, err)
	}
	var verbs []string
	for _, c := range *calls {
		verbs = append(verbs, c.Args[1])
	}
	if want := []string{"get", "remove", "add"}; !reflect.DeepEqual(verbs, want) {
		t.Errorf("verbs = %v, want %v", verbs, want)
	}
}

func TestFileTarget_InstallReportsUpdatedOnlyWhenEntryExisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	tg := target{ID: "cursor", InstallPaths: fixedPaths(path), Shape: shapeMcpServers}
	if replaced, _, err := installTarget(tg, testBin, testMoodle); err != nil || replaced {
		t.Fatalf("first install: replaced=%v err=%v", replaced, err)
	}
	if replaced, _, err := installTarget(tg, testBin, testMoodle); err != nil || !replaced {
		t.Fatalf("second install: replaced=%v err=%v", replaced, err)
	}
}

// --- uninstall across Claude Code scopes -------------------------------------------------------------

func TestClaude_UninstallRemovesLocalScope(t *testing.T) {
	fakeHome(t, "linux")
	scopes := map[string]bool{scopeLocal: true}
	calls := fakeClaude(t, scopes)
	removed, warnings, err := uninstallTarget(mustTarget(t, "claude"))
	if err != nil || !removed || len(warnings) != 0 {
		t.Fatalf("removed=%v warnings=%v err=%v", removed, warnings, err)
	}
	if !containsCall(*calls, claudeRemove(scopeLocal)) || len(scopes) != 0 {
		t.Errorf("expected the local registration removed, calls = %+v, left %v", *calls, scopes)
	}
}

func TestClaude_UninstallRemovesEveryScopeButProject(t *testing.T) {
	fakeHome(t, "linux")
	scopes := map[string]bool{scopeLocal: true, scopeProject: true, scopeUser: true}
	calls := fakeClaude(t, scopes)
	removed, warnings, err := uninstallTarget(mustTarget(t, "claude"))
	if err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	if containsCall(*calls, claudeRemove(scopeProject)) {
		t.Error("a project-scope registration must never be removed")
	}
	if !reflect.DeepEqual(scopes, map[string]bool{scopeProject: true}) {
		t.Errorf("expected only the project registration left, got %v", scopes)
	}
	if len(warnings) != 1 {
		t.Errorf("expected one project-scope warning, got %v", warnings)
	}
}

func TestClaude_UninstallProjectOnlyWarnsAndSucceeds(t *testing.T) {
	fakeHome(t, "linux")
	claudeOnPath(t)
	scopes := map[string]bool{scopeProject: true}
	calls := fakeClaude(t, scopes)
	out := captureStdout(t, func() {
		if err := Uninstall("claude", false); err != nil {
			t.Errorf("a project-scope registration must not make uninstall fail: %v", err)
		}
	})
	if containsCall(*calls, claudeRemove(scopeProject)) || !scopes[scopeProject] {
		t.Error("a project-scope registration must never be removed")
	}
	for _, want := range []string{"Claude Code... nothing removed.", "Warning:", "claude mcp remove --scope project build82"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got %q", want, out)
		}
	}
}

// --- Cline CLI data directory ---------------------------------------------------------------------

func TestCline_HonorsClineDataDir(t *testing.T) {
	home := fakeHome(t, "linux")
	noCLIsOnPath(t)
	dataDir := filepath.Join(t.TempDir(), "cline-data")
	t.Setenv("CLINE_DATA_DIR", dataDir)
	tg := mustTarget(t, "cline")
	if detectTarget(tg) {
		t.Fatal("not detected before CLINE_DATA_DIR exists")
	}
	mustMkdir(t, dataDir)
	mustMkdir(t, filepath.Join(home, ".cline"))
	if !detectTarget(tg) {
		t.Fatal("expected detection through CLINE_DATA_DIR")
	}
	want := filepath.Join(dataDir, "settings", "cline_mcp_settings.json")
	if got := tg.InstallPaths(); !reflect.DeepEqual(got, []string{want}) {
		t.Errorf("paths = %v, want %v", got, []string{want})
	}
	if _, _, err := installTarget(tg, testBin, testMoodle); err != nil {
		t.Fatal(err)
	}
	if entry := readEntry(t, want, "mcpServers"); entry["command"] != testBin {
		t.Errorf("unexpected entry: %+v", entry)
	}
	if _, err := os.Stat(filepath.Join(home, ".cline", "data")); err == nil {
		t.Error("~/.cline/data must not be written when CLINE_DATA_DIR is set")
	}
	if removed, _, err := uninstallTarget(tg); err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	if hasEntry(tg) {
		t.Error("expected the CLINE_DATA_DIR entry removed")
	}
}

// withStdin feeds input to os.Stdin for the duration of the test.
func withStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.WriteString(input)
	w.Close()
	prev := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = prev; r.Close() })
}
