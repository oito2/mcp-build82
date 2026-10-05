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

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This file tests main()'s dispatch by running the real compiled binary as a subprocess: main()
// calls os.Exit directly, so its behavior can't be exercised in-process (an in-process os.Exit
// would kill the test runner itself).

func buildBuildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "build82")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build build82: %v\n%s", err, out)
	}
	return bin
}

// TestMain_UnknownSubcommandExitsWithError verifies that a typo'd subcommand (e.g. `build82
// instal`) fails fast with a clear message instead of starting server mode.
func TestMain_UnknownSubcommandExitsWithError(t *testing.T) {
	bin := buildBuildBinary(t)

	cmd := exec.Command(bin, "instal")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected the process to exit with a non-zero status (not hang or exit 0), got err=%v", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("expected exit code 1, got %d", exitErr.ExitCode())
	}
	if !strings.Contains(stderr.String(), `unknown command "instal"`) {
		t.Errorf("expected an 'unknown command' error naming the typo, got:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("expected the help text to be printed alongside the error, got:\n%s", stderr.String())
	}
}

// TestMain_UnrecognizedFlagStillAttemptsServerMode verifies that flag-like unrecognized first args
// (e.g. a bare, undocumented flag) still fall through to server mode; only bareword typos are
// rejected.
func TestMain_UnrecognizedFlagStillAttemptsServerMode(t *testing.T) {
	bin := buildBuildBinary(t)

	cmd := exec.Command(bin, "--totally-unrecognized-flag")
	cmd.Stdin = strings.NewReader("") // closed/empty stdin: the stdio transport sees EOF and returns quickly
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run() // exit status depends on transport EOF handling — not the point of this test

	if strings.Contains(stderr.String(), "unknown command") {
		t.Errorf("expected a flag-like unrecognized arg to NOT be treated as an unknown command, got:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "build82 server running on stdio") {
		t.Errorf("expected it to still attempt server mode, got:\n%s", stderr.String())
	}
}

// TestMain_HTTPWithExplicitEmptyTokenFailsToStart is an end-to-end test: `--token ""`
// (an explicit empty value, as a shell would produce interpolating an empty variable into
// `--token "$TOKEN"`) must be a hard configuration error that refuses to start the --http server,
// rather than silently falling back to "no token = auth disabled". Uses the real compiled binary
// (like the tests above) since main()'s error path calls os.Exit directly.
func TestMain_HTTPWithExplicitEmptyTokenFailsToStart(t *testing.T) {
	bin := buildBuildBinary(t)

	// exec.Command passes "" through as a genuinely empty argv entry — equivalent to what a shell
	// produces for `--token "$UNSET_OR_EMPTY_VAR"`, without needing an actual subshell here.
	cmd := exec.Command(bin, "--http", "--token", "", "--port", "0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected the process to exit with a non-zero status (not start the server), got err=%v", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("expected exit code 1, got %d", exitErr.ExitCode())
	}
	if !strings.Contains(stderr.String(), "empty value") {
		t.Errorf("expected an error naming the empty --token value, got:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "HTTP server listening") {
		t.Errorf("expected the server to never start listening, got:\n%s", stderr.String())
	}
}

// TestMain_HTTPWithEnvTokenDoesNotFailToStart is an end-to-end test: BUILD82_TOKEN must
// be usable as a --token fallback when --token itself isn't passed on the CLI at all — a non-empty
// env token must never trigger the "empty token" configuration error.
func TestMain_HTTPWithEnvTokenDoesNotFailToStart(t *testing.T) {
	bin := buildBuildBinary(t)

	cmd := exec.Command(bin, "--http", "--port", "0")
	cmd.Env = append(cmd.Environ(), "BUILD82_TOKEN=from-env-e2e")
	cmd.Stdin = strings.NewReader("")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	// The process runs the HTTP server indefinitely until signaled — it must not have exited
	// immediately with the empty-token config error. Give it a brief moment to fail fast if it were
	// going to, then terminate it; we only assert on stderr content, not on process lifetime.
	time.Sleep(200 * time.Millisecond)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	out := stderr.String()
	if strings.Contains(out, "empty value") || strings.Contains(out, "is set but empty") {
		t.Errorf("did not expect an empty-token config error when BUILD82_TOKEN is non-empty, got:\n%s", out)
	}
}

// TestMain_SelfUpdateRollbackRestoresBackup is the end-to-end coverage for `build82 self-update
// --rollback`: it must resolve the running binary's own path (exactly like self-update itself does via
// binpath.Resolve) and promote a "<binary>.bak" sitting next to it back into place.
//
// The just-built binary is copied to serve as its own ".bak" — it's a real, fully working build82
// binary that already answers --version, so it doubles as a legitimate "previous version" without
// needing a second, separately-versioned build. The exec path is resolved through
// filepath.EvalSymlinks first (mirroring binpath.Resolve's own fallback-on-failure behavior)
// so the .bak file is placed next to whatever path the running process will actually resolve to.
func TestMain_SelfUpdateRollbackRestoresBackup(t *testing.T) {
	bin := buildBuildBinary(t)
	resolvedBin, err := filepath.EvalSymlinks(bin)
	if err != nil {
		resolvedBin = bin
	}

	data, err := os.ReadFile(resolvedBin)
	if err != nil {
		t.Fatalf("read built binary: %v", err)
	}
	backupPath := resolvedBin + ".bak"
	if err := os.WriteFile(backupPath, data, 0o755); err != nil {
		t.Fatalf("write .bak: %v", err)
	}

	cmd := exec.Command(resolvedBin, "self-update", "--rollback")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected rollback to succeed, err=%v stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Rolled back") {
		t.Errorf("expected a success message, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if _, statErr := os.Stat(backupPath); statErr == nil {
		t.Error("expected the .bak file to be gone after being promoted back into place")
	}
}

// TestMain_SelfUpdateRollbackWithNoBackupFailsClearly confirms the CLI surfaces Rollback's "no
// backup found" error on stderr with exit code 1, rather than a panic or a silent success.
func TestMain_SelfUpdateRollbackWithNoBackupFailsClearly(t *testing.T) {
	bin := buildBuildBinary(t)
	resolvedBin, err := filepath.EvalSymlinks(bin)
	if err != nil {
		resolvedBin = bin
	}
	// No .bak file placed next to resolvedBin.

	cmd := exec.Command(resolvedBin, "self-update", "--rollback")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	exitErr, ok := runErr.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected a non-zero exit, got err=%v", runErr)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("expected exit code 1, got %d", exitErr.ExitCode())
	}
	if !strings.Contains(stderr.String(), "no backup found") {
		t.Errorf("expected a 'no backup found' error, got:\n%s", stderr.String())
	}
}
