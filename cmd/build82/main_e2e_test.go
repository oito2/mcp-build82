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
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The tests in this file run the compiled binary as a subprocess, because main calls os.Exit and
// cannot be exercised in-process.

// buildBuildBinary compiles the command into a temporary directory and returns the path of the
// binary (with the ".exe" suffix on Windows). It fails the test when the build fails.
func buildBuildBinary(t *testing.T) string {
	t.Helper()
	name := "build82"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(t.TempDir(), name)
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

// TestMain_UsageErrorsExitWithCode2 verifies that unknown flags, missing flag values and
// server-only flags without --http print a message on stderr and exit with status 2, for every
// subcommand.
func TestMain_UsageErrorsExitWithCode2(t *testing.T) {
	bin := buildBuildBinary(t)

	tests := [][]string{
		{"--totally-unrecognized-flag"},
		{"--port", "8080"},
		{"--http", "--port"},
		{"--token", "x"},
		{"install", "--bogus"},
		{"self-update", "--channel"},
		{"self-update", "--bogus"},
		{"self-update", "--rollback", "--check"},
		{"self-update", "--check", "--require-signature"},
		{"uninstall", "--bogus"},
	}
	for _, args := range tests {
		cmd := exec.Command(bin, args...)
		cmd.Stdin = strings.NewReader("")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
			t.Errorf("%v: expected exit status 2, got %v", args, err)
		}
		if !strings.Contains(stderr.String(), "Error:") {
			t.Errorf("%v: expected an error message on stderr, got %q", args, stderr.String())
		}
	}
}

// TestMain_HTTPWithExplicitEmptyTokenFailsToStart verifies that an explicit empty `--token ""`
// makes the binary exit with status 1 and never start the --http server, instead of falling back
// to running without authentication.
func TestMain_HTTPWithExplicitEmptyTokenFailsToStart(t *testing.T) {
	bin := buildBuildBinary(t)

	// exec.Command passes "" as an empty argv entry, as a shell does for an empty variable.
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

// TestMain_HTTPWithEnvTokenDoesNotFailToStart verifies that a non-empty BUILD82_TOKEN is accepted
// as the token when --token is not passed, without the empty-token configuration error.
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
	// The server runs until signaled. Allow a brief moment for a startup failure to show, then
	// kill it; only stderr is checked, not the process lifetime.
	time.Sleep(200 * time.Millisecond)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	out := stderr.String()
	if strings.Contains(out, "empty value") || strings.Contains(out, "is set but empty") {
		t.Errorf("did not expect an empty-token config error when BUILD82_TOKEN is non-empty, got:\n%s", out)
	}
}

// TestMain_SelfUpdateRollbackRestoresBackup verifies that `build82 self-update --rollback`
// resolves the running binary's path and promotes the "<binary>.bak" file next to it back into
// place, printing a confirmation.
//
// A copy of the built binary serves as the backup. The path is resolved with
// filepath.EvalSymlinks first, so the backup sits next to the path the process will resolve.
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

// TestMain_SelfUpdateRollbackWithNoBackupFailsClearly verifies that a rollback without a backup
// file reports "no backup found" on stderr and exits with status 1.
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

// TestMain_SubcommandHelpPrintsWithoutRunning verifies that -h/--help after install, uninstall or
// self-update prints that subcommand's help on stdout and exits 0 without running it, and that a
// help flag after "--" is not taken as a help request.
func TestMain_SubcommandHelpPrintsWithoutRunning(t *testing.T) {
	bin := buildBuildBinary(t)
	cases := map[string][]string{
		"Usage: build82 install [target]":   {"install", "--help"},
		"Usage: build82 uninstall [target]": {"uninstall", "-h"},
		"Usage: build82 self-update":        {"self-update", "--check", "--help"},
	}
	for want, args := range cases {
		cmd := exec.Command(bin, args...)
		cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "PATH="+t.TempDir())
		cmd.Stdin = strings.NewReader("")
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Errorf("%v: expected exit 0, got %v (stderr %q)", args, err, stderr.String())
		}
		if !strings.HasPrefix(stdout.String(), want) || stderr.Len() != 0 {
			t.Errorf("%v: stdout=%q stderr=%q, want stdout starting with %q", args, stdout.String(), stderr.String(), want)
		}
	}

	cmd := exec.Command(bin, "uninstall", "--", "--help")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "PATH="+t.TempDir())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 || !strings.Contains(stderr.String(), `unknown target "--help"`) {
		t.Errorf(`uninstall -- --help: expected "--help" taken as the target (exit 1, unknown target), got err=%v stderr=%q`, err, stderr.String())
	}
}

// TestMain_UsageErrorsPointToSubcommandHelp verifies that a usage error of a subcommand names that
// subcommand's help, and that a server flag error names the global help.
func TestMain_UsageErrorsPointToSubcommandHelp(t *testing.T) {
	bin := buildBuildBinary(t)
	cases := map[string][]string{
		"Run 'build82 install --help' for usage.":     {"install", "--bogus"},
		"Run 'build82 uninstall --help' for usage.":   {"uninstall", "a", "b"},
		"Run 'build82 self-update --help' for usage.": {"self-update", "--", "extra"},
		"Run 'build82 --help' for usage.":             {"--port", "1"},
	}
	for want, args := range cases {
		cmd := exec.Command(bin, args...)
		cmd.Stdin = strings.NewReader("")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
			t.Errorf("%v: expected exit 2, got %v", args, err)
		}
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("%v: expected %q on stderr, got %q", args, want, stderr.String())
		}
	}
}

// TestMain_CtrlCAtPromptExitsWithoutChanges verifies that an interrupt while `build82 uninstall`
// waits at its confirmation prompt exits at once with status 1 and "Interrupted; nothing was
// changed.", leaving the client configuration untouched.
func TestMain_CtrlCAtPromptExitsWithoutChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Interrupt cannot be sent to a process on Windows")
	}
	bin := buildBuildBinary(t)
	home := t.TempDir()
	cursorDir := filepath.Join(home, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(cursorDir, "mcp.json")
	original := []byte(`{"mcpServers":{"build82":{"command":"/x"}}}`)
	if err := os.WriteFile(config, original, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "uninstall")
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME=", "PATH="+t.TempDir())
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	prompted := make(chan struct{})
	go func() {
		r := bufio.NewReader(stdout)
		var seen strings.Builder
		buf := make([]byte, 256)
		for {
			n, err := r.Read(buf)
			seen.Write(buf[:n])
			if strings.Contains(seen.String(), "[y/N]") {
				close(prompted)
				_, _ = io.Copy(io.Discard, r)
				return
			}
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-prompted:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("the confirmation prompt never appeared (stderr %q)", stderr.String())
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("the process did not exit after the interrupt")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Errorf("expected exit status 1, got %v", err)
	}
	if !strings.Contains(stderr.String(), "Interrupted; nothing was changed.") {
		t.Errorf("expected the interrupt message on stderr, got %q", stderr.String())
	}
	if got, err := os.ReadFile(config); err != nil || !bytes.Equal(got, original) {
		t.Errorf("expected %s untouched, got %q (err=%v)", config, got, err)
	}
}

// TestMain_StdioAcceptsMessagesAboveTheSDKDefault verifies that the stdio server accepts an
// inbound JSON-RPC message larger than the SDK's default line limit (16 MiB) and answers it,
// instead of dropping the connection.
func TestMain_StdioAcceptsMessagesAboveTheSDKDefault(t *testing.T) {
	bin := buildBuildBinary(t)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "BUILD82_MOODLE_PATH=")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

	big := strings.Repeat("x", 20<<20)
	go func() {
		_, _ = io.WriteString(stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`+"\n")
		_, _ = io.WriteString(stdin, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n")
		_, _ = io.WriteString(stdin, `{"jsonrpc":"2.0","id":2,"method":"ping","params":{"_meta":{"padding":"`+big+`"}}}`+"\n")
	}()

	answered := make(chan bool, 1)
	go func() {
		r := bufio.NewReader(stdout)
		for {
			line, err := r.ReadString('\n')
			if strings.Contains(line, `"id":2`) {
				answered <- !strings.Contains(line, `"error"`)
				return
			}
			if err != nil {
				answered <- false
				return
			}
		}
	}()
	select {
	case ok := <-answered:
		if !ok {
			t.Fatal("the 20 MiB request was not answered successfully")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("no answer to the 20 MiB request")
	}
}
