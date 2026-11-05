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
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/oito2/mcp-build82/internal/prompt"
)

// TestWriteConfig_KeepsUserKeysAndEnvironment verifies that updating an existing build82 entry
// keeps the keys and environment variables the user added, for both environment-key spellings,
// and that an entry already holding the wanted values is left unchanged.
func TestWriteConfig_KeepsUserKeysAndEnvironment(t *testing.T) {
	cases := []struct {
		shape        configShape
		before       string
		envKey, wrap string
	}{
		{shapeCursor, `{"mcpServers":{"build82":{"type":"stdio","command":"/old","args":[],"env":{"BUILD82_MOODLE_PATH":"/old/moodle","OTHER":"x"},"disabled":true,"autoApprove":["a"]}}}`, "env", "mcpServers"},
		{shapeOpenCode, `{"mcp":{"build82":{"type":"local","command":["/old"],"environment":{"BUILD82_MOODLE_PATH":"/old/moodle","OTHER":"x"},"enabled":true,"timeout":5000}}}`, "environment", "mcp"},
	}
	for _, c := range cases {
		path := filepath.Join(t.TempDir(), "config.json")
		mustWriteFile(t, path, []byte(c.before), 0o644)
		tg := target{ID: "t", InstallPaths: fixedPaths(path), Shape: c.shape}
		if err := writeConfig(tg, path, testBin, testMoodle); err != nil {
			t.Fatal(err)
		}
		m, _, err := readConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		entry := m[c.wrap].(map[string]any)["build82"].(map[string]any)
		env := entry[c.envKey].(map[string]any)
		if env["BUILD82_MOODLE_PATH"] != testMoodle || env["OTHER"] != "x" {
			t.Errorf("%v: environment = %v", c.shape, env)
		}
		if c.shape == shapeCursor && (entry["disabled"] != true || entry["command"] != testBin || entry["autoApprove"] == nil) {
			t.Errorf("%v: entry = %v", c.shape, entry)
		}
		if c.shape == shapeOpenCode && entry["timeout"] == nil {
			t.Errorf("%v: entry = %v", c.shape, entry)
		}

		old := time.Now().Add(-time.Hour).Truncate(time.Second)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
		if err := writeConfig(tg, path, testBin, testMoodle); err != nil {
			t.Fatal(err)
		}
		if fi, err := os.Stat(path); err != nil || !fi.ModTime().Equal(old) {
			t.Errorf("%v: an up-to-date entry with extra user keys was rewritten", c.shape)
		}
	}
}

// TestInstallTarget_AddFailureAfterRemovalSaysSo verifies that when `claude mcp add` fails after
// the previous registration was removed, the error says it was removed and how to restore it.
func TestInstallTarget_AddFailureAfterRemovalSaysSo(t *testing.T) {
	fakeHome(t, "linux")
	claudeOnPath(t)
	registered := true
	fakeCLI(t, func(c recordedCall) (string, error) {
		failure := errors.New("exit status 1")
		switch c.Args[1] {
		case "get":
			if registered {
				return claudeGetUser, nil
			}
			return "No MCP server named \"build82\".", failure
		case "remove":
			registered = false
			return "Removed", nil
		case "add":
			return "Error: invalid ~/.claude.json", failure
		}
		return "", failure
	})
	replaced, _, err := installTarget(mustTarget(t, "claude"), testBin, testMoodle)
	if !replaced || err == nil || !strings.Contains(err.Error(), "already removed") || !strings.Contains(err.Error(), "build82 install claude") {
		t.Errorf("replaced=%v err=%v", replaced, err)
	}
}

// TestClaude_GetFailureIsNotTakenForNotRegistered verifies that a `claude mcp get` failure other
// than "No MCP server named" is reported by install and counted as a registration by uninstall.
func TestClaude_GetFailureIsNotTakenForNotRegistered(t *testing.T) {
	fakeHome(t, "linux")
	claudeOnPath(t)
	fakeCLI(t, func(c recordedCall) (string, error) {
		return "Error: failed to parse ~/.claude.json", errors.New("exit status 1")
	})
	tg := mustTarget(t, "claude")
	if _, _, err := removeCLIRegistration(tg.CLI); err == nil || !strings.Contains(err.Error(), "failed to parse") {
		t.Errorf("removeCLIRegistration: %v", err)
	}
	if !hasEntry(tg) {
		t.Error("hasEntry must count a failing `mcp get` as a registration, so uninstall reports the error")
	}
}

// TestCheckBatchArgs verifies that cmd.exe metacharacters are refused only for a batch-file CLI on
// Windows.
func TestCheckBatchArgs(t *testing.T) {
	prevOS, prevLook := goos, lookPath
	t.Cleanup(func() { goos, lookPath = prevOS, prevLook })
	resolved := `C:\npm\claude.cmd`
	lookPath = func(string) (string, error) { return resolved, nil }

	goos = "windows"
	if err := checkBatchArgs("claude", []string{`C:\dev\R&D\moodle`}); err == nil {
		t.Error("expected & to be refused for a .cmd CLI")
	}
	if err := checkBatchArgs("claude", []string{`C:\dev\moodle`, "--", `C:\bin\build82.exe`}); err != nil {
		t.Errorf("plain paths refused: %v", err)
	}
	resolved = `C:\bin\claude.exe`
	if err := checkBatchArgs("claude", []string{`a&b`}); err != nil {
		t.Errorf("a .exe CLI is quoted correctly by Go: %v", err)
	}
	goos, resolved = "linux", "/usr/bin/claude.cmd"
	if err := checkBatchArgs("claude", []string{`a&b`}); err != nil {
		t.Errorf("not Windows: %v", err)
	}
}

// TestRunCommand_TimesOut verifies that a tool CLI that hangs is stopped after cliTimeout.
func TestRunCommand_TimesOut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses the unix sleep command")
	}
	prev := cliTimeout
	cliTimeout = 200 * time.Millisecond
	t.Cleanup(func() { cliTimeout = prev })
	start := time.Now()
	_, err := runCommand("sleep", "5")
	if err == nil || !strings.Contains(err.Error(), "did not finish within") {
		t.Errorf("expected a timeout error, got %v", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("runCommand took %s", d)
	}
}

// TestClineCLIDirs_RelativeDataDirIgnored verifies that a relative CLINE_DATA_DIR, which would point
// at a path relative to wherever the command runs, is ignored.
func TestClineCLIDirs_RelativeDataDirIgnored(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLINE_DIR", "")
	t.Setenv("CLINE_DATA_DIR", "relative/data")
	marker, data := clineCLIDirs(home)
	if marker != filepath.Join(home, ".cline") || data != filepath.Join(home, ".cline", "data") {
		t.Errorf("marker=%q data=%q", marker, data)
	}
}

// TestUninstall_InterruptedPurgeAfterRemovalDoesNotClaimNothingChanged verifies that interrupting
// the purge confirmation after a registration was removed yields an error that does not wrap
// prompt.ErrInterrupted, so the CLI does not print "nothing was changed".
func TestUninstall_InterruptedPurgeAfterRemovalDoesNotClaimNothingChanged(t *testing.T) {
	home := fakeHome(t, "linux")
	noCLIsOnPath(t)
	dir := filepath.Join(home, ".cursor")
	mustMkdir(t, dir)
	mustWriteFile(t, filepath.Join(dir, "mcp.json"), []byte(`{"mcpServers":{"build82":{"command":"/x"}}}`), 0o644)
	r, w, err := os.Pipe() // stdin that never answers
	if err != nil {
		t.Fatal(err)
	}
	prev := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = prev; w.Close(); r.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var uerr error
	captureStdout(t, func() { uerr = Uninstall(ctx, "cursor", true) })
	if uerr == nil || errors.Is(uerr, prompt.ErrInterrupted) || !strings.Contains(uerr.Error(), "already removed") {
		t.Errorf("err = %v", uerr)
	}
	if has, _ := fileHasEntry(filepath.Join(dir, "mcp.json"), shapeCursor); has {
		t.Error("the registration should have been removed before the purge prompt")
	}
}
